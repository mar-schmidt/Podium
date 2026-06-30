// Package autostart configures Podium's daemon (podiumd) to start on login.
// It mirrors the logic that used to live in scripts/install.sh so the onboarding
// wizard can offer autostart as its final step.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNoSystemd is returned on Linux when systemd --user isn't available.
var ErrNoSystemd = errors.New("systemd --user is not available")

// ErrUnsupported is returned on platforms without an autostart mechanism.
var ErrUnsupported = errors.New("autostart is not supported on this platform")

// Options configures the autostart install.
type Options struct {
	// PodiumdPath is the absolute path to the podiumd binary. Required.
	PodiumdPath string
	// PodiumHome optionally pins the storage root via PODIUM_HOME. Empty = unset.
	PodiumHome string
}

// Install configures podiumd to launch on login for the current user.
func Install(opts Options) error {
	if opts.PodiumdPath == "" {
		return errors.New("podiumd path is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return installDarwin(opts)
	case "linux":
		return installLinux(opts)
	default:
		return ErrUnsupported
	}
}

func installDarwin(opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.podium.podiumd.plist")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, renderPlist(opts, home), 0o644); err != nil {
		return err
	}
	// Reload so changes take effect; ignore unload errors (it may not be loaded).
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, string(out))
	}
	return nil
}

func installLinux(opts Options) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return ErrNoSystemd
	}
	if err := exec.Command("systemctl", "--user", "show-environment").Run(); err != nil {
		return ErrNoSystemd
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", "podium.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(renderUnit(opts)), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w: %s", err, string(out))
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "podium.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %w: %s", err, string(out))
	}
	return nil
}

// renderPlist builds the launchd plist. Kept side-effect free for testing.
func renderPlist(opts Options, home string) []byte {
	env := ""
	if opts.PodiumHome != "" {
		env = fmt.Sprintf("  <key>EnvironmentVariables</key><dict><key>PODIUM_HOME</key><string>%s</string></dict>\n", opts.PodiumHome)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.podium.podiumd</string>
  <key>ProgramArguments</key><array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
%s  <key>StandardOutPath</key><string>%s/Library/Logs/podiumd.log</string>
  <key>StandardErrorPath</key><string>%s/Library/Logs/podiumd.err.log</string>
</dict></plist>
`, opts.PodiumdPath, env, home, home)
	return []byte(plist)
}

// renderUnit builds the systemd user unit. Kept side-effect free for testing.
func renderUnit(opts Options) string {
	env := ""
	if opts.PodiumHome != "" {
		env = fmt.Sprintf("Environment=PODIUM_HOME=%s\n", opts.PodiumHome)
	}
	return fmt.Sprintf(`[Unit]
Description=Podium daemon

[Service]
ExecStart=%s
%sRestart=always

[Install]
WantedBy=default.target
`, opts.PodiumdPath, env)
}
