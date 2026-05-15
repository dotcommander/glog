package entities

import (
	"fmt"
	"strings"
	"time"
)

type InboxStatus string

const (
	InboxStatusOpen       InboxStatus = "open"
	InboxStatusInProgress InboxStatus = "in_progress"
	InboxStatusBlocked    InboxStatus = "blocked"
	InboxStatusDone       InboxStatus = "done"
	InboxStatusDismissed  InboxStatus = "dismissed"
)

type InboxKind string

const (
	InboxKindNote     InboxKind = "note"
	InboxKindTask     InboxKind = "task"
	InboxKindQuestion InboxKind = "question"
	InboxKindBug      InboxKind = "bug"
	InboxKindHandoff  InboxKind = "handoff"
)

type InboxPriority string

const (
	InboxPriorityLow    InboxPriority = "low"
	InboxPriorityNormal InboxPriority = "normal"
	InboxPriorityHigh   InboxPriority = "high"
)

type InboxItem struct {
	ID                 int64         `json:"id"`
	Title              string        `json:"title"`
	Body               string        `json:"body,omitempty"`
	Status             InboxStatus   `json:"status"`
	Kind               InboxKind     `json:"kind"`
	Priority           InboxPriority `json:"priority"`
	Source             string        `json:"source,omitempty"`
	Repo               string        `json:"repo,omitempty"`
	SessionID          string        `json:"session_id,omitempty"`
	Tags               HostTags      `json:"tags"`
	LinkedLogIDs       HostTags      `json:"linked_log_ids"`
	LinkedFingerprints HostTags      `json:"linked_fingerprints"`
	CreatedBy          string        `json:"created_by,omitempty"`
	ClaimedBy          string        `json:"claimed_by,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	DueAt              *time.Time    `json:"due_at,omitempty"`
}

func NewInboxItem(title string) *InboxItem {
	now := time.Now().UTC()
	return &InboxItem{
		Title:     strings.TrimSpace(title),
		Status:    InboxStatusOpen,
		Kind:      InboxKindTask,
		Priority:  InboxPriorityNormal,
		Source:    "agent",
		Tags:      HostTags{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func IsValidInboxStatus(status InboxStatus) bool {
	switch status {
	case InboxStatusOpen, InboxStatusInProgress, InboxStatusBlocked, InboxStatusDone, InboxStatusDismissed:
		return true
	default:
		return false
	}
}

func IsValidInboxKind(kind InboxKind) bool {
	switch kind {
	case InboxKindNote, InboxKindTask, InboxKindQuestion, InboxKindBug, InboxKindHandoff:
		return true
	default:
		return false
	}
}

func IsValidInboxPriority(priority InboxPriority) bool {
	switch priority {
	case InboxPriorityLow, InboxPriorityNormal, InboxPriorityHigh:
		return true
	default:
		return false
	}
}

func (i *InboxItem) Validate() error {
	i.Title = strings.TrimSpace(i.Title)
	if i.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if i.Status == "" {
		i.Status = InboxStatusOpen
	}
	if !IsValidInboxStatus(i.Status) {
		return fmt.Errorf("invalid status %q", i.Status)
	}
	if i.Kind == "" {
		i.Kind = InboxKindTask
	}
	if !IsValidInboxKind(i.Kind) {
		return fmt.Errorf("invalid kind %q", i.Kind)
	}
	if i.Priority == "" {
		i.Priority = InboxPriorityNormal
	}
	if !IsValidInboxPriority(i.Priority) {
		return fmt.Errorf("invalid priority %q", i.Priority)
	}
	if i.Source == "" {
		i.Source = "agent"
	}
	if i.Tags == nil {
		i.Tags = HostTags{}
	}
	if i.LinkedLogIDs == nil {
		i.LinkedLogIDs = HostTags{}
	}
	if i.LinkedFingerprints == nil {
		i.LinkedFingerprints = HostTags{}
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	i.UpdatedAt = time.Now().UTC()
	return nil
}
