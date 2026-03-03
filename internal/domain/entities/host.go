package entities

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// HostStatus represents the current status of a host.
type HostStatus string

const (
	HostStatusOnline   HostStatus = "online"
	HostStatusOffline  HostStatus = "offline"
	HostStatusDegraded HostStatus = "degraded"
	HostStatusUnknown  HostStatus = "unknown"
)

// HostTags is a custom type for handling tag arrays.
type HostTags []string

// Scan implements sql.Scanner for HostTags.
func (ht *HostTags) Scan(value interface{}) error {
	if value == nil {
		*ht = HostTags{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("failed to unmarshal HostTags value")
	}

	// Handle empty string
	if len(bytes) == 0 {
		*ht = HostTags{}
		return nil
	}

	return json.Unmarshal(bytes, ht)
}

// Value implements driver.Valuer for HostTags.
func (ht HostTags) Value() (driver.Value, error) {
	if len(ht) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(ht)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// JSONMap is a flexible map[string]any for JSON storage.
type JSONMap map[string]any

// Scan implements sql.Scanner for JSONMap.
func (jm *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*jm = JSONMap{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to unmarshal JSONMap value: unsupported type %T", value)
	}

	// If empty string, treat as empty object
	if len(bytes) == 0 {
		*jm = JSONMap{}
		return nil
	}

	// Attempt to unmarshal
	err := json.Unmarshal(bytes, jm)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSONMap value: %w", err)
	}

	return nil
}

// Value implements driver.Valuer for JSONMap.
func (jm JSONMap) Value() (driver.Value, error) {
	if len(jm) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(jm)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Host represents a registered host that can send logs.
type Host struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	APIKey     string     `json:"-"` // Never expose in API responses
	Tags       HostTags   `json:"tags"`
	Status     HostStatus `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeen   time.Time  `json:"last_seen"`
	LastLogID  *int64     `json:"last_log_id,omitempty"` // Most recent log ID
	LogCount   int64      `json:"log_count"`             // Total logs from this host
	ErrorCount int64      `json:"error_count"`           // Error logs in last 24h
	ErrorRate  float64    `json:"error_rate"`            // Error rate (0-1)

	// Optional metadata
	Description string  `json:"description,omitempty"`
	Hostname    string  `json:"hostname,omitempty"`   // Actual hostname
	IP          string  `json:"ip,omitempty"`         // Last known IP
	UserAgent   string  `json:"user_agent,omitempty"` // Client identifier
	Metadata    JSONMap `json:"metadata,omitempty"`   // Custom key-value pairs
}

// NewHost creates a new host with a generated API key.
func NewHost(name string, tags []string) (*Host, error) {
	if name == "" {
		return nil, errors.New("host name cannot be empty")
	}

	if len(name) > 255 {
		return nil, errors.New("host name too long (max 255 characters)")
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	// Validate and sanitize tags
	sanitizedTags := make(HostTags, 0, len(tags))
	for _, tag := range tags {
		if tag = sanitizeTag(tag); tag != "" {
			sanitizedTags = append(sanitizedTags, tag)
		}
	}

	now := time.Now()

	return &Host{
		Name:      name,
		APIKey:    apiKey,
		Tags:      sanitizedTags,
		Status:    HostStatusUnknown,
		CreatedAt: now,
		LastSeen:  now,
	}, nil
}

// generateAPIKey creates a random API key with the format:
// glog_v1_<timestamp>_<random>
func generateAPIKey() (string, error) {
	// Generate 23 random bytes (184 bits of entropy) to get 46 hex chars
	// Total length: "glog_v1_" (8 chars) + 46 hex chars = 54 chars
	random := make([]byte, 23)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	// Format: glog_v1_<hex>
	// Example: glog_v1_00112233445566778899aabbccddeeff0011223344
	return "glog_v1_" + hex.EncodeToString(random), nil
}

// sanitizeTag validates and sanitizes a tag string.
func sanitizeTag(tag string) string {
	// Remove leading/trailing whitespace
	tag = strings.TrimSpace(tag)

	// Remove illegal characters (keep alphanumeric, hyphen, underscore)
	var sb strings.Builder
	for _, r := range tag {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}

	return sb.String()
}

// IsOnline returns true if the host has sent logs in the last 5 minutes.
func (h *Host) IsOnline() bool {
	return time.Since(h.LastSeen) < 5*time.Minute
}

// UpdateStatus recalculates host status based on last_seen and error rate.
func (h *Host) UpdateStatus() {
	now := time.Now()
	timeSinceLastSeen := now.Sub(h.LastSeen)

	// Offline if no logs in 10 minutes
	if timeSinceLastSeen > 10*time.Minute {
		h.Status = HostStatusOffline
		return
	}

	// Degraded if error rate > 10%
	if h.ErrorRate > 0.1 {
		h.Status = HostStatusDegraded
		return
	}

	// Online if logs in last 5 minutes
	if h.IsOnline() {
		h.Status = HostStatusOnline
		return
	}

	// Unknown otherwise
	h.Status = HostStatusUnknown
}

// GetSeverity returns the current status as a severity level for display.
func (h *Host) GetSeverity() string {
	switch h.Status {
	case HostStatusOnline:
		return "success"
	case HostStatusDegraded:
		return "warning"
	case HostStatusOffline:
		return "error"
	default:
		return "info"
	}
}

// GetColor returns the color associated with this host's status.
func (h *Host) GetColor() string {
	switch h.Status {
	case HostStatusOnline:
		return "#22c55e" // Green
	case HostStatusDegraded:
		return "#eab308" // Yellow
	case HostStatusOffline:
		return "#ef4444" // Red
	default:
		return "#6b7280" // Gray
	}
}
