package api

import "net/http"

// ProjectSummary describes one configured project exposed to the UI.
type ProjectSummary struct {
	Slug                    string `json:"slug"`
	Name                    string `json:"name"`
	Description             string `json:"description,omitempty"`
	IsDefault               bool   `json:"isDefault"`
	UIEnabled               bool   `json:"uiEnabled"`
	EventStorageEnabled     bool   `json:"eventStorageEnabled"`
	TaskVerificationEnabled bool   `json:"taskVerificationEnabled"`
}

type projectsResponse struct {
	Projects []ProjectSummary `json:"projects"`
}

// ProjectsHandler serves configured project metadata.
type ProjectsHandler struct {
	projects []ProjectSummary
}

// NewProjectsHandler builds a project metadata API handler.
func NewProjectsHandler(projects []ProjectSummary) *ProjectsHandler {
	return &ProjectsHandler{projects: append([]ProjectSummary{}, projects...)}
}

// RegisterRoutesWithMiddleware registers project metadata routes.
func (h *ProjectsHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	if h == nil || mux == nil {
		return
	}
	var wrapped http.Handler = http.HandlerFunc(h.handleListProjects)
	if middleware != nil {
		wrapped = middleware(wrapped)
	}
	mux.Handle("GET /api/projects", wrapped)
}

func (h *ProjectsHandler) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, projectsResponse{
		Projects: append([]ProjectSummary{}, h.projects...),
	})
}
