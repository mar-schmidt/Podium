package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfirmAgentDeletionRequiresExactName(t *testing.T) {
	var out bytes.Buffer
	confirmation, ok := confirmAgentDeletion(strings.NewReader("wrong\n"), &out, "atlas")
	if ok {
		t.Fatal("expected mismatched confirmation to abort")
	}
	if confirmation != "wrong" {
		t.Fatalf("confirmation = %q, want wrong", confirmation)
	}
	if !strings.Contains(out.String(), "agent deletion aborted") {
		t.Fatalf("missing abort message: %q", out.String())
	}

	out.Reset()
	confirmation, ok = confirmAgentDeletion(strings.NewReader("atlas\n"), &out, "atlas")
	if !ok {
		t.Fatal("expected exact confirmation to proceed")
	}
	if confirmation != "atlas" {
		t.Fatalf("confirmation = %q, want atlas", confirmation)
	}
	if strings.Contains(out.String(), "agent deletion aborted") {
		t.Fatalf("unexpected abort message: %q", out.String())
	}
}

func TestActiveLogPathUsesPodiumHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PODIUM_HOME", home)
	got, err := activeLogPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "logs", "podiumd.log")
	if got != want {
		t.Fatalf("activeLogPath() = %q, want %q", got, want)
	}
}
