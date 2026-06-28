package http

import (
	"net/http"
)

// NewRouter sets up the multiplexer routing table using Go 1.22+ pattern matching.
func NewRouter(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", h.GetDashboard)
	mux.HandleFunc("GET /status-table", h.GetStatusTable)
	mux.HandleFunc("GET /api/v1/status", h.GetAPIStatus)
	mux.HandleFunc("POST /hosts", h.AddHost)
	mux.HandleFunc("DELETE /hosts", h.DeleteHost)
	mux.HandleFunc("GET /targets", h.GetTargetsView)
	mux.HandleFunc("GET /groups", h.GetGroupsView)
	mux.HandleFunc("POST /groups", h.AddGroup)
	mux.HandleFunc("DELETE /groups", h.DeleteGroup)
	mux.HandleFunc("GET /healthz", h.GetHealthz)
	mux.HandleFunc("GET /readyz", h.GetReadyz)

	return mux
}

