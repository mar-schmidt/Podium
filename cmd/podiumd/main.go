// Command podiumd is the long-running Podium daemon: it owns all session, agent,
// and schedule state and serves the web UI + API. In Phase 0 it resolves the
// storage root, scaffolds ~/.podium/ on first run, opens the database, and serves
// a health endpoint and the embedded SPA.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/buildinfo"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/schedule"
	"github.com/mar-schmidt/Podium/internal/server"
	"github.com/mar-schmidt/Podium/internal/store"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "podiumd",
		Short: "The Podium orchestration daemon",
		Long: "podiumd is the long-running Podium daemon. It owns session, agent, and\n" +
			"schedule state, serves the web UI and API, and runs the embedded scheduler.\n" +
			"All state lives under $PODIUM_HOME (default ~/.podium/).",
		Version:       fmt.Sprintf("%s (%s)", buildinfo.Version, buildinfo.Commit),
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}
	cmd.AddCommand(newPermissionMCPCmd())
	return cmd
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	home, err := config.ResolveHome()
	if err != nil {
		return fmt.Errorf("resolve storage root: %w", err)
	}
	paths := config.NewPaths(home)
	log.Info("storage root", "home", paths.Home)

	res, err := config.Scaffold(paths)
	if err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}
	if res.CreatedHome {
		log.Info("initialized fresh storage root", "home", paths.Home)
	}
	if res.CreatedConfig {
		log.Info("wrote default config", "path", paths.ConfigYAML)
	}

	cfg, err := config.Load(paths.ConfigYAML)
	if err != nil {
		return err
	}
	log.Info("config loaded",
		"provider", cfg.Global.Provider,
		"agents", len(cfg.Agents),
		"profiles", len(cfg.Profiles),
	)

	db, err := store.Open(paths.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database open", "path", paths.DB)

	addr := net.JoinHostPort(cfg.Server.Bind, strconv.Itoa(cfg.Server.Port))
	permissionTimeout, err := time.ParseDuration(cfg.Global.PermissionTimeout)
	if err != nil {
		return err
	}
	adapters := map[config.Provider]adapter.Adapter{}
	claude, err := adapter.NewClaude(adapter.ClaudeOptions{
		DaemonAddr:        addr,
		PermissionTimeout: permissionTimeout,
	})
	if err != nil {
		log.Warn("claude adapter unavailable", "error", err)
		adapters[config.ProviderClaude] = adapter.Unavailable{Provider: config.ProviderClaude, Err: err}
	} else {
		adapters[config.ProviderClaude] = claude
	}
	codex, err := adapter.NewCodex(adapter.CodexOptions{
		PermissionTimeout: permissionTimeout,
	})
	if err != nil {
		log.Warn("codex adapter unavailable", "error", err)
		adapters[config.ProviderCodex] = adapter.Unavailable{Provider: config.ProviderCodex, Err: err}
	} else {
		adapters[config.ProviderCodex] = codex
	}
	coreSvc, err := core.New(core.Options{
		Paths:    paths,
		Store:    db,
		Adapter:  adapter.NewRouter(adapters),
		Global:   cfg.Global,
		Profiles: cfg.Profiles,
		Logger:   log,
	})
	if err != nil {
		return err
	}
	if err := syncConfiguredAgents(context.Background(), coreSvc, cfg); err != nil {
		return err
	}

	scheduler := schedule.New(schedule.Options{
		Dir:    paths.SchedulesDir,
		Core:   coreSvc,
		Store:  db,
		Logger: log,
	})
	scheduler.Start()
	defer scheduler.Stop()
	log.Info("scheduler started", "dir", paths.SchedulesDir)

	srv := server.New(server.Options{
		Bind: cfg.Server.Bind,
		Port: cfg.Server.Port,
		Build: server.BuildInfo{
			Version: buildinfo.Version,
			Commit:  buildinfo.Commit,
		},
		Core:      coreSvc,
		Scheduler: scheduler,
	})

	// Serve until a termination signal arrives, then shut down gracefully.
	errc := make(chan error, 1)
	go func() { errc <- srv.Start() }()
	log.Info("podiumd listening", "addr", srv.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func syncConfiguredAgents(ctx context.Context, coreSvc *core.Core, cfg *config.Config) error {
	for _, a := range cfg.Agents {
		agent, err := coreSvc.GetAgent(ctx, a.Name)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				if _, err := coreSvc.CreateAgent(ctx, core.CreateAgentRequest{
					Name:           a.Name,
					Provider:       a.Provider,
					Profile:        a.Profile,
					Model:          a.Model,
					Effort:         a.Effort,
					PermissionMode: a.PermissionMode,
					Fallback:       a.Fallback,
					MCPConfig:      a.MCPConfig,
				}); err != nil {
					return err
				}
				continue
			}
			return err
		}
		agent.Provider = a.Provider
		agent.Profile = a.Profile
		agent.Model = a.Model
		agent.Effort = a.Effort
		agent.PermissionMode = a.PermissionMode
		agent.Fallback = a.Fallback
		agent.MCPConfig = a.MCPConfig
		if _, err := coreSvc.UpdateAgent(ctx, agent); err != nil {
			return err
		}
	}
	return nil
}
