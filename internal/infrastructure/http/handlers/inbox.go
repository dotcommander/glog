package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/go-chi/chi/v5"
)

type InboxItemRequest struct {
	Title              string   `json:"title"`
	Body               string   `json:"body,omitempty"`
	Status             string   `json:"status,omitempty"`
	Kind               string   `json:"kind,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	Source             string   `json:"source,omitempty"`
	Repo               string   `json:"repo,omitempty"`
	SessionID          string   `json:"session_id,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	LinkedLogIDs       []string `json:"linked_log_ids,omitempty"`
	LinkedFingerprints []string `json:"linked_fingerprints,omitempty"`
	CreatedBy          string   `json:"created_by,omitempty"`
	ClaimedBy          string   `json:"claimed_by,omitempty"`
	DueAt              string   `json:"due_at,omitempty"`
}

func ListInboxItemsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		filters := parseInboxFilters(r)
		items, total, err := repo.List(filters)
		if err != nil {
			http.Error(w, "Failed to fetch inbox items: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []*entities.InboxItem{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":  items,
			"total":  total,
			"limit":  filters.Limit,
			"offset": filters.Offset,
		})
	}
}

func GetInboxItemHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		id, ok := parseInboxID(w, r)
		if !ok {
			return
		}
		item, err := repo.FindByID(id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Inbox item not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func CreateInboxItemHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		req, ok := decodeInboxItemRequest(w, r)
		if !ok {
			return
		}
		item, err := inboxRequestToItem(req, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := repo.Create(item); err != nil {
			http.Error(w, "Failed to create inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func UpdateInboxItemHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		id, ok := parseInboxID(w, r)
		if !ok {
			return
		}
		existing, err := repo.FindByID(id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Inbox item not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req, ok := decodeInboxItemRequest(w, r)
		if !ok {
			return
		}
		item, err := inboxRequestToItem(req, existing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := repo.Update(item); err != nil {
			http.Error(w, "Failed to update inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func CompleteInboxItemHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		id, ok := parseInboxID(w, r)
		if !ok {
			return
		}
		item, err := repo.FindByID(id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Inbox item not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		item.Status = entities.InboxStatusDone
		if err := repo.Update(item); err != nil {
			http.Error(w, "Failed to complete inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func DeleteInboxItemHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := h.requireInboxRepo(w)
		if !ok {
			return
		}
		id, ok := parseInboxID(w, r)
		if !ok {
			return
		}
		if err := repo.Delete(id); err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Inbox item not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to delete inbox item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handlers) requireInboxRepo(w http.ResponseWriter) (ports.InboxRepository, bool) {
	if h.inboxRepo == nil {
		http.Error(w, "Inbox is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.inboxRepo, true
}

func parseInboxFilters(r *http.Request) ports.InboxFilters {
	filters := ports.InboxFilters{
		Status:    r.URL.Query().Get("status"),
		Kind:      r.URL.Query().Get("kind"),
		Repo:      r.URL.Query().Get("repo"),
		SessionID: r.URL.Query().Get("session_id"),
		Source:    r.URL.Query().Get("source"),
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			filters.Limit = n
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if n, err := strconv.Atoi(offset); err == nil {
			filters.Offset = n
		}
	}
	filters.Limit, filters.Offset = constants.NormalizePagination(filters.Limit, filters.Offset)
	return filters
}

func parseInboxID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid inbox item ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func decodeInboxItemRequest(w http.ResponseWriter, r *http.Request) (InboxItemRequest, bool) {
	var req InboxItemRequest
	return req, decodeJSONRequest(w, r, &req)
}

func inboxRequestToItem(req InboxItemRequest, existing *entities.InboxItem) (*entities.InboxItem, error) {
	item := entities.NewInboxItem(req.Title)
	if existing != nil {
		item = existing
		if req.Title != "" {
			item.Title = req.Title
		}
	}
	if req.Body != "" || existing == nil {
		item.Body = req.Body
	}
	if req.Status != "" {
		item.Status = entities.InboxStatus(req.Status)
	}
	if req.Kind != "" {
		item.Kind = entities.InboxKind(req.Kind)
	}
	if req.Priority != "" {
		item.Priority = entities.InboxPriority(req.Priority)
	}
	if req.Source != "" {
		item.Source = req.Source
	}
	if req.Repo != "" || existing == nil {
		item.Repo = req.Repo
	}
	if req.SessionID != "" || existing == nil {
		item.SessionID = req.SessionID
	}
	if req.Tags != nil {
		item.Tags = entities.HostTags(req.Tags)
	}
	if req.LinkedLogIDs != nil {
		item.LinkedLogIDs = entities.HostTags(req.LinkedLogIDs)
	}
	if req.LinkedFingerprints != nil {
		item.LinkedFingerprints = entities.HostTags(req.LinkedFingerprints)
	}
	if req.CreatedBy != "" || existing == nil {
		item.CreatedBy = req.CreatedBy
	}
	if req.ClaimedBy != "" || existing == nil {
		item.ClaimedBy = req.ClaimedBy
	}
	if req.DueAt != "" {
		dueAt, err := time.Parse(time.RFC3339, req.DueAt)
		if err != nil {
			return nil, err
		}
		item.DueAt = &dueAt
	}
	return item, item.Validate()
}
