package notify

import (
	"context"
	"errors"
	"testing"
)

type fakeChannel struct {
	name string
	got  []Notification
	err  error
}

func (f *fakeChannel) Name() string { return f.name }
func (f *fakeChannel) Send(_ context.Context, n Notification) error {
	f.got = append(f.got, n)
	return f.err
}

func TestDispatcherFansOutToAllChannels(t *testing.T) {
	a := &fakeChannel{name: "a"}
	b := &fakeChannel{name: "b"}
	d := NewDispatcher(nil, a, nil, b) // nil channel must be dropped, not panic

	n := Notification{SessionID: "s1", AgentName: "jared", Title: "t", Body: "b", Kind: "permission"}
	d.Notify(context.Background(), n)

	if len(a.got) != 1 || len(b.got) != 1 {
		t.Fatalf("expected both channels to receive 1 notification, got a=%d b=%d", len(a.got), len(b.got))
	}
	if a.got[0].SessionID != "s1" || a.got[0].Kind != "permission" {
		t.Fatalf("channel received wrong notification: %+v", a.got[0])
	}
}

func TestDispatcherIsolatesChannelFailures(t *testing.T) {
	failing := &fakeChannel{name: "boom", err: errors.New("down")}
	ok := &fakeChannel{name: "ok"}
	d := NewDispatcher(nil, failing, ok)

	// A failing channel must not prevent later channels from being tried.
	d.Notify(context.Background(), Notification{Kind: "question"})

	if len(ok.got) != 1 {
		t.Fatalf("healthy channel should still receive despite earlier failure, got %d", len(ok.got))
	}
}
