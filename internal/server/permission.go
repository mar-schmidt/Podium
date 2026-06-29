package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
)

var errPermissionTimeout = errors.New("permission request timed out")

type permissionBroker struct {
	mu      sync.Mutex
	turns   map[string]chan adapter.PermissionRequest
	pending map[string]chan adapter.PermissionDecision
}

func newPermissionBroker() *permissionBroker {
	return &permissionBroker{
		turns:   map[string]chan adapter.PermissionRequest{},
		pending: map[string]chan adapter.PermissionDecision{},
	}
}

func (b *permissionBroker) subscribe(turnID string) (<-chan adapter.PermissionRequest, func()) {
	ch := make(chan adapter.PermissionRequest, 8)
	b.mu.Lock()
	b.turns[turnID] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.turns, turnID)
		close(ch)
		b.mu.Unlock()
	}
}

func (b *permissionBroker) RequestPermission(ctx context.Context, req adapter.PermissionRequest, timeout time.Duration) (adapter.PermissionDecision, error) {
	decisionCh := make(chan adapter.PermissionDecision, 1)
	b.mu.Lock()
	b.pending[req.ID] = decisionCh
	turnCh := b.turns[req.TurnID]
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, req.ID)
		b.mu.Unlock()
	}()

	if turnCh != nil {
		select {
		case <-ctx.Done():
			return adapter.PermissionDecision{Behavior: "deny"}, ctx.Err()
		case turnCh <- req:
		default:
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return adapter.PermissionDecision{Behavior: "deny"}, ctx.Err()
	case decision := <-decisionCh:
		if decision.Behavior == "" {
			decision.Behavior = "deny"
		}
		return decision, nil
	case <-timer.C:
		return adapter.PermissionDecision{Behavior: "deny"}, errPermissionTimeout
	}
}

func (b *permissionBroker) decide(id string, decision adapter.PermissionDecision) bool {
	b.mu.Lock()
	ch := b.pending[id]
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}
