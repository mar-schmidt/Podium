package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLedgerCreateAndListRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	if list, err := l.List(); err != nil || len(list) != 0 {
		t.Fatalf("empty ledger: list=%v err=%v", list, err)
	}

	created, err := l.Create(Project{ID: "mission-control", Name: "Mission Control", Stack: []string{"Next.js"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Path != "mission-control" || created.Status != "active" {
		t.Fatalf("defaults not applied: %+v", created)
	}
	// Project directory is scaffolded.
	if _, err := os.Stat(filepath.Join(dir, "mission-control")); err != nil {
		t.Fatalf("project dir not created: %v", err)
	}
	// projects.yaml is written and reloads.
	reloaded := New(dir)
	list, err := reloaded.List()
	if err != nil || len(list) != 1 || list[0].ID != "mission-control" {
		t.Fatalf("reload failed: list=%+v err=%v", list, err)
	}
}

func TestLedgerRejectsDuplicateAndBadID(t *testing.T) {
	l := New(t.TempDir())
	if _, err := l.Create(Project{ID: "ok"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := l.Create(Project{ID: "ok"}); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if _, err := l.Create(Project{ID: "../escape"}); err == nil {
		t.Fatal("expected invalid id error")
	}
}
