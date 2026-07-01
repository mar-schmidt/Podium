// Package github provides Podium's local GitHub App user-auth flow and archive
// snapshot download support. It intentionally does not use app private keys or
// client secrets because Podium is distributed as a local app.
package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/projects"
)

var ErrConfirmationRequired = errors.New("project directory has existing files; confirmation required")

type Service struct {
	cfg    config.GitHub
	home   string
	client *http.Client
}

type Options struct {
	Config config.GitHub
	Home   string
	Client *http.Client
}

func New(opts Options) *Service {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	cfg := opts.Config
	if cfg.WebBase == "" {
		cfg.WebBase = "https://github.com"
	}
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.github.com"
	}
	if cfg.LoginBase == "" {
		cfg.LoginBase = "https://github.com/login"
	}
	return &Service{cfg: cfg, home: opts.Home, client: client}
}

type Status struct {
	Configured bool   `json:"configured"`
	Authed     bool   `json:"authed"`
	AppSlug    string `json:"app_slug"`
	ClientID   string `json:"client_id,omitempty"`
	InstallURL string `json:"install_url,omitempty"`
	Message    string `json:"message,omitempty"`
}

func (s *Service) Status(ctx context.Context) Status {
	st := Status{
		Configured: s.cfg.AppSlug != "" && s.cfg.ClientID != "",
		AppSlug:    s.cfg.AppSlug,
		ClientID:   s.cfg.ClientID,
		InstallURL: s.installURL(),
	}
	if !st.Configured {
		st.Message = "GitHub is not configured. Add github.app_slug and github.client_id to config.yaml."
		return st
	}
	token, err := s.loadToken()
	if err != nil || token.AccessToken == "" {
		st.Message = "GitHub is not connected."
		return st
	}
	if err := s.checkToken(ctx, token.AccessToken); err != nil {
		st.Message = "GitHub token is unavailable or expired. Reconnect GitHub."
		return st
	}
	st.Authed = true
	st.Message = "GitHub is connected."
	return st
}

type DeviceStart struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func (s *Service) StartDevice(ctx context.Context) (DeviceStart, error) {
	if s.cfg.ClientID == "" {
		return DeviceStart{}, errors.New("github.client_id is not configured")
	}
	form := url.Values{"client_id": {s.cfg.ClientID}}
	var out DeviceStart
	if err := s.postForm(ctx, s.cfg.LoginBase+"/device/code", form, "", &out); err != nil {
		return DeviceStart{}, err
	}
	return out, nil
}

type DevicePollResult struct {
	Status      string `json:"status"`
	AccessToken string `json:"-"`
	Error       string `json:"error,omitempty"`
}

