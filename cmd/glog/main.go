package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"
	"github.com/dotcommander/glog/cmd/glog/commands"
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

	var cli struct {
		VersionFlag kong.VersionFlag        `name:"version" short:"v" help:"Print version information and quit."`
		Serve       commands.ServeCommand   `cmd:"" help:"Start the GLog server."`
		Migrate     commands.MigrateCommand `cmd:"" help:"Run database migrations."`
		Host        commands.HostCommand    `cmd:"" help:"Manage hosts."`
		Inspect     commands.InspectCommand `cmd:"" help:"Inspect GLog data files offline."`
		Inbox       commands.InboxCommand   `cmd:"" help:"Manage agent inbox notes and tasks."`
		Doctor      commands.DoctorCommand  `cmd:"" help:"Check first-run setup and print next steps."`
		Log         commands.LogCommand     `cmd:"" help:"Send logs to the GLog server."`
		Tail        commands.TailCommand    `cmd:"" help:"Stream live logs from the GLog server."`
		Version     commands.VersionCommand `cmd:"" help:"Print version information."`
	}
	ctx := kong.Parse(&cli,
		kong.Name("glog"),
		kong.Description("A local-first log utility with a live dashboard, SQLite storage, and agent debugging workflows."),
		kong.Vars{"version": Version},
		kong.UsageOnError(),
		kong.BindTo(context.Background(), (*context.Context)(nil)),
	)
	if err := ctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
