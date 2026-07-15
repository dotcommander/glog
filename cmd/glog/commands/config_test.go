package commands

import (
	"strings"
	"testing"
)

func TestResolveServerUsesDefaultConfigAndNormalizesURL(t *testing.T) {
	t.Setenv("GLOG_SERVER", "")
	t.Setenv("GLOG_API_KEY", "")
	t.Chdir(t.TempDir())

	if err := saveConfig(defaultConfigPath, &CLIConfig{
		Server: " http://localhost:6016/ ",
		APIKey: "glog_v1_test",
	}); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}

	server, err := resolveServer("", "")
	if err != nil {
		t.Fatalf("resolveServer() error = %v", err)
	}
	if server != "http://localhost:6016" {
		t.Fatalf("server = %q; want normalized default config URL", server)
	}

	apiKey, err := resolveAPIKey("", "")
	if err != nil {
		t.Fatalf("resolveAPIKey() error = %v", err)
	}
	if apiKey != "glog_v1_test" {
		t.Fatalf("apiKey = %q; want config API key", apiKey)
	}
}

func TestReadResponseBodySnippetTruncatesLargeErrors(t *testing.T) {
	t.Parallel()

	got := readResponseBodySnippet(strings.NewReader(strings.Repeat("x", 70*1024)))
	if !strings.HasSuffix(got, "... (truncated)") {
		t.Fatalf("snippet suffix = %q; want truncation marker", got[len(got)-32:])
	}
	if len(got) > 66*1024 {
		t.Fatalf("snippet len = %d; want bounded output", len(got))
	}
}
