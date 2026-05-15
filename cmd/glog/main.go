package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/dotcommander/glog/cmd/glog/commands"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time
	Version = "dev"
	// BuildTime is set at build time
	BuildTime = "unknown"
)

func main() {
	// Fallback to build info when not set via ldflags
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}

	// Pass version info to commands package
	commands.Version = Version
	commands.BuildTime = BuildTime

	rootCmd := &cobra.Command{
		Use:     "glog",
		Short:   "Local-first log utility, dashboard, and agent debugging workspace",
		Long:    "A local-first log utility with a live dashboard, SQLite storage, and agent debugging workflows",
		Version: Version,
	}

	rootCmd.AddCommand(
		commands.ServeCmd(),
		commands.MigrateCmd(),
		commands.HostCmd(),
		commands.InspectCmd(),
		commands.InboxCmd(),
		commands.DoctorCmd(),
		commands.LogCmd(),
		commands.TailCmd(),
		commands.VersionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
