package server

import (
	"context"
	"testing"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
)

func TestPermissionBrokerDecision(t *testing.T) {
	b := newPermissionBroker()
	requests, unsubscribe := b.subscribe("turn-1")
	defer unsubscribe()

	done := make(chan adapter.PermissionDecision, 1)
	go func() {
		decision, _ := b.RequestPermission(context.Background(), adapter.PermissionRequest{
			ID:       "req-1",
			TurnID:   "turn-1",
			ToolName: "Bash",
		}, time.Second)
		done <- decision
	}()

	req := <-requests
	if req.ID != "req-1" || req.ToolName != "Bash" {
		t.Fatalf("bad relayed request: %+v", req)
	}
	if !b.decide("req-1", adapter.PermissionDecision{Behavior: "allow"}) {
		t.Fatalf("decision was not accepted")
	}
	decision := <-done
	if decision.Behavior != "allow" {
		t.Fatalf("bad decision: %+v", decision)
	}
}

func TestPermissionBrokerTimeoutAutoDenies(t *testing.T) {
	b := newPermissionBroker()
	decision, err := b.RequestPermission(context.Background(), adapter.PermissionRequest{
		ID:     "req-1",
		TurnID: "turn-1",
	}, time.Nanosecond)
	if err != errPermissionTimeout {
		t.Fatalf("expected timeout, got %v", err)
	}
	if decision.Behavior != "deny" {
		t.Fatalf("expected auto deny, got %+v", decision)
	}
}
