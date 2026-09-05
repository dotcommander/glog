package commands

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dotcommander/glog/internal/constants"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (r *closeTracker) Close() error {
	r.closed = true
	return nil
}

func useHTTPClient(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := httpClient
	httpClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		httpClient = previous
	})
}

func TestDoJSONSuccess(t *testing.T) {
	body := &closeTracker{Reader: strings.NewReader(`{"id": 42}`)}
	useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusCreated, Body: body}, nil
	}))

	req, err := http.NewRequest(http.MethodPost, "http://glog.test/api/v1/logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID int `json:"id"`
	}
	if err := doJSON(req, http.StatusCreated, "request failed", &result); err != nil {
		t.Fatalf("doJSON returned error: %v", err)
	}
	if result.ID != 42 {
		t.Errorf("decoded ID = %d, want 42", result.ID)
	}
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestDoJSONUnexpectedStatusUsesBoundedBody(t *testing.T) {
	body := &closeTracker{Reader: strings.NewReader(strings.Repeat("x", constants.MaxErrorBodyBytes+1))}
	useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: body}, nil
	}))

	req, err := http.NewRequest(http.MethodGet, "http://glog.test/api/v1/hosts", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = doJSON(req, http.StatusOK, "request failed", &struct{}{})
	want := "request failed (status 502): " + strings.Repeat("x", constants.MaxErrorBodyBytes) + "... (truncated)"
	if err == nil || err.Error() != want {
		t.Errorf("doJSON error = %v, want %q", err, want)
	}
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestDoJSONMalformedJSON(t *testing.T) {
	body := &closeTracker{Reader: bytes.NewBufferString(`{"id":`)}
	useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	}))

	req, err := http.NewRequest(http.MethodGet, "http://glog.test/api/v1/hosts", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = doJSON(req, http.StatusOK, "request failed", &struct{}{})
	if err == nil || !strings.HasPrefix(err.Error(), "failed to decode response: ") {
		t.Errorf("doJSON error = %v, want JSON decode error", err)
	}
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestDoJSONTransportError(t *testing.T) {
	transportErr := errors.New("unreachable")
	useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, transportErr
	}))

	req, err := http.NewRequest(http.MethodGet, "http://glog.test/api/v1/hosts", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = doJSON(req, http.StatusOK, "request failed", &struct{}{})
	if err == nil || !strings.HasPrefix(err.Error(), "failed to send request: ") || !errors.Is(err, transportErr) {
		t.Errorf("doJSON error = %v, want wrapped transport error", err)
	}
}

func TestJSONCallersPreserveRequestContracts(t *testing.T) {
	t.Run("register host", func(t *testing.T) {
		useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost || req.URL.Path != "/api/v1/hosts" {
				t.Errorf("request = %s %s, want POST /api/v1/hosts", req.Method, req.URL.Path)
			}
			if got := req.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			data, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte(`"name":"node-1"`)) || bytes.Contains(data, []byte(`"description"`)) {
				t.Errorf("request body = %s, want name and omitted empty description", data)
			}
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"id":7,"name":"node-1"}`))}, nil
		}))

		host, err := registerHost("http://glog.test", "node-1", []string{"prod"}, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if host.ID != 7 || host.Name != "node-1" {
			t.Errorf("host = %+v, want decoded registration", host)
		}
	})

	t.Run("send log", func(t *testing.T) {
		useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost || req.URL.Path != "/api/v1/logs" {
				t.Errorf("request = %s %s, want POST /api/v1/logs", req.Method, req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("Authorization = %q, want Bearer secret", got)
			}
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"id":11,"success":true}`))}, nil
		}))

		id, err := sendLog("http://glog.test", "secret", "info", "hello", nil)
		if err != nil {
			t.Fatal(err)
		}
		if id != 11 {
			t.Errorf("log ID = %d, want 11", id)
		}
	})

	t.Run("list inbox", func(t *testing.T) {
		useHTTPClient(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet || req.URL.Path != "/api/v1/inbox" {
				t.Errorf("request = %s %s, want GET /api/v1/inbox", req.Method, req.URL.Path)
			}
			if got := req.URL.Query().Get("status"); got != "open" {
				t.Errorf("status query = %q, want open", got)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":13,"title":"task"}]}`))}, nil
		}))

		items, err := listInboxItems("http://glog.test", "open", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != 13 {
			t.Errorf("items = %+v, want decoded item 13", items)
		}
	})
}
