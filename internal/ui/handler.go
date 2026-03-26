package ui

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFiles embed.FS

//go:embed fallback/index.html
var fallbackFiles embed.FS

// Handler serves the embedded frontend SPA.
type Handler struct {
	distFS       fs.FS
	fallbackHTML []byte
}

// NewHandler creates a UI handler backed by embedded assets.
func NewHandler() *Handler {
	fallbackHTML, err := fs.ReadFile(fallbackFiles, "fallback/index.html")
	if err != nil {
		fallbackHTML = []byte("<!doctype html><title>Centian UI unavailable</title>")
	}
	return newHandler(distFiles, fallbackHTML)
}

func newHandler(distFS fs.FS, fallbackHTML []byte) *Handler {
	return &Handler{
		distFS:       distFS,
		fallbackHTML: fallbackHTML,
	}
}

// RegisterRoutes registers the SPA handler under /ui.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.Handle("GET /ui", h)
	mux.Handle("GET /ui/", h)
}

// ServeHTTP serves embedded assets or the SPA entrypoint.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trimmedPath := strings.TrimPrefix(r.URL.Path, "/ui")
	if trimmedPath == "" || trimmedPath == "/" {
		h.serveIndex(w, r)
		return
	}

	cleanPath := path.Clean("/" + trimmedPath)
	if cleanPath == "/" {
		h.serveIndex(w, r)
		return
	}

	distPath := strings.TrimPrefix(cleanPath, "/")
	if file, ok := h.readStaticFile(distPath); ok {
		h.serveStaticFile(w, r, distPath, file)
		return
	}

	if strings.HasPrefix(distPath, "assets/") {
		http.NotFound(w, r)
		return
	}

	h.serveIndex(w, r)
}

func (h *Handler) readStaticFile(name string) ([]byte, bool) {
	if h == nil || h.distFS == nil {
		return nil, false
	}
	stat, err := fs.Stat(h.distFS, path.Join("dist", name))
	if err != nil || stat.IsDir() {
		return nil, false
	}
	file, err := fs.ReadFile(h.distFS, path.Join("dist", name))
	if err != nil {
		return nil, false
	}
	return file, true
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index := h.fallbackHTML
	if file, ok := h.readStaticFile("index.html"); ok {
		index = file
	}
	h.serveStaticFile(w, r, "index.html", index)
}

func (h *Handler) serveStaticFile(w http.ResponseWriter, r *http.Request, name string, contents []byte) {
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(contents)
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(contents))
}
