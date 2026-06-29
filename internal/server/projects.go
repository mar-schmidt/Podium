package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/projects"
	"github.com/mar-schmidt/Podium/internal/store"
)

type projectCreateRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Stack       []string `json:"stack"`
	Notes       string   `json:"notes"`
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.core.ListProjects(r.Context())
		writeJSON(w, list, err)
	case http.MethodPost:
		var req projectCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		project, err := s.core.CreateProject(r.Context(), projects.Project{
			ID:          strings.TrimSpace(req.ID),
			Name:        strings.TrimSpace(req.Name),
			Description: req.Description,
			Stack:       req.Stack,
			Notes:       req.Notes,
		})
		writeJSON(w, project, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type taskCreateRequest struct {
	ProjectID     string `json:"project_id"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	AssignedAgent string `json:"assigned_agent"`
	Status        string `json:"status"`
	PickupAt      string `json:"pickup_at"`
}

type taskUpdateRequest struct {
	ProjectID     *string `json:"project_id,omitempty"`
	Title         *string `json:"title,omitempty"`
	Body          *string `json:"body,omitempty"`
	AssignedAgent *string `json:"assigned_agent,omitempty"`
	Status        *string `json:"status,omitempty"`
	PickupAt      *string `json:"pickup_at,omitempty"`
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		tasks, err := s.core.ListTasks(r.Context())
		writeJSON(w, tasks, err)
	case http.MethodPost:
		var req taskCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status := store.TaskStatus(req.Status)
		if status == "" {
			status = store.TaskBacklog
		}
		task, err := s.core.CreateTask(r.Context(), store.Task{
			ProjectID:     req.ProjectID,
			Title:         strings.TrimSpace(req.Title),
			Body:          req.Body,
			AssignedAgent: req.AssignedAgent,
			Status:        status,
			PickupAt:      req.PickupAt,
		})
		writeJSON(w, task, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTask handles /api/tasks/<id> (PATCH update) and /api/tasks/<id>/start.
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	id, action, _ := strings.Cut(rest, "/")
	if id == "" {
		http.Error(w, "task id is required", http.StatusBadRequest)
		return
	}

	if action == "start" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		session, err := s.core.StartTask(r.Context(), core.StartTaskRequest{TaskID: id})
		writeJSON(w, session, err)
		return
	}

	if action == "session" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		session, ok, err := s.core.TaskSession(r.Context(), id)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		if !ok {
			http.Error(w, "task has no session yet", http.StatusNotFound)
			return
		}
		writeJSON(w, session, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, err := s.core.GetTask(r.Context(), id)
		writeJSON(w, task, err)
	case http.MethodPatch, http.MethodPut:
		task, err := s.core.GetTask(r.Context(), id)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		var req taskUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		applyTaskUpdate(&task, req)
		updated, err := s.core.UpdateTask(r.Context(), task)
		writeJSON(w, updated, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func applyTaskUpdate(task *store.Task, req taskUpdateRequest) {
	if req.ProjectID != nil {
		task.ProjectID = *req.ProjectID
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Body != nil {
		task.Body = *req.Body
	}
	if req.AssignedAgent != nil {
		task.AssignedAgent = *req.AssignedAgent
	}
	if req.Status != nil {
		task.Status = store.TaskStatus(*req.Status)
	}
	if req.PickupAt != nil {
		task.PickupAt = *req.PickupAt
	}
}
