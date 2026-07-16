package http

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStaticFileHandler(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("dashboard")},
		"assets/app.js": &fstest.MapFile{Data: []byte("javascript")},
	}
	handler := staticFileHandler(webFS)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "index", path: "/", want: "dashboard"},
		{name: "asset", path: "/assets/app.js", want: "javascript"},
		{name: "spa fallback", path: "/inbox/item", want: "dashboard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Body.String(); got != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStaticFileHandlerWithoutIndexReturnsNotFound(t *testing.T) {
	handler := staticFileHandler(fstest.MapFS{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestConfiguredWebFS(t *testing.T) {
	embedded := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("embedded")}}
	diskDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(diskDir, "index.html"), []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := configuredWebFS(&Config{WebDir: diskDir, WebFS: embedded})
	data, err := fs.ReadFile(got, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "disk" {
		t.Errorf("configuredWebFS() = %q, want disk override", data)
	}
}
