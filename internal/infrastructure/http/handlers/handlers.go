// Package handlers provides HTTP handlers for the GLog API.
package handlers

import (
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/dotcommander/glog/internal/domain/services"
	"github.com/dotcommander/glog/internal/infrastructure/metrics"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

// Handlers holds all HTTP handler dependencies.
type Handlers struct {
	hostRepo       ports.HostRepository
	logRepo        ports.LogRepository
	inboxRepo      ports.InboxRepository
	hub            *sse.Hub
	patternMatcher *services.PatternMatcher
	recorder       *metrics.Recorder
}

// NewHandlers creates a new Handlers instance with all dependencies.
func NewHandlers(hostRepo ports.HostRepository, logRepo ports.LogRepository, hub *sse.Hub) *Handlers {
	return &Handlers{
		hostRepo:       hostRepo,
		logRepo:        logRepo,
		hub:            hub,
		patternMatcher: services.NewPatternMatcher(),
	}
}

// SetInboxRepo attaches the optional agent inbox repository.
func (h *Handlers) SetInboxRepo(inboxRepo ports.InboxRepository) {
	h.inboxRepo = inboxRepo
}

// SetRecorder attaches an internal metrics recorder.
func (h *Handlers) SetRecorder(recorder *metrics.Recorder) {
	h.recorder = recorder
}

// HostRepo returns the host repository.
func (h *Handlers) HostRepo() ports.HostRepository {
	return h.hostRepo
}

// LogRepo returns the log repository.
func (h *Handlers) LogRepo() ports.LogRepository {
	return h.logRepo
}

// InboxRepo returns the inbox repository.
func (h *Handlers) InboxRepo() ports.InboxRepository {
	return h.inboxRepo
}

// Hub returns the SSE hub.
func (h *Handlers) Hub() *sse.Hub {
	return h.hub
}

// PatternMatcher returns the pattern matcher service.
func (h *Handlers) PatternMatcher() *services.PatternMatcher {
	return h.patternMatcher
}

// Recorder returns the internal metrics recorder.
func (h *Handlers) Recorder() *metrics.Recorder {
	return h.recorder
}

// BroadcastLogCreated broadcasts a log created event to all SSE clients.
func (h *Handlers) BroadcastLogCreated(log LogResponse) {
	event := sse.Event{
		Type: "log.created",
		Data: log,
		ID:   "",
	}
	h.hub.Broadcast(event)
}

// BroadcastHostRegistered broadcasts a host registered event to all SSE clients.
func (h *Handlers) BroadcastHostRegistered(host HostResponse) {
	event := sse.Event{
		Type: "host.registered",
		Data: host,
		ID:   "",
	}
	h.hub.Broadcast(event)
}

// BroadcastHostUpdated broadcasts a host updated event to all SSE clients.
func (h *Handlers) BroadcastHostUpdated(host HostResponse) {
	event := sse.Event{
		Type: "host.updated",
		Data: host,
		ID:   "",
	}
	h.hub.Broadcast(event)
}
