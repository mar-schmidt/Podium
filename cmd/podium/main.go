// Command podium is the thin Podium CLI client. It always connects to a running
// podiumd (it does not run sessions in-process) so there is a single source of
// runtime truth (R11.1 / D2). In Phase 0 it can report daemon status.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/mar-schmidt/Podium/internal/buildinfo"
	"github.com/mar-schmidt/Podium/internal/client"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var addr string

	root := &cobra.Command{
		Use:   "podium",
		Short: "Podium CLI — a thin client for the podiumd daemon",
		Long: "podium is the command-line client for Podium. It connects to a running\n" +
			"podiumd daemon, which owns all session, agent, and schedule state. Start the\n" +
			"daemon with `podiumd`.",
		Version:       fmt.Sprintf("%s (%s)", buildinfo.Version, buildinfo.Commit),
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVar(&addr, "addr", "",
		"daemon address host:port (default: from config.yaml, or PODIUM_ADDR)")

	root.AddCommand(newStatusCmd(&addr))
	return root
}

func newStatusCmd(addr *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether podiumd is running and its version",
		Long:  "Connects to the daemon's health endpoint and prints its status, version, and uptime.",
		Example: "  podium status\n  podium --addr 127.0.0.1:8787 status",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveAddr(*addr)
			if err != nil {
				return err
			}
			c := client.New(resolved)
			h, err := c.Health(context.Background())
			if err != nil {
				if errors.Is(err, client.ErrDaemonUnreachable) {
					fmt.Fprintf(os.Stderr, "podiumd is not running (tried %s).\nStart it with: podiumd\n", resolved)
					return err
				}
				return err
			}
			fmt.Printf("podiumd is live at %s\n", resolved)
			fmt.Printf("  status:  %s\n", h.Status)
			fmt.Printf("  version: %s (%s)\n", h.Version, h.Commit)
			fmt.Printf("  uptime:  %dms\n", h.UptimeMS)
			return nil
		},
	}
}

// resolveAddr determines the daemon address with precedence:
//  1. explicit --addr flag,
//  2. PODIUM_ADDR environment variable,
//  3. config.yaml server.bind:server.port under $PODIUM_HOME,
//  4. the built-in default 127.0.0.1:8787.
func resolveAddr(flagAddr string) (string, error) {
	if flagAddr != "" {
		return flagAddr, nil
	}
	if env := os.Getenv("PODIUM_ADDR"); env != "" {
		return env, nil
	}
	home, err := config.ResolveHome()
	if err == nil {
		paths := config.NewPaths(home)
		if cfg, err := config.Load(paths.ConfigYAML); err == nil {
			return net.JoinHostPort(cfg.Server.Bind, strconv.Itoa(cfg.Server.Port)), nil
		}
	}
	return "127.0.0.1:8787", nil
}
