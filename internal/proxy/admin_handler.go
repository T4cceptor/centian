package proxy

import (
	"encoding/json"
	"net/http"
)

type adminReloadResponse struct {
	Status           string `json:"status"`
	GatewaysReloaded int    `json:"gatewaysReloaded,omitempty"`
	Message          string `json:"message"`
}

// handleAdminReload handles POST /admin/reload.
// It reloads gateway configuration from the provider and recreates all gateway endpoints.
// Requires a valid API key with no gateway scope restriction (full-access key).
func (c *CentianServer) handleAdminReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := c.ReloadGateways(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		resp := adminReloadResponse{
			Status:  "error",
			Message: err.Error(),
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	c.reloadMu.RLock()
	count := len(c.Gateways)
	c.reloadMu.RUnlock()
	resp := adminReloadResponse{
		Status:           "ok",
		GatewaysReloaded: count,
		Message:          "Gateway configuration reloaded successfully",
	}
	_ = json.NewEncoder(w).Encode(resp)
}
