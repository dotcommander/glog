package commands

import (
	"io"

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