func (s *Service) PollDevice(ctx context.Context, deviceCode string) (DevicePollResult, error) {
	if s.cfg.ClientID == "" {
		return DevicePollResult{}, errors.New("github.client_id is not configured")
	}
	form := url.Values{
		"client_id":   {s.cfg.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := s.postForm(ctx, s.cfg.LoginBase+"/oauth/access_token", form, "", &raw); err != nil {
		return DevicePollResult{}, err
	}
	if raw.Error != "" {
		return DevicePollResult{Status: raw.Error, Error: raw.ErrorDesc}, nil
	}
	if raw.AccessToken == "" {
		return DevicePollResult{Status: "authorization_pending"}, nil
	}
	if err := s.saveToken(tokenFile{
		AccessToken:  raw.AccessToken,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    expiresAt(raw.ExpiresIn),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return DevicePollResult{}, err
	}
	return DevicePollResult{Status: "authorized", AccessToken: raw.AccessToken}, nil
}

type Repo struct {
	ID            int64  `json:"id"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

func (s *Service) ListRepos(ctx context.Context) ([]Repo, error) {
	token, err := s.requireToken()
	if err != nil {
		return nil, err
	}
	var installations struct {
		Installations []struct {
			ID int64 `json:"id"`
		} `json:"installations"`
	}
	if err := s.getJSON(ctx, s.cfg.APIBase+"/user/installations", token.AccessToken, &installations); err != nil {
		return nil, err
	}
	var repos []Repo
	for _, inst := range installations.Installations {
		var page struct {
			Repositories []struct {
				ID            int64  `json:"id"`
				Name          string `json:"name"`
				FullName      string `json:"full_name"`
				HTMLURL       string `json:"html_url"`
				DefaultBranch string `json:"default_branch"`
				Private       bool   `json:"private"`
				Owner         struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}
		u := fmt.Sprintf("%s/user/installations/%d/repositories?per_page=100", s.cfg.APIBase, inst.ID)
		if err := s.getJSON(ctx, u, token.AccessToken, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Repositories {
			repos = append(repos, Repo{
				ID:            r.ID,
				Owner:         r.Owner.Login,
				Name:          r.Name,
				FullName:      r.FullName,
				HTMLURL:       r.HTMLURL,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
			})
		}
	}
	return repos, nil
}

type SyncRequest struct {
	Project projects.Project
	Repo    projects.Repo
	Force   bool
}

type SyncResult struct {
	Repo projects.Repo `json:"repo"`
	Path string        `json:"path"`
}

func (s *Service) SyncProject(ctx context.Context, req SyncRequest) (SyncResult, error) {
	token, err := s.requireToken()
	if err != nil {
		return SyncResult{}, err
	}
	repo := req.Repo
	ref := firstNonEmpty(repo.Ref, repo.DefaultBranch, "HEAD")
	root := filepath.Join(s.home, "projects", req.Project.Path)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return SyncResult{}, fmt.Errorf("create project dir: %w", err)
	}
	if !req.Force && needsConfirmation(root) {
		return SyncResult{}, ErrConfirmationRequired
	}
	sha, _ := s.commitSHA(ctx, token.AccessToken, repo.Owner, repo.Name, ref)
	archive, err := s.downloadArchive(ctx, token.AccessToken, repo.Owner, repo.Name, ref)
	if err != nil {
		return SyncResult{}, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(root), ".podium-snapshot-*")
	if err != nil {
		return SyncResult{}, fmt.Errorf("create snapshot staging: %w", err)
	}
	defer os.RemoveAll(tmp)
	stage := filepath.Join(tmp, "contents")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return SyncResult{}, err
	}
	if err := extractZipSnapshot(bytes.NewReader(archive), int64(len(archive)), stage); err != nil {
		return SyncResult{}, err
	}
	if err := replaceProjectContents(root, stage); err != nil {
		return SyncResult{}, err
	}
	repo.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	repo.Mode = "snapshot"
	repo.Provider = "github"
	repo.SourceKind = "archive"
	if repo.Ref == "" {
		repo.Ref = ref
	}
	if err := writeManifest(root, repo, sha); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Repo: repo, Path: root}, nil
}

func (s *Service) installURL() string {
	if s.cfg.AppSlug == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.WebBase, "/") + "/apps/" + s.cfg.AppSlug + "/installations/new"
}

func (s *Service) checkToken(ctx context.Context, token string) error {
	var user struct {
		Login string `json:"login"`
	}
	return s.getJSON(ctx, s.cfg.APIBase+"/user", token, &user)
}

func (s *Service) postForm(ctx context.Context, endpoint string, form url.Values, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("github request failed: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (s *Service) getJSON(ctx context.Context, endpoint, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("github request failed: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (s *Service) commitSHA(ctx context.Context, token, owner, name, ref string) (string, error) {
	var commit struct {
		SHA string `json:"sha"`
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", s.cfg.APIBase, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(ref))
	if err := s.getJSON(ctx, u, token, &commit); err != nil {
		return "", err
	}
	return commit.SHA, nil
}

func (s *Service) downloadArchive(ctx context.Context, token, owner, name, ref string) ([]byte, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/zipball/%s", s.cfg.APIBase, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("github archive download failed: %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 512*1024*1024))
}

type tokenFile struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

func (s *Service) tokenPath() string {
	return filepath.Join(s.home, "github", "token.json")
}

func (s *Service) requireToken() (tokenFile, error) {
	token, err := s.loadToken()
	if err != nil || token.AccessToken == "" {
		return tokenFile{}, errors.New("GitHub is not connected")
	}
	return token, nil
}

func (s *Service) loadToken() (tokenFile, error) {
	raw, err := os.ReadFile(s.tokenPath())
	if err != nil {
		return tokenFile{}, err
	}
	var token tokenFile
	if err := json.Unmarshal(raw, &token); err != nil {
		return tokenFile{}, err
	}
	return token, nil
}

func (s *Service) saveToken(token tokenFile) error {
	path := s.tokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func expiresAt(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339)
}

func extractZipSnapshot(r io.ReaderAt, size int64, dest string) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("open github archive: %w", err)
	}
	for _, file := range zr.File {
		rel, ok, err := archiveRelativePath(file.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("unsafe archive path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.FileInfo().Mode().Perm())
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := errors.Join(src.Close(), dst.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
	}
	return nil
}

func archiveRelativePath(name string) (string, bool, error) {
	if filepath.IsAbs(name) || strings.Contains(name, "\\") {
		return "", false, fmt.Errorf("unsafe archive path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false, fmt.Errorf("unsafe archive path %q", name)
		}
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("unsafe archive path %q", name)
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	if len(parts) <= 1 {
		return "", false, nil
	}
	rel := filepath.Join(parts[1:]...)
	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false, fmt.Errorf("unsafe archive path %q", name)
	}
	return rel, true, nil
}

func needsConfirmation(root string) bool {
	if _, err := os.Stat(filepath.Join(root, ".podium-source.json")); err == nil {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() == ".podium-backups" {
			continue
		}
		return true
	}
	return false
}

func replaceProjectContents(root, stage string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		backupRoot := filepath.Join(root, ".podium-backups")
		stamp := time.Now().UTC().Format("20060102T150405Z")
		backup := filepath.Join(backupRoot, stamp)
		for _, entry := range entries {
			if entry.Name() == ".podium-backups" {
				continue
			}
			if err := os.MkdirAll(backup, 0o755); err != nil {
				return err
			}
			if err := os.Rename(filepath.Join(root, entry.Name()), filepath.Join(backup, entry.Name())); err != nil {
				return err
			}
		}
	}
	stageEntries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	for _, entry := range stageEntries {
		if err := os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func writeManifest(root string, repo projects.Repo, sha string) error {
	manifest := map[string]any{
		"provider":    repo.Provider,
		"mode":        repo.Mode,
		"full_name":   repo.FullName,
		"html_url":    repo.HTMLURL,
		"ref":         repo.Ref,
		"commit_sha":  sha,
		"synced_at":   repo.SyncedAt,
		"source_kind": repo.SourceKind,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".podium-source.json"), raw, 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
