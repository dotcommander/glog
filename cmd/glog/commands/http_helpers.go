package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dotcommander/glog/internal/constants"
)

func readResponseBodySnippet(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, constants.MaxErrorBodyBytes+1))
	if err != nil {
		return err.Error()
	}
	if len(data) > constants.MaxErrorBodyBytes {
		return string(data[:constants.MaxErrorBodyBytes]) + "... (truncated)"
	}
	return string(data)
}

// doJSON executes a JSON API request and decodes a successful response.
func doJSON(req *http.Request, expectedStatus int, statusError string, result any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("%s (status %d): %s", statusError, resp.StatusCode, readResponseBodySnippet(resp.Body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
