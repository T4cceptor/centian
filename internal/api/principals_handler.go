package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// PrincipalStore provides the distinct-principal lookup for the principals API.
type PrincipalStore interface {
	DistinctPrincipals(ctx context.Context, start, end int64) ([]string, error)
}

// PrincipalNamer resolves principal ids to human-readable display names. It is
// optional; a nil namer (or one returning an empty map) falls back to raw ids.
type PrincipalNamer interface {
	PrincipalNames(ctx context.Context) (map[string]string, error)
}

// PrincipalRef is one principal option for a filter dropdown.
type PrincipalRef struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type principalsResponse struct {
	Principals []PrincipalRef `json:"principals"`
}

// PrincipalsHandler serves the read-only list of principals seen in events,
// labeled with display names where available.
type PrincipalsHandler struct {
	store PrincipalStore
	namer PrincipalNamer
}

// NewPrincipalsHandler builds a principals API handler. namer may be nil.
func NewPrincipalsHandler(store PrincipalStore, namer PrincipalNamer) *PrincipalsHandler {
	return &PrincipalsHandler{store: store, namer: namer}
}

// RegisterRoutesWithMiddleware registers principal routes under the default /api prefix.
func (h *PrincipalsHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	h.RegisterRoutesWithPrefix(mux, "/api", middleware)
}

// RegisterRoutesWithPrefix registers principal routes under prefix.
func (h *PrincipalsHandler) RegisterRoutesWithPrefix(mux *http.ServeMux, prefix string, middleware func(http.Handler) http.Handler) {
	if h == nil || h.store == nil || mux == nil {
		return
	}
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		prefix = "/api"
	}

	register := func(pattern string, handler http.HandlerFunc) {
		var wrapped http.Handler = handler
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
		mux.Handle(pattern, wrapped)
	}

	register(fmt.Sprintf("GET %s/principals", prefix), h.handlePrincipals)
}

func (h *PrincipalsHandler) handlePrincipals(w http.ResponseWriter, r *http.Request) {
	start, end, err := optionalWindowFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := h.store.DistinctPrincipals(r.Context(), start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list principals")
		return
	}

	names := map[string]string{}
	if h.namer != nil {
		if resolved, nameErr := h.namer.PrincipalNames(r.Context()); nameErr == nil && resolved != nil {
			names = resolved
		}
	}

	principals := make([]PrincipalRef, 0, len(ids))
	for _, id := range ids {
		displayName := names[id]
		if displayName == "" {
			displayName = id
		}
		principals = append(principals, PrincipalRef{ID: id, DisplayName: displayName})
	}

	writeJSON(w, http.StatusOK, principalsResponse{Principals: principals})
}

// optionalWindowFromQuery parses optional numeric start/end (unix milli) bounds.
// Missing values yield 0, which the store treats as "all time".
func optionalWindowFromQuery(r *http.Request) (int64, int64, error) {
	query := r.URL.Query()
	start, err := parseOptionalInt(query.Get("start"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start")
	}
	end, err := parseOptionalInt(query.Get("end"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end")
	}
	return start, end, nil
}

func parseOptionalInt(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
