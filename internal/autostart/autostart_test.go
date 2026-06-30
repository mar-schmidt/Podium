package autostart

import (
	"strings"
	"testing"
)

func TestRenderPlistContainsRequiredKeys(t *testing.T) {
	out := string(renderPlist(Options{PodiumdPath: "/usr/local/bin/podiumd"}, "/Users/test"))
	for _, want := range []string{
		"<key>Label</key><string>com.podium.podiumd</string>",
		"<string>/usr/local/bin/podiumd</string>",
		"<key>RunAtLoad</key><true/>",
		"<key>KeepAlive</key><true/>",
		"/Users/test/Library/Logs/podiumd.log",
		"/Users/test/Library/Logs/podiumd.err.log",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plist missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "EnvironmentVariables") {
		t.Errorf("plist should omit EnvironmentVariables when PodiumHome is empty:\n%s", out)
	}
}

func TestRenderPlistIncludesPodiumHomeWhenSet(t *testing.T) {
	out := string(renderPlist(Options{PodiumdPath: "/bin/podiumd", PodiumHome: "/data/podium"}, "/home/x"))
	if !strings.Contains(out, "<key>EnvironmentVariables</key><dict><key>PODIUM_HOME</key><string>/data/podium</string></dict>") {
		t.Errorf("plist missing PODIUM_HOME env dict:\n%s", out)
	}
}

func TestRenderUnitContent(t *testing.T) {
	out := renderUnit(Options{PodiumdPath: "/opt/podiumd"})
	for _, want := range []string{
		"Description=Podium daemon",
		"ExecStart=/opt/podiumd",
		"Restart=always",
		"WantedBy=default.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("unit missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Environment=PODIUM_HOME") {
		t.Errorf("unit should omit PODIUM_HOME when unset:\n%s", out)
	}
}

func TestRenderUnitIncludesPodiumHomeWhenSet(t *testing.T) {
	out := renderUnit(Options{PodiumdPath: "/opt/podiumd", PodiumHome: "/srv/podium"})
	if !strings.Contains(out, "Environment=PODIUM_HOME=/srv/podium") {
		t.Errorf("unit missing PODIUM_HOME line:\n%s", out)
	}
}
