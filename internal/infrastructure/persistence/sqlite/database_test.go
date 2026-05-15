package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/dotcommander/glog/internal/constants"
)

func TestNewWithOptionsAppliesDurabilityProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{name: "strict", profile: constants.DurabilityStrict, want: "FULL"},
		{name: "balanced", profile: constants.DurabilityBalanced, want: "NORMAL"},
		{name: "fast", profile: constants.DurabilityFast, want: "OFF"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, err := NewWithOptions(filepath.Join(t.TempDir(), "glog.db"), Options{DurabilityProfile: tt.profile})
			if err != nil {
				t.Fatalf("NewWithOptions failed: %v", err)
			}
			defer db.Close()

			stats, err := db.GetStats()
			if err != nil {
				t.Fatalf("GetStats failed: %v", err)
			}
			if stats.DurabilityProfile != tt.profile {
				t.Fatalf("profile mismatch: got %q want %q", stats.DurabilityProfile, tt.profile)
			}
			if stats.SynchronousMode != tt.want {
				t.Fatalf("synchronous mismatch: got %q want %q", stats.SynchronousMode, tt.want)
			}
			if !stats.WALMode {
				t.Fatal("expected WAL mode to be enabled")
			}
		})
	}
}

func TestNewWithOptionsRejectsInvalidDurabilityProfile(t *testing.T) {
	t.Parallel()

	if _, err := NewWithOptions(filepath.Join(t.TempDir(), "glog.db"), Options{DurabilityProfile: "reckless"}); err == nil {
		t.Fatal("expected invalid durability profile error")
	}
}
