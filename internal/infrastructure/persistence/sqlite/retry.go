package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  constants.MaxRetries,
		BaseBackoff: constants.RetryBaseDelay,
	}
}

// WithRetry executes an operation with retry logic for transient SQLite errors.
// Uses exponential backoff: baseBackoff * 2^attempt (e.g., 10ms, 20ms, 40ms).
func WithRetry[T any](ctx context.Context, config RetryConfig, op func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for i := 0; i < config.MaxRetries; i++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		result, lastErr = op()
		if lastErr == nil {
			return result, nil
		}

		// Only retry on transient errors
		if !IsTransientError(lastErr) {
			return result, lastErr
		}

		// Don't sleep after the last attempt
		if i < config.MaxRetries-1 {
			backoff := config.BaseBackoff * (1 << i)
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return result, lastErr
}

// WithRetryNoResult executes an operation with no return value with retry logic.
func WithRetryNoResult(ctx context.Context, config RetryConfig, op func() error) error {
	_, err := WithRetry(ctx, config, func() (struct{}, error) {
		return struct{}{}, op()
	})
	return err
}

// IsTransientError checks if an error is a transient SQLite error that can be retried.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "locked") ||
		strings.Contains(errStr, "busy") ||
		strings.Contains(errStr, "SQLITE_BUSY")
}
