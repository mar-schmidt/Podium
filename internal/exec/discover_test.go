package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindHonoursEnvOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BIN", bin)

	found, err := Discovery{}.Find("claude")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Path != bin {
		t.Errorf("Find path = %q, want %q", found.Path, bin)
	}
}

func TestFindMissingBinaryErrors(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "")
	// A name that won't exist on PATH or npm dirs.
	_, err := Discovery{ExtraDirs: []string{t.TempDir()}}.Find("definitely-not-a-real-cli-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}
