package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/assert"
)

func TestProjectsHandlerListsConfiguredProjects(t *testing.T) {
	handler := NewProjectsHandler([]ProjectSummary{
		{
			Slug:                    "default",
			Name:                    "default",
			IsDefault:               true,
			UIEnabled:               true,
			EventStorageEnabled:     true,
			TaskVerificationEnabled: true,
		},
		{
			Slug:                "research",
			Name:                "Research",
			Description:         "Research project",
			UIEnabled:           true,
			EventStorageEnabled: true,
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var response struct {
		Projects []ProjectSummary `json:"projects"`
	}
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, len(response.Projects), 2)
	assert.Equal(t, response.Projects[0].Slug, "default")
	assert.Equal(t, response.Projects[0].IsDefault, true)
	assert.Equal(t, response.Projects[1].Slug, "research")
	assert.Equal(t, response.Projects[1].Description, "Research project")
}
