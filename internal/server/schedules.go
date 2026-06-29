package server

import (
	"net/http"
	"strings"
)

// handleSchedules lists all schedules with their next-run times and recent run
// history (R7.5).
func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses, err := s.scheduler.List(r.Context())
	writeJSON(w, statuses, err)
}

// handleSchedule handles per-schedule actions under /api/schedules/<name>/...
// Currently: POST /api/schedules/<name>/run triggers a manual run.
func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	name, action, _ := strings.Cut(rest, "/")
	if name == "" {
		http.Error(w, "schedule name is required", http.StatusBadRequest)
		return
	}
	switch action {
	case "run":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		run, err := s.scheduler.RunNow(r.Context(), name)
		writeJSON(w, run, err)
	default:
		http.Error(w, "unknown schedule action", http.StatusNotFound)
	}
}
