package webui

import (
	"io/fs"
	"testing"
)

func TestFSContainsDashboard(t *testing.T) {
	dashboard, err := FS()
	if err != nil {
		t.Fatalf("FS() error = %v", err)
	}

	for _, path := range []string{"index.html", "_app/version.json"} {
		if _, err := fs.Stat(dashboard, path); err != nil {
			t.Errorf("embedded dashboard missing %q: %v", path, err)
		}
	}
}
