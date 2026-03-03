package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time
	Version = "dev"
	// BuildTime is set at build time
	BuildTime = "unknown"

	// httpClient is reused across sendLog calls for connection pooling.
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
)

// LogCmd creates the log command.
func LogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [message]",
		Short: "Send logs to the GLog server",
		Long: `Send log entries to the GLog server from the command line.

The log can be provided as:
1. Command line argument: glog log "Application started"
2. Via stdin: echo "Error occurred" | glog log --level error
3. Streaming: journalctl -f | glog log --stream --level info

If no config file exists, --server and --api-key are required.
Otherwise, they can be loaded from the config file (default: ./glog.json).

Examples:
  # Send an info log (using config file)
  glog log "User login successful"

  # Send an error log
  glog log --level error "Database connection failed"

  # Send from stdin (waits for EOF)
  cat application.log | glog log --level info

  # Stream continuously (line by line, for journalctl -f etc)
  journalctl -f | glog log --stream --level info
  tail -f /var/log/syslog | glog log --stream --level info

  # Send with additional fields
  glog log --level warn --field retry_count=3 "API rate limit warning"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			level, _ := cmd.Flags().GetString("level")
			fields, _ := cmd.Flags().GetStringSlice("field")
			configPath, _ := cmd.Flags().GetString("config")
			streamMode, _ := cmd.Flags().GetBool("stream")

			// Resolve server and API key from flag → env → config
			server, err := resolveServer(cmd, configPath)
			if err != nil {
				return err
			}
			apiKey, err := resolveAPIKey(cmd, configPath)
			if err != nil {
				return err
			}

			// Parse fields
			fieldMap := parseFields(fields)

			// Validate log level
			if !constants.IsValidLogLevel(level) {
				return fmt.Errorf("invalid level '%s'. Must be one of: %s", level, strings.Join(constants.ValidLogLevels, ", "))
			}

			// Stream mode: read stdin line by line
			if streamMode {
				return streamLogs(server, apiKey, level, fieldMap)
			}

			// Get message from args or stdin
			message := getMessageFromArgs(args)
			if message == "" {
				return fmt.Errorf("log message is required (provide as argument or via stdin)")
			}

			// Send log
			logID, err := sendLog(server, apiKey, level, message, fieldMap)
			if err != nil {
				return fmt.Errorf("failed to send log: %w", err)
			}

			fmt.Printf("✓ Log sent successfully (ID: %d)\n", logID)
			return nil
		},
	}

	cmd.Flags().String("server", "", "GLog server URL")
	cmd.Flags().String("api-key", "", "API key for authentication")
	cmd.Flags().String("level", "info", "Log level (trace, debug, info, warn, error, fatal)")
	cmd.Flags().StringSlice("field", nil, "Additional fields in key=value format (can be used multiple times)")
	cmd.Flags().String("config", "./glog.json", "Path to the configuration file")
	cmd.Flags().Bool("stream", false, "Stream mode: read stdin line by line and send each as a separate log")

	return cmd
}

// getMessageFromArgs gets the log message from cobra args or stdin.
func getMessageFromArgs(args []string) string {
	// Try to get from args first
	if len(args) > 0 {
		return args[0]
	}

	// Check if stdin has data
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Stdin has data, read it
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read stdin: %v\n", err)
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	return ""
}

// sendBulkLogs sends multiple log entries to the GLog server via the bulk endpoint.
func sendBulkLogs(server, apiKey, level string, messages []string, fields map[string]any) (int, error) {
	logs := make([]map[string]any, len(messages))
	for i, msg := range messages {
		entry := map[string]any{
			"level":   level,
			"message": msg,
		}
		if len(fields) > 0 {
			entry["fields"] = fields
		}
		logs[i] = entry
	}

	reqBody := map[string]any{"logs": logs}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/logs/bulk", server)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Count   int  `json:"count"`
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Count, nil
}

// streamLogs reads stdin line by line, batches lines, and sends them via the bulk endpoint.
func streamLogs(server, apiKey, level string, fields map[string]any) error {
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	const batchSize = 100
	const flushInterval = 500 * time.Millisecond

	batch := make([]string, 0, batchSize)
	sent := 0
	errCount := 0
	lastFlush := time.Now()

	fmt.Fprintf(os.Stderr, "Streaming logs to %s (level: %s, batch size: %d). Press Ctrl+C to stop.\n", server, level, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		count, err := sendBulkLogs(server, apiKey, level, batch, fields)
		if err != nil {
			errCount += len(batch)
			fmt.Fprintf(os.Stderr, "✗ Failed to send batch of %d: %v\n", len(batch), err)
		} else {
			sent += count
		}
		batch = batch[:0]
		lastFlush = time.Now()

		if sent > 0 && (sent%1000 < batchSize || errCount > 0) {
			fmt.Fprintf(os.Stderr, "✓ Sent %d logs (%d errors)\n", sent, errCount)
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		batch = append(batch, line)

		if len(batch) >= batchSize || time.Since(lastFlush) >= flushInterval {
			flush()
		}
	}

	// Flush remaining
	flush()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Stream ended. Total: %d sent, %d errors\n", sent, errCount)
	return nil
}

// parseFields parses key=value fields into a map.
func parseFields(fields []string) map[string]any {
	result := make(map[string]any)

	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Warning: invalid field format '%s' (should be key=value)\n", field)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			fmt.Fprintf(os.Stderr, "Warning: empty field key\n")
			continue
		}

		// Try to parse as number
		if num, err := parseNumber(value); err == nil {
			result[key] = num
			continue
		}

		// Try to parse as boolean
		if value == "true" {
			result[key] = true
			continue
		}
		if value == "false" {
			result[key] = false
			continue
		}

		// Default to string
		result[key] = value
	}

	return result
}

// parseNumber attempts to parse a string as int or float.
func parseNumber(s string) (any, error) {
	// Try integer first
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}

	// Try float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}

	return nil, fmt.Errorf("not a number")
}

// sendLog sends a log entry to the GLog server.
func sendLog(server, apiKey, level, message string, fields map[string]any) (int64, error) {
	// Prepare request body
	reqBody := map[string]any{
		"level":   level,
		"message": message,
	}

	if len(fields) > 0 {
		reqBody["fields"] = fields
	}

	// Marshal JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	url := fmt.Sprintf("%s/api/v1/logs", server)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		ID      int64 `json:"id"`
		HostID  int64 `json:"host_id"`
		Success bool  `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.ID, nil
}

// VersionCmd creates the version command.
func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("GLog version: %s\n", Version)
			fmt.Printf("Build time: %s\n", BuildTime)
		},
	}
}
