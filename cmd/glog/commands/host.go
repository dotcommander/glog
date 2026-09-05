package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const defaultConfigPath = "./glog.json"

// resolveServer resolves the server URL from flag, env var, or config file.
func resolveServer(server, configPath string) (string, error) {
	if server == "" {
		server = os.Getenv("GLOG_SERVER")
	}
	if server == "" {
		if configPath == "" {
			configPath = defaultConfigPath
		}
		if config, err := loadConfig(configPath); err == nil {
			server = config.Server
		}
	}
	if server == "" {
		return "", fmt.Errorf("server is required (provide --server or set GLOG_SERVER)")
	}
	return normalizeServerURL(server), nil
}

// resolveAPIKey resolves the API key from flag, env var, or config file.
func resolveAPIKey(apiKey, configPath string) (string, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GLOG_API_KEY")
	}
	if apiKey == "" {
		if configPath == "" {
			configPath = defaultConfigPath
		}
		if config, err := loadConfig(configPath); err == nil {
			apiKey = config.APIKey
		}
	}
	if apiKey == "" {
		return "", fmt.Errorf("API key is required (provide --api-key or run 'glog host register' first)")
	}
	return apiKey, nil
}

func normalizeServerURL(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

type HostCommand struct {
	Register HostRegisterCommand `cmd:"" help:"Register a new host with the GLog server."`
	List     HostListCommand     `cmd:"" help:"List all registered hosts."`
}

type HostRegisterCommand struct {
	Server      string   `help:"GLog server URL (e.g., https://logs.example.com)."`
	Name        string   `required:"" help:"Host name (must be unique)."`
	Tags        []string `name:"tag" help:"Tags for categorizing the host."`
	Description string   `help:"Optional description of the host."`
	Config      string   `default:"./glog.json" type:"path" help:"Path to save the configuration file."`
	Hostname    string   `help:"Actual hostname of the machine."`
	IP          string   `help:"IP address of the host."`
}

func (c *HostRegisterCommand) Run() error {
	server, err := resolveServer(c.Server, c.Config)
	if err != nil {
		return err
	}

	fmt.Printf("Registering host '%s' with server %s...\n", c.Name, server)

	// Call API to register host
	host, err := registerHost(server, c.Name, c.Tags, c.Description, c.Hostname, c.IP)
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

	if err := saveConfig(c.Config, config); err != nil {
		fmt.Printf("\n⚠ Warning: Failed to save config: %v\n", err)
		fmt.Printf("Please save the API key above manually.\n")
	} else {
		fmt.Printf("\n✓ Configuration saved to: %s\n", c.Config)
		fmt.Printf("  You can now use 'glog log' to send logs without specifying --server or --api-key\n")
	}

	return nil
}

type HostListCommand struct {
	Server string `help:"GLog server URL."`
	Config string `default:"./glog.json" type:"path" help:"Path to the configuration file."`
}

func (c *HostListCommand) Run() error {
	server, err := resolveServer(c.Server, c.Config)
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
	var hostResp HostRegistrationResponse
	if err := doJSON(req, http.StatusCreated, "registration failed", &hostResp); err != nil {
		return nil, err
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
	var result struct {
		Hosts []HostRegistrationResponse `json:"hosts"`
		Total int                        `json:"total"`
	}
	if err := doJSON(req, http.StatusOK, "request failed", &result); err != nil {
		return nil, err
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
