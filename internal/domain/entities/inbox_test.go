package entities

import "testing"

func TestInboxItemValidateDefaults(t *testing.T) {
	t.Parallel()

	item := NewInboxItem("  Leave handoff  ")
	item.Status = ""
	item.Kind = ""
	item.Priority = ""
	item.Source = ""
	item.Tags = nil
	item.LinkedLogIDs = nil
	item.LinkedFingerprints = nil

	if err := item.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if item.Title != "Leave handoff" {
		t.Fatalf("Title = %q; want trimmed title", item.Title)
	}
	if item.Status != InboxStatusOpen {
		t.Fatalf("Status = %q; want %q", item.Status, InboxStatusOpen)
	}
	if item.Kind != InboxKindTask {
		t.Fatalf("Kind = %q; want %q", item.Kind, InboxKindTask)
	}
	if item.Priority != InboxPriorityNormal {
		t.Fatalf("Priority = %q; want %q", item.Priority, InboxPriorityNormal)
	}
	if item.Source != "agent" {
		t.Fatalf("Source = %q; want agent", item.Source)
	}
	if item.Tags == nil || item.LinkedLogIDs == nil || item.LinkedFingerprints == nil {
		t.Fatal("Validate() should initialize nil slices")
	}
}

func TestInboxItemValidateRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item InboxItem
	}{
		{
			name: "empty title",
			item: InboxItem{Status: InboxStatusOpen, Kind: InboxKindTask, Priority: InboxPriorityNormal},
		},
		{
			name: "invalid status",
			item: InboxItem{Title: "x", Status: "waiting", Kind: InboxKindTask, Priority: InboxPriorityNormal},
		},
		{
			name: "invalid kind",
			item: InboxItem{Title: "x", Status: InboxStatusOpen, Kind: "memo", Priority: InboxPriorityNormal},
		},
		{
			name: "invalid priority",
			item: InboxItem{Title: "x", Status: InboxStatusOpen, Kind: InboxKindTask, Priority: "urgent"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.item.Validate(); err == nil {
				t.Fatal("Validate() error = nil; want error")
			}
		})
	}
}
