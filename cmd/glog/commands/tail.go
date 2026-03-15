package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/spf13/cobra"
)

// ANSI color codes for log levels.
const (
	ansiReset     = "\033[0m"
	ansiFatal     = "\033[1;91m" // bright red + bold
	ansiError     = "\033[91m"   // red
	ansiWarn      = "\033[93m"   // yellow
	ansiInfo      = "\033[94m"   // blue
	ansiDebug     = "\033[90m"   // gray
	ansiTrace     = "\033[90m"   // dark gray
	ansiConnected = "\033[92m"   // green
)

// tailLogEvent mirrors the SSE log.created payload for client-side parsing.
type tailLogEvent struct {
	ID        int64          `json:"id"`
	HostID    int64          `json:"host_id"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Timestamp string         `json:"timestamp"`
	CreatedAt string         `json:"created_at"`
	Host      *tailHostInfo  `json:"host,omitempty"`
}

type tailHostInfo struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags,omitempty"`
	Status string   `json:"status,omitempty"`
}

// TailCmd creates the tail command.
func TailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream live logs from the GLog server",
		Long: `Connect to the GLog server's SSE endpoint and stream logs in real time.

Output is color-coded by log level. Use --level to filter by minimum severity
and --host to filter by host name.

Examples:
  # Stream all logs
  glog tail --server http://localhost:6016

  # Only errors and above
  glog tail --level error

  # Filter by host name
  glog tail --host prod-api

  # Raw JSON output
  glog tail --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			server, err := resolveServer(cmd, configPath)
			if err != nil {
				return err
			}

			levelFilter, _ := cmd.Flags().GetString("level")
			hostFilter, _ := cmd.Flags().GetString("host")
			jsonOutput, _ := cmd.Flags().GetBool("json")

			// Validate level filter if provided
			if levelFilter != "" {
				if _, ok := entities.LogPriority[entities.LogLevel(levelFilter)]; !ok {
					return fmt.Errorf("invalid level filter '%s'. Must be one of: trace, debug, info, warn, error, fatal", levelFilter)
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Fprintln(os.Stderr, "\nDisconnecting...")
				cancel()
			}()

			return tailLogs(ctx, server, levelFilter, hostFilter, jsonOutput)
		},
	}

	cmd.Flags().String("server", "", "GLog server URL")
	cmd.Flags().String("config", "./glog.json", "Path to the configuration file")
	cmd.Flags().String("level", "", "Minimum log level to show (trace, debug, info, warn, error, fatal)")
	cmd.Flags().String("host", "", "Filter by host name")
	cmd.Flags().Bool("json", false, "Output raw JSON instead of formatted text")

	return cmd
}

// tailLogs connects to the SSE endpoint and streams logs with reconnection.
func tailLogs(ctx context.Context, server, levelFilter, hostFilter string, jsonOutput bool) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		err := streamSSE(ctx, server, levelFilter, hostFilter, jsonOutput)
		if ctx.Err() != nil {
			return nil // graceful shutdown
		}

		fmt.Fprintf(os.Stderr, "Connection lost: %v. Reconnecting in %s...\n", err, backoff)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		// Exponential backoff: 1s, 2s, 4s, ..., 30s
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// streamSSE opens an SSE connection and processes events until error or context cancellation.
func streamSSE(ctx context.Context, server, levelFilter, hostFilter string, jsonOutput bool) error {
	url := fmt.Sprintf("%s/api/v1/events", server)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// SSE connections are long-lived — no timeout.
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	fmt.Fprintf(os.Stderr, "Connected to %s\n", server)

	scanner := bufio.NewScanner(resp.Body)
	// Allow up to 1MB lines for large JSON payloads
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Comment line (keep-alive)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Empty line = end of event
		if line == "" {
			if eventType != "" && len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				processSSEEvent(eventType, data, levelFilter, hostFilter, jsonOutput)
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	return fmt.Errorf("stream ended")
}

// processSSEEvent handles a single SSE event.
func processSSEEvent(eventType, data, levelFilter, hostFilter string, jsonOutput bool) {
	switch eventType {
	case "log.created":
		var logEvent tailLogEvent
		if err := json.Unmarshal([]byte(data), &logEvent); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse log event: %v\n", err)
			return
		}

		// Apply level filter
		if levelFilter != "" {
			logPriority, lok := entities.LogPriority[entities.LogLevel(logEvent.Level)]
			filterPriority, fok := entities.LogPriority[entities.LogLevel(levelFilter)]
			if lok && fok && logPriority < filterPriority {
				return
			}
		}

		// Apply host filter
		if hostFilter != "" {
			hostName := ""
			if logEvent.Host != nil {
				hostName = logEvent.Host.Name
			}
			if !strings.EqualFold(hostName, hostFilter) {
				return
			}
		}

		if jsonOutput {
			fmt.Println(data)
		} else {
			printFormattedLog(&logEvent)
		}

	case "connected":
		fmt.Fprintf(os.Stderr, "SSE stream active\n")
	}
}

// printFormattedLog prints a log entry with ANSI colors.
func printFormattedLog(log *tailLogEvent) {
	// Parse timestamp
	ts, err := time.Parse(time.RFC3339, log.Timestamp)
	if err != nil {
		ts, err = time.Parse(time.RFC3339Nano, log.Timestamp)
		if err != nil {
			ts = time.Now()
		}
	}
	localTime := ts.Local().Format("2006-01-02 15:04:05")

	// Format level with color
	level := strings.ToUpper(log.Level)
	color := levelColor(log.Level)
	paddedLevel := fmt.Sprintf("%-5s", level)

	// Host name
	hostName := "unknown"
	if log.Host != nil && log.Host.Name != "" {
		hostName = log.Host.Name
	}

	fmt.Printf("%s %s[%s]%s %s: %s\n",
		localTime,
		color,
		paddedLevel,
		ansiReset,
		hostName,
		log.Message,
	)
}

// levelColor returns the ANSI escape code for a log level.
func levelColor(level string) string {
	switch entities.LogLevel(level) {
	case entities.LogLevelFatal:
		return ansiFatal
	case entities.LogLevelError:
		return ansiError
	case entities.LogLevelWarn:
		return ansiWarn
	case entities.LogLevelInfo:
		return ansiInfo
	case entities.LogLevelDebug:
		return ansiDebug
	case entities.LogLevelTrace:
		return ansiTrace
	default:
		return ansiReset
	}
}
