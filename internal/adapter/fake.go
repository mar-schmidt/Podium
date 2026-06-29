package adapter

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a deterministic in-memory adapter for tests and local core
// development. It never shells out or spends tokens.
type Fake struct {
	mu               sync.Mutex
	started          map[string]Handle
	Responses        []string
	Requests         []TurnRequest
	RateLimitedTurns int
}

// NewFake returns a fake adapter with the default echo-style response script.
func NewFake() *Fake {
	return &Fake{started: map[string]Handle{}}
}

// Start records a new backing handle for a session.
func (f *Fake) Start(ctx context.Context, req StartRequest) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	handle := Handle{Provider: req.Provider, ID: fmt.Sprintf("fake-%s", req.SessionID)}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.started == nil {
		f.started = map[string]Handle{}
	}
	f.started[req.SessionID] = handle
	return handle, nil
}

// Resume returns the supplied handle unchanged.
func (f *Fake) Resume(ctx context.Context, req ResumeRequest) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.started == nil {
		f.started = map[string]Handle{}
	}
	f.started[req.SessionID] = req.Handle
	return req.Handle, nil
}

// SendTurn streams a final assistant message and a turn-done marker.
func (f *Fake) SendTurn(ctx context.Context, req TurnRequest) (<-chan Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rateLimited, response := f.nextResult(req)
	ch := make(chan Event, 2)
	go func() {
		defer close(ch)
		if rateLimited {
			select {
			case <-ctx.Done():
				return
			case ch <- Event{Kind: EventRateLimited, Content: "fake rate limit"}:
			}
			select {
			case <-ctx.Done():
				return
			case ch <- Event{Kind: EventTurnDone}:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case ch <- Event{Kind: EventAssistantMessage, Content: response}:
		}
		select {
		case <-ctx.Done():
			return
		case ch <- Event{Kind: EventTurnDone}:
		}
	}()
	return ch, nil
}

// Teardown forgets any stored fake handle.
func (f *Fake) Teardown(ctx context.Context, handle Handle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for sessionID, h := range f.started {
		if h == handle {
			delete(f.started, sessionID)
		}
	}
	return nil
}

func (f *Fake) nextResult(req TurnRequest) (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, req)
	if f.RateLimitedTurns > 0 {
		f.RateLimitedTurns--
		return true, ""
	}
	if len(f.Responses) > 0 {
		response := f.Responses[0]
		f.Responses = f.Responses[1:]
		return false, response
	}
	return false, fmt.Sprintf("fake response: %s", req.Message)
}
