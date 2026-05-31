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

// ProjectSummaryFilter filters project metadata for the current request.
type ProjectSummaryFilter func(*http.Request, ProjectSummary) bool

// ProjectsHandler serves configured project metadata.
type ProjectsHandler struct {
	projects []ProjectSummary
	filter   ProjectSummaryFilter
}

// NewProjectsHandler builds a project metadata API handler.
func NewProjectsHandler(projects []ProjectSummary) *ProjectsHandler {
	return &ProjectsHandler{projects: append([]ProjectSummary{}, projects...)}
}

// WithFilter returns the handler with a request-scoped project filter attached.
func (h *ProjectsHandler) WithFilter(filter ProjectSummaryFilter) *ProjectsHandler {
	if h == nil {
		return nil
	}
	h.filter = filter
	return h
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

func (h *ProjectsHandler) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects := make([]ProjectSummary, 0, len(h.projects))
	for _, project := range h.projects {
		if h.filter != nil && !h.filter(r, project) {
			continue
		}
		projects = append(projects, project)
	}
	writeJSON(w, http.StatusOK, projectsResponse{
		Projects: projects,
	})
}
