package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/store"
)

const defaultHTTPPermissionTimeout = 2 * time.Minute

type agentCreateRequest struct {
	Name           string                `json:"name"`
	Provider       config.Provider       `json:"provider,omitempty"`
	Profile        string                `json:"profile,omitempty"`
	Model          string                `json:"model,omitempty"`
	Effort         string                `json:"effort,omitempty"`
	PermissionMode config.PermissionMode `json:"permission_mode,omitempty"`
}

type sessionCreateRequest struct {
	AgentName string              `json:"agent_name"`
	Origin    store.SessionOrigin `json:"origin"`
}

type chatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Message   string `json:"message"`
}

type streamEvent struct {
	Type       string                     `json:"type"`
	Session    *store.Session             `json:"session,omitempty"`
	Message    *store.Message             `json:"message,omitempty"`
	Delta      string                     `json:"delta,omitempty"`
	Request    *adapter.PermissionRequest `json:"request,omitempty"`
	Error      string                     `json:"error,omitempty"`
	AutoDenied bool                       `json:"auto_denied,omitempty"`
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		agents, err := s.core.ListAgents(r.Context())
		writeJSON(w, agents, err)
	case http.MethodPost:
		var req agentCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		agent, err := s.core.CreateAgent(r.Context(), core.CreateAgentRequest{
			Name:           req.Name,
			Provider:       req.Provider,
			Profile:        req.Profile,
			Model:          req.Model,
			Effort:         req.Effort,
			PermissionMode: req.PermissionMode,
		})
		writeJSON(w, agent, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.core.ListSessions(r.Context())
		writeJSON(w, sessions, err)
	case http.MethodPost:
		var req sessionCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Origin == "" {
			req.Origin = store.OriginCLI
		}
		session, err := s.core.CreateSession(r.Context(), core.CreateSessionRequest{
			AgentName: req.AgentName,
			Origin:    req.Origin,
		})
		writeJSON(w, session, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var session store.Session
	var err error
	if req.SessionID == "" {
		if req.AgentName == "" {
			http.Error(w, "agent_name is required when session_id is omitted", http.StatusBadRequest)
			return
		}
		session, err = s.core.CreateSession(ctx, core.CreateSessionRequest{AgentName: req.AgentName, Origin: store.OriginCLI})
	} else {
		session, err = s.core.GetSession(ctx, req.SessionID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	writeStreamEvent(enc, flusher, streamEvent{Type: "session", Session: &session})

	turnID := uuid.NewString()
	requests, unsubscribe := s.broker.subscribe(turnID)
	subscribed := true
	defer func() {
		if subscribed {
			unsubscribe()
		}
	}()

	events, err := s.core.StreamTurn(ctx, session.ID, req.Message, core.TurnOptions{
		PermissionTurnID: turnID,
		PermissionRelay:  s.broker,
	})
	if err != nil {
		writeStreamEvent(enc, flusher, streamEvent{Type: "error", Error: err.Error()})
		return
	}

	for events != nil || requests != nil {
		select {
		case <-ctx.Done():
			return
		case request, ok := <-requests:
			if !ok {
				requests = nil
				continue
			}
			writeStreamEvent(enc, flusher, streamEvent{Type: "permission_request", Request: &request})
		case event, ok := <-events:
			if !ok {
				events = nil
				if subscribed {
					unsubscribe()
					subscribed = false
					requests = nil
				}
				continue
			}
			switch event.Kind {
			case "message_stored":
				writeStreamEvent(enc, flusher, streamEvent{Type: "message", Message: event.Message})
			case adapter.EventAssistantDelta:
				writeStreamEvent(enc, flusher, streamEvent{Type: "delta", Delta: event.Content})
			case adapter.EventAssistantMessage:
				writeStreamEvent(enc, flusher, streamEvent{Type: "assistant", Delta: event.Content})
			case adapter.EventPermissionRequest:
				writeStreamEvent(enc, flusher, streamEvent{Type: "permission_request", Request: event.PermissionRequest})
			case adapter.EventTurnDone:
				writeStreamEvent(enc, flusher, streamEvent{Type: "done"})
			case "error":
				writeStreamEvent(enc, flusher, streamEvent{Type: "error", Error: event.Content})
			}
		}
	}
	writeStreamEvent(enc, flusher, streamEvent{Type: "done"})
}

func (s *Server) handlePermissionDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/permission-decisions/")
	var decision adapter.PermissionDecision
	if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if decision.Behavior != "allow" && decision.Behavior != "deny" {
		http.Error(w, "behavior must be allow or deny", http.StatusBadRequest)
		return
	}
	if !s.broker.decide(id, decision) {
		http.Error(w, "permission request not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"}, nil)
}

func (s *Server) handlePermissionRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	turnID := strings.TrimPrefix(r.URL.Path, "/api/permissions/")
	var req adapter.PermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.TurnID = turnID
	if req.ID == "" {
		req.ID = uuid.NewString()
	}
	timeout := defaultHTTPPermissionTimeout
	if rawTimeout := r.URL.Query().Get("timeout"); rawTimeout != "" {
		if parsed, err := time.ParseDuration(rawTimeout); err == nil {
			timeout = parsed
		}
	}
	decision, err := s.broker.RequestPermission(r.Context(), req, timeout)
	if err != nil && err != errPermissionTimeout && err != context.Canceled {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, decision, nil)
}

func writeJSON(w http.ResponseWriter, value any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeStreamEvent(enc *json.Encoder, flusher http.Flusher, event streamEvent) {
	_ = enc.Encode(event)
	if flusher != nil {
		flusher.Flush()
	}
}
