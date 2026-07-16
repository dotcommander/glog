package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestVersionFlagOutput(t *testing.T) {
	var stdout bytes.Buffer
	parser := kong.Must(&cli{},
		kong.Name("glog"),
		kong.Vars{"version": "v0.4.1"},
		kong.Writers(&stdout, &bytes.Buffer{}),
		kong.Exit(func(code int) {
			panic(exitCode(code))
		}),
	)

	assertExits(t, 0, func() {
		if _, err := parser.Parse([]string{"--version"}); err != nil {
			t.Fatalf("parse --version: %v", err)
		}
	})
	if got, want := stdout.String(), "glog version v0.4.1\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type exitCode int

func assertExits(t *testing.T, want int, f func()) {
	t.Helper()
	defer func() {
		got, ok := recover().(exitCode)
		if !ok {
			t.Fatalf("function did not exit")
		}
		if int(got) != want {
			t.Fatalf("exit code = %d, want %d", got, want)
		}
	}()
	f()
}

func TestCompletionCommand(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{shell: shellBash, want: "complete -F _glog_completion glog"},
		{shell: "zsh", want: "#compdef glog"},
		{shell: "fish", want: "complete -c glog"},
		{shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var stdout bytes.Buffer
			parser := kong.Must(&cli{}, kong.Name("glog"), kong.Writers(&stdout, &bytes.Buffer{}))
			ctx, err := parser.Parse([]string{"completion", tt.shell})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := ctx.Run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			output := stdout.String()
			if strings.Contains(output, `" --addr`) {
				t.Error("completion output starts with an empty candidate")
			}
			for _, want := range []string{tt.want, "completion", "--help", "--version"} {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q", want)
				}
			}
		})
	}
}

func TestCompletionNoDescriptionsCompatibility(t *testing.T) {
	var stdout bytes.Buffer
	parser := kong.Must(&cli{}, kong.Name("glog"), kong.Writers(&stdout, &bytes.Buffer{}))
	ctx, err := parser.Parse([]string{"completion", shellBash, "--no-descriptions"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "complete -F _glog_completion glog") {
		t.Fatal("completion script missing bash registration")
	}
}
