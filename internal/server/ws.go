package server

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/store"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.core == nil {
		http.Error(w, "core unavailable", http.StatusServiceUnavailable)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()
	writer := &wsWriter{conn: conn}
	_ = writer.write(ctx, ServerMessage{Type: "hello"})
	_ = s.writeState(ctx, writer)

	incoming := make(chan ClientMessage, 16)
	errc := make(chan error, 1)
	go readWS(ctx, conn, incoming, errc)

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errc:
			if err != nil && websocket.CloseStatus(err) == -1 {
				_ = writer.write(ctx, ServerMessage{Type: "error", Error: err.Error()})
			}
			return
		case msg, ok := <-incoming:
			if !ok {
				return
			}
			if err := s.handleWSMessage(ctx, writer, msg); err != nil {
				_ = writer.write(ctx, ServerMessage{
					Type:      "error",
					RequestID: msg.RequestID,
					Error:     err.Error(),
				})
			}
		}
	}
}

type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsWriter) write(ctx context.Context, msg ServerMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return wsjson.Write(ctx, w.conn, msg)
}

func readWS(ctx context.Context, conn *websocket.Conn, out chan<- ClientMessage, errc chan<- error) {
	defer close(out)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			errc <- err
			return
		}
		msg, err := decodeClientMessage(data)
		if err != nil {
			errc <- err
			return
		}
		select {
		case <-ctx.Done():
			return
		case out <- msg:
		}
	}
}

func (s *Server) handleWSMessage(ctx context.Context, writer *wsWriter, msg ClientMessage) error {
	switch msg.Type {
	case "list":
		return s.writeState(ctx, writer)
	case "create_session":
		session, err := s.core.CreateSession(ctx, core.CreateSessionRequest{
			AgentName: msg.AgentName,
			Origin:    store.OriginWeb,
		})
		if err != nil {
			return err
		}
		if err := writer.write(ctx, ServerMessage{Type: "session", RequestID: msg.RequestID, Session: &session}); err != nil {
			return err
		}
		history, err := s.core.History(ctx, session.ID)
		if err != nil {
			return err
		}
		return writer.write(ctx, ServerMessage{Type: "history", RequestID: msg.RequestID, History: history})
	case "send_turn":
		go s.runWSTurn(ctx, writer, msg)
		return nil
	case "permission_decision":
		if msg.Decision == nil {
			return errors.New("permission decision is required")
		}
		if !s.broker.decide(msg.RequestID, *msg.Decision) {
			return errors.New("permission request not found")
		}
		return nil
	default:
		return errors.New("unknown websocket message type")
	}
}

func (s *Server) writeState(ctx context.Context, writer *wsWriter) error {
	agents, err := s.core.ListAgents(ctx)
	if err != nil {
		return err
	}
	sessions, err := s.core.ListSessions(ctx)
	if err != nil {
		return err
	}
	return writer.write(ctx, ServerMessage{Type: "state", Agents: agents, Sessions: sessions})
}

func (s *Server) runWSTurn(ctx context.Context, writer *wsWriter, msg ClientMessage) {
	var session store.Session
	var err error
	if msg.SessionID == "" {
		if msg.AgentName == "" {
			_ = writer.write(ctx, ServerMessage{Type: "error", RequestID: msg.RequestID, Error: "agent_name is required"})
			return
		}
		session, err = s.core.CreateSession(ctx, core.CreateSessionRequest{AgentName: msg.AgentName, Origin: store.OriginWeb})
	} else {
		session, err = s.core.GetSession(ctx, msg.SessionID)
	}
	if err != nil {
		_ = writer.write(ctx, ServerMessage{Type: "error", RequestID: msg.RequestID, Error: err.Error()})
		return
	}
	if err := writer.write(ctx, ServerMessage{Type: "session", RequestID: msg.RequestID, Session: &session}); err != nil {
		return
	}
	slash, err := s.core.HandleSlashCommand(ctx, session.ID, msg.Message)
	if err != nil {
		_ = writer.write(ctx, ServerMessage{Type: "error", RequestID: msg.RequestID, Error: err.Error()})
		return
	}
	if slash.Handled {
		_ = writer.write(ctx, ServerMessage{Type: "session", RequestID: msg.RequestID, Session: &slash.Session})
		_ = writer.write(ctx, ServerMessage{Type: "notice", RequestID: msg.RequestID, Notice: slash.Notice})
		_ = writer.write(ctx, ServerMessage{Type: "done", RequestID: msg.RequestID})
		_ = s.writeState(ctx, writer)
		return
	}

	turnID := uuid.NewString()
	requests, unsubscribe := s.broker.subscribe(turnID)
	defer unsubscribe()

	events, err := s.core.StreamTurn(ctx, session.ID, msg.Message, core.TurnOptions{
		PermissionTurnID: turnID,
		PermissionRelay:  s.broker,
	})
	if err != nil {
		_ = writer.write(ctx, ServerMessage{Type: "error", RequestID: msg.RequestID, Error: err.Error()})
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
			_ = writer.write(ctx, ServerMessage{Type: "permission_request", RequestID: msg.RequestID, Request: &request})
		case event, ok := <-events:
			if !ok {
				events = nil
				requests = nil
				continue
			}
			if err := writeTurnEvent(ctx, writer, msg.RequestID, event); err != nil {
				return
			}
		}
	}
	_ = writer.write(ctx, ServerMessage{Type: "done", RequestID: msg.RequestID})
	_ = s.writeState(ctx, writer)
}

func writeTurnEvent(ctx context.Context, writer *wsWriter, requestID string, event core.TurnEvent) error {
	switch event.Kind {
	case "message_stored":
		return writer.write(ctx, ServerMessage{Type: "message", RequestID: requestID, Message: event.Message})
	case adapter.EventAssistantDelta:
		return writer.write(ctx, ServerMessage{Type: "delta", RequestID: requestID, Delta: event.Content})
	case adapter.EventAssistantMessage:
		return writer.write(ctx, ServerMessage{Type: "assistant", RequestID: requestID, Delta: event.Content})
	case adapter.EventPermissionRequest:
		return writer.write(ctx, ServerMessage{Type: "permission_request", RequestID: requestID, Request: event.PermissionRequest})
	case adapter.EventTurnDone:
		return writer.write(ctx, ServerMessage{Type: "done", RequestID: requestID})
	case "error":
		return writer.write(ctx, ServerMessage{Type: "error", RequestID: requestID, Error: event.Content})
	default:
		return nil
	}
}
