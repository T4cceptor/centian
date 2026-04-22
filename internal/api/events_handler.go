package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/T4cceptor/centian/internal/persistence"
)

const (
	defaultEventPageLimit = 100
	maxEventPageLimit     = 200
)

// EventStore provides the read methods required by the global events API.
type EventStore interface {
	ListEvents(context.Context, *persistence.EventListFilter) (*persistence.EventListPage, error)
}

// EventsHandler serves the read-only global action-event API.
type EventsHandler struct {
	store EventStore
}

// NewEventsHandler builds an events API handler.
func NewEventsHandler(store EventStore) *EventsHandler {
	return &EventsHandler{store: store}
}

// RegisterRoutesWithMiddleware registers event routes with optional middleware.
func (h *EventsHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	if h == nil || h.store == nil || mux == nil {
		return
	}

	register := func(pattern string, handler http.HandlerFunc) {
		var wrapped http.Handler = handler
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
		mux.Handle(pattern, wrapped)
	}

	register("GET /api/events", h.handleListEvents)
}

func (h *EventsHandler) handleListEvents(w http.ResponseWriter, r *http.Request) {
	filter, err := eventFiltersFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.store.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	if page == nil {
		page = &persistence.EventListPage{}
	}
	if page.Items == nil {
		page.Items = []persistence.EventListItem{}
	}
	writeJSON(w, http.StatusOK, page)
}

func eventFiltersFromQuery(r *http.Request) (*persistence.EventListFilter, error) {
	query := r.URL.Query()

	filter := &persistence.EventListFilter{
		Gateway:     strings.TrimSpace(query.Get("gateway")),
		ServerName:  strings.TrimSpace(query.Get("server")),
		ToolName:    strings.TrimSpace(query.Get("tool")),
		Direction:   strings.TrimSpace(query.Get("direction")),
		MessageType: strings.TrimSpace(query.Get("messageType")),
		RequestID:   strings.TrimSpace(query.Get("requestId")),
		SessionID:   strings.TrimSpace(query.Get("sessionId")),
		Limit:       defaultEventPageLimit,
	}

	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 || limit > maxEventPageLimit {
			return nil, fmt.Errorf("invalid limit")
		}
		filter.Limit = limit
	}

	if rawSuccess := strings.TrimSpace(query.Get("success")); rawSuccess != "" {
		success, err := strconv.ParseBool(rawSuccess)
		if err != nil {
			return nil, fmt.Errorf("invalid success")
		}
		filter.Success = &success
	}

	cursor, hasCursor, err := persistence.ParseEventCursor(query.Get("cursor"))
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	if hasCursor {
		filter.Cursor = &cursor
	}

	return filter, nil
}
