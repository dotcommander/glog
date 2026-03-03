package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// resolveServer resolves the server URL from flag, env var, or config file.
func resolveServer(cmd *cobra.Command, configPath string) (string, error) {
	server, _ := cmd.Flags().GetString("server")
	if server == "" {
		server = os.Getenv("GLOG_SERVER")
	}
	if server == "" {
		if config, err := loadConfig(configPath); err == nil {
			server = config.Server
		}
	}
	if server == "" {
		return "", fmt.Errorf("server is required (provide --server or set GLOG_SERVER)")
	}
	return server, nil
}

// resolveAPIKey resolves the API key from flag, env var, or config file.
func resolveAPIKey(cmd *cobra.Command, configPath string) (string, error) {
	apiKey, _ := cmd.Flags().GetString("api-key")
	if apiKey == "" {
		apiKey = os.Getenv("GLOG_API_KEY")
	}
	if apiKey == "" {
		if config, err := loadConfig(configPath); err == nil {
			apiKey = config.APIKey
		}
	}
	if apiKey == "" {
		return "", fmt.Errorf("API key is required (provide --api-key or run 'glog host register' first)")
	}
	return apiKey, nil
}

// HostCmd creates the host command.
func HostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage hosts",
	}

	cmd.AddCommand(hostRegisterCmd(), hostListCmd())
	return cmd
}

func hostRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new host with the GLog server",
		Long: `Register a new host to start sending logs.

This command will:
- Create a new host on the GLog server
- Generate a unique API key for authentication
- Save the configuration to a file for future use

Example:
  glog host register --server https://logs.example.com --name "production-api" --tag production --tag api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			server, err := resolveServer(cmd, configPath)
			if err != nil {
				return err
			}

			name, _ := cmd.Flags().GetString("name")
			tags, _ := cmd.Flags().GetStringSlice("tag")
			description, _ := cmd.Flags().GetString("description")
			hostname, _ := cmd.Flags().GetString("hostname")
			ip, _ := cmd.Flags().GetString("ip")

			fmt.Printf("Registering host '%s' with server %s...\n", name, server)

			// Call API to register host
			host, err := registerHost(server, name, tags, description, hostname, ip)
			if err != nil {
				return fmt.Errorf("failed to register host: %w", err)
			}

			fmt.Printf("\n✓ Host registered successfully!\n")
			fmt.Printf("  Name: %s\n", host.Name)
			fmt.Printf("  API Key: %s\n", host.APIKey)
			fmt.Printf("  Tags: %v\n", host.Tags)
			if host.Description != "" {
				fmt.Printf("  Description: %s\n", host.Description)
			}

			// Save configuration
			config := &CLIConfig{
				Server:   server,
				APIKey:   host.APIKey,
				HostID:   host.ID,
				HostName: host.Name,
			}

			if err := saveConfig(configPath, config); err != nil {
				fmt.Printf("\n⚠ Warning: Failed to save config: %v\n", err)
				fmt.Printf("Please save the API key above manually.\n")
			} else {
				fmt.Printf("\n✓ Configuration saved to: %s\n", configPath)
				fmt.Printf("  You can now use 'glog log' to send logs without specifying --server or --api-key\n")
			}

			return nil
		},
	}

	cmd.Flags().String("server", "", "GLog server URL (e.g., https://logs.example.com)")
	cmd.Flags().String("name", "", "Host name (must be unique)")
	cmd.Flags().StringSlice("tag", nil, "Tags for categorizing the host (can be used multiple times)")
	cmd.Flags().String("description", "", "Optional description of the host")
	cmd.Flags().String("config", "./glog.json", "Path to save the configuration file")
	cmd.Flags().String("hostname", "", "Actual hostname of the machine")
	cmd.Flags().String("ip", "", "IP address of the host")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func hostListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := resolveServer(cmd, "")
			if err != nil {
				return err
			}

			fmt.Printf("Fetching hosts from %s...\n", server)

			hosts, err := listHosts(server)
			if err != nil {
				return fmt.Errorf("failed to list hosts: %w", err)
			}

			if len(hosts) == 0 {
				fmt.Println("No hosts registered")
				return nil
			}

			fmt.Printf("\nFound %d host(s):\n\n", len(hosts))
			for _, host := range hosts {
				fmt.Printf("ID: %d\n", host.ID)
				fmt.Printf("Name: %s\n", host.Name)
				fmt.Printf("Status: %s\n", host.Status)
				fmt.Printf("Tags: %v\n", host.Tags)
				fmt.Printf("Created: %s\n", host.CreatedAt)
				fmt.Printf("Last seen: %s\n", host.LastSeen)
				fmt.Printf("Logs: %d\n", host.LogCount)
				if host.Description != "" {
					fmt.Printf("Description: %s\n", host.Description)
				}
				fmt.Println()
			}

			return nil
		},
	}

	cmd.Flags().String("server", "", "GLog server URL")

	return cmd
}

// HostRegistrationResponse represents the response from the host registration API.
type HostRegistrationResponse struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	APIKey      string   `json:"api_key"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"created_at"`
	LastSeen    string   `json:"last_seen"`
	LogCount    int64    `json:"log_count"`
	ErrorCount  int64    `json:"error_count"`
	ErrorRate   float64  `json:"error_rate"`
	Description string   `json:"description,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	IP          string   `json:"ip,omitempty"`
}

// registerHost calls the API to register a new host.
func registerHost(server, name string, tags []string, description, hostname, ip string) (*HostRegistrationResponse, error) {
	// Prepare request body
	reqBody := map[string]any{
		"name":        name,
		"tags":        tags,
		"description": description,
		"hostname":    hostname,
		"ip":          ip,
	}

	// Remove empty fields
	if description == "" {
		delete(reqBody, "description")
	}
	if hostname == "" {
		delete(reqBody, "hostname")
	}
	if ip == "" {
		delete(reqBody, "ip")
	}

	// Marshal JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	url := fmt.Sprintf("%s/api/v1/hosts", server)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var hostResp HostRegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&hostResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &hostResp, nil
}

// listHosts calls the API to list all hosts.
func listHosts(server string) ([]HostRegistrationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/hosts", server)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Hosts []HostRegistrationResponse `json:"hosts"`
		Total int                        `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Hosts, nil
}

// CLIConfig represents the CLI configuration file.
type CLIConfig struct {
	Server   string `json:"server"`
	APIKey   string `json:"api_key"`
	HostID   int64  `json:"host_id"`
	HostName string `json:"host_name"`
}

// saveConfig saves the CLI configuration to a file.
func saveConfig(path string, config *CLIConfig) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	// Write config file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Set file permissions (readable only by owner)
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// loadConfig loads the CLI configuration from a file.
func loadConfig(path string) (*CLIConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var config CLIConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}
