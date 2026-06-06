package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

type stubEventStore struct {
	listEventsFn func(context.Context, *persistence.EventListFilter) (*persistence.EventListPage, error)
	calls        int
	filter       *persistence.EventListFilter
}

func (s *stubEventStore) ListEvents(ctx context.Context, filter *persistence.EventListFilter) (*persistence.EventListPage, error) {
	s.calls++
	s.filter = filter
	if s.listEventsFn == nil {
		return nil, nil
	}
	return s.listEventsFn(ctx, filter)
}

func TestEventsHandler_ListEvents(t *testing.T) {
	t.Run("returns empty items instead of null", func(t *testing.T) {
		handler := NewEventsHandler(&stubEventStore{})
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)

		var page persistence.EventListPage
		err := json.Unmarshal(rec.Body.Bytes(), &page)
		assert.NilError(t, err)
		assert.Assert(t, page.Items != nil)
		assert.Equal(t, len(page.Items), 0)
	})

	t.Run("passes filters through query params", func(t *testing.T) {
		store := &stubEventStore{
			listEventsFn: func(context.Context, *persistence.EventListFilter) (*persistence.EventListPage, error) {
				return &persistence.EventListPage{Items: []persistence.EventListItem{}}, nil
			},
		}
		handler := NewEventsHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		cursor := persistence.EventListCursor{
			CreatedAtUnixMilli: 1_234,
			ID:                 identifiers.New(identifiers.KindActionEvent),
		}.Encode()

		req := httptest.NewRequest(
			http.MethodGet,
			"/api/events?gateway=gw&server=server-a&tool=shell__exec&direction=[CLIENT%20-%3E%20SERVER]&messageType=request&success=true&withGovernanceEvent=true&requestId=req-1&sessionId=sid-1&cursor="+cursor+"&limit=25",
			http.NoBody,
		)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Assert(t, store.filter != nil)
		assert.Equal(t, store.filter.Gateway, "gw")
		assert.Equal(t, store.filter.ServerName, "server-a")
		assert.Equal(t, store.filter.ToolName, "shell__exec")
		assert.Equal(t, store.filter.Direction, "[CLIENT -> SERVER]")
		assert.Equal(t, store.filter.MessageType, "request")
		assert.Equal(t, store.filter.RequestID, "req-1")
		assert.Equal(t, store.filter.SessionID, "sid-1")
		assert.Equal(t, store.filter.Limit, 25)
		assert.Equal(t, store.filter.WithGovernanceEvent, true)
		assert.Assert(t, store.filter.Success != nil)
		assert.Equal(t, *store.filter.Success, true)
		assert.Assert(t, store.filter.Cursor != nil)
		assert.Equal(t, store.filter.Cursor.CreatedAtUnixMilli, int64(1_234))
	})

	t.Run("returns next cursor when more rows exist", func(t *testing.T) {
		handler := NewEventsHandler(&stubEventStore{
			listEventsFn: func(context.Context, *persistence.EventListFilter) (*persistence.EventListPage, error) {
				return &persistence.EventListPage{
					Items: []persistence.EventListItem{{
						ID:                 identifiers.New(identifiers.KindActionEvent),
						CreatedAtUnixMilli: 1_000,
						ToolName:           "shell__exec",
						Success:            true,
						Annotations: []common.EventAnnotation{{
							Processor: "prompt_injection_guard",
							Action:    "blocked",
						}},
					}},
					NextCursor: "cursor-2",
				}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)

		var page persistence.EventListPage
		err := json.Unmarshal(rec.Body.Bytes(), &page)
		assert.NilError(t, err)
		assert.Equal(t, page.NextCursor, "cursor-2")
		assert.Equal(t, len(page.Items), 1)
		assert.Equal(t, page.Items[0].Annotations[0].Processor, "prompt_injection_guard")
		assert.Equal(t, page.Items[0].Annotations[0].Action, "blocked")
	})

	t.Run("rejects invalid success values", func(t *testing.T) {
		store := &stubEventStore{}
		handler := NewEventsHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events?success=maybe", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})

	t.Run("rejects invalid governance filter values", func(t *testing.T) {
		store := &stubEventStore{}
		handler := NewEventsHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events?withGovernanceEvent=maybe", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})

	t.Run("rejects invalid limits", func(t *testing.T) {
		for _, rawLimit := range []string{"0", "201", "oops"} {
			store := &stubEventStore{}
			handler := NewEventsHandler(store)
			mux := http.NewServeMux()
			handler.RegisterRoutesWithMiddleware(mux, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/events?limit="+rawLimit, http.NoBody)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, rec.Code, http.StatusBadRequest)
			assert.Equal(t, store.calls, 0)
		}
	})

	t.Run("rejects invalid cursors", func(t *testing.T) {
		store := &stubEventStore{}
		handler := NewEventsHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events?cursor=Zm9v", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})
}
