package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DoctorCommand struct {
	Config string `default:"./glog.json" type:"path" help:"Path to CLI config file."`
	Server string `help:"GLog server URL to check."`
}

func (c *DoctorCommand) Run() error {

	fmt.Println("GLog doctor")
	fmt.Printf("Version: %s\n", Version)
	if BuildTime != "" && BuildTime != "unknown" {
		fmt.Printf("Build time: %s\n", BuildTime)
	}
	fmt.Println()

	checkPath()
	config, hasConfig := checkConfig(c.Config)
	server := c.Server
	if server == "" {
		server = os.Getenv("GLOG_SERVER")
	}
	if server == "" && hasConfig {
		server = config.Server
	}
	serverOK := checkServer(server)
	printFirstRunNextSteps(c.Config, hasConfig, server, serverOK)
	return nil
}

func checkPath() {
	current, err := os.Executable()
	if err != nil {
		fmt.Printf("Binary: unknown (%v)\n", err)
		return
	}
	current, _ = filepath.EvalSymlinks(current)
	found, err := exec.LookPath("glog")
	if err != nil {
		fmt.Printf("PATH: glog is not on PATH\n")
		fmt.Printf("Binary: %s\n", current)
		fmt.Println("Fix: move glog into a PATH directory, or run it as ./glog")
		return
	}
	found, _ = filepath.EvalSymlinks(found)
	if found == current {
		fmt.Printf("PATH: ok (%s)\n", found)
		return
	}
	fmt.Printf("PATH: warning, PATH resolves to %s\n", found)
	fmt.Printf("Current binary: %s\n", current)
	fmt.Println("Fix: remove the stale binary or put this one earlier in PATH")
}

func checkConfig(configPath string) (*CLIConfig, bool) {
	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Config: missing (%s)\n", configPath)
		return nil, false
	}
	fmt.Printf("Config: ok (%s)\n", configPath)
	fmt.Printf("Configured server: %s\n", config.Server)
	if config.HostName != "" {
		fmt.Printf("Configured host: %s\n", config.HostName)
	}
	return config, true
}

func checkServer(server string) bool {
	if server == "" {
		fmt.Println("Server: not checked (no --server, GLOG_SERVER, or config)")
		return false
	}
	url := strings.TrimRight(server, "/") + "/health"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Server: unreachable (%s)\n", server)
		fmt.Printf("Reason: %v\n", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Server: unhealthy (%s returned HTTP %d)\n", url, resp.StatusCode)
		return false
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		fmt.Printf("Server: reachable, but health response was not JSON (%v)\n", err)
		return false
	}
	if body.Status == "" {
		body.Status = "ok"
	}
	fmt.Printf("Server: %s (%s)\n", body.Status, server)
	return true
}

func printFirstRunNextSteps(configPath string, hasConfig bool, server string, serverOK bool) {
	fmt.Println()
	fmt.Println("Next steps:")
	if server == "" {
		fmt.Println("  1. Start GLog: glog serve")
		fmt.Println("  2. Register this machine: glog host register --server http://localhost:6016 --name my-machine")
		fmt.Println("  3. Send a test log: glog log \"hello from glog\"")
		fmt.Println("  4. Open dashboard: http://localhost:6016")
		return
	}
	if !serverOK {
		if strings.Contains(server, "localhost") || strings.Contains(server, "127.0.0.1") {
			fmt.Println("  1. Start GLog: glog serve")
		} else {
			fmt.Printf("  1. Check that the GLog server is running: %s/health\n", strings.TrimRight(server, "/"))
		}
		fmt.Println("  2. Re-run: glog doctor")
		return
	}
	if !hasConfig {
		fmt.Printf("  1. Register this machine: glog host register --server %s --name my-machine --config %s\n", server, configPath)
		fmt.Println("  2. Send a test log: glog log \"hello from glog\"")
		return
	}
	fmt.Println("  1. Send a test log: glog log \"hello from glog\"")
	fmt.Println("  2. Watch live logs: glog tail")
}
