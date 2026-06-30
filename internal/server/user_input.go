package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
)

var errUserInputTimeout = errors.New("user input request timed out")

type userInputBroker struct {
	mu      sync.Mutex
	turns   map[string]chan adapter.UserInputRequest
	pending map[string]chan adapter.UserInputDecision
}

func newUserInputBroker() *userInputBroker {
	return &userInputBroker{
		turns:   map[string]chan adapter.UserInputRequest{},
		pending: map[string]chan adapter.UserInputDecision{},
	}
}

func (b *userInputBroker) subscribe(turnID string) (<-chan adapter.UserInputRequest, func()) {
	ch := make(chan adapter.UserInputRequest, 8)
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

func (b *userInputBroker) RequestUserInput(ctx context.Context, req adapter.UserInputRequest, timeout time.Duration) (adapter.UserInputDecision, error) {
	if timeout <= 0 {
		timeout = defaultHTTPPermissionTimeout
	}
	decisionCh := make(chan adapter.UserInputDecision, 1)
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
			return adapter.UserInputDecision{}, ctx.Err()
		case turnCh <- req:
		default:
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return adapter.UserInputDecision{}, ctx.Err()
	case decision := <-decisionCh:
		if decision.Answers == nil {
			decision.Answers = map[string][]string{}
		}
		return decision, nil
	case <-timer.C:
		return adapter.UserInputDecision{}, errUserInputTimeout
	}
}

func (b *userInputBroker) decide(id string, decision adapter.UserInputDecision) bool {
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
