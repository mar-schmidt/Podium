package server

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/mar-schmidt/Podium/internal/updater"
)

type updateApplyRequest struct {
	Version string `json:"version,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !localRequest(r) {
		http.Error(w, "update checks are only available from loopback clients", http.StatusForbidden)
		return
	}
	status, err := updater.Check(r.Context(), updater.Options{
		CurrentVersion: s.build.Version,
		CurrentCommit:  s.build.Commit,
		Home:           s.paths.Home,
	})
	writeJSON(w, status, err)
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !localRequest(r) {
		http.Error(w, "updates are only available from loopback clients", http.StatusForbidden)
		return
	}
	var req updateApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := updater.Apply(r.Context(), updater.Options{
		CurrentVersion: s.build.Version,
		CurrentCommit:  s.build.Commit,
		Version:        req.Version,
		Force:          req.Force,
		Home:           s.paths.Home,
		RestartDaemon:  true,
	})
	if err != nil {
		writeJSON(w, result, err)
		return
	}
	writeJSON(w, result, nil)
	if result.RestartRequired || result.HelperStarted {
		go s.exitAfterUpdate()
	}
}

func (s *Server) exitAfterUpdate() {
	time.Sleep(300 * time.Millisecond)
	if runtime.GOOS != "windows" {
		if installDir, err := updater.ResolveInstallDir(""); err == nil {
			_ = updater.ScheduleUnixDaemonRestart(installDir)
		}
	}
	os.Exit(0)
}

func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
