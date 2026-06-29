package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// SlashResult reports whether a user message was handled as a slash command.
type SlashResult struct {
	Handled bool
	Session store.Session
	Notice  string
}

// HandleSlashCommand applies session-scoped slash commands without appending
// them to canonical chat history.
func (c *Core) HandleSlashCommand(ctx context.Context, sessionID, input string) (SlashResult, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return SlashResult{}, nil
	}
	command, arg, _ := strings.Cut(trimmed, " ")
	command = strings.ToLower(strings.TrimPrefix(command, "/"))
	arg = strings.TrimSpace(arg)

	sess, err := c.store.GetSession(ctx, sessionID)
	if err != nil {
		return SlashResult{}, err
	}

	switch command {
	case "model":
		if arg == "" {
			return SlashResult{Handled: true, Session: sess, Notice: "Usage: /model <name>"}, nil
		}
		updated, err := c.store.UpdateSessionSettings(ctx, sess.ID, arg, sess.Effort, sess.PermissionMode)
		return SlashResult{Handled: true, Session: updated, Notice: fmt.Sprintf("Model set to %s", arg)}, err
	case "effort":
		if !validEffort(arg) {
			return SlashResult{Handled: true, Session: sess, Notice: "Usage: /effort low|medium|high|xhigh|max"}, nil
		}
		updated, err := c.store.UpdateSessionSettings(ctx, sess.ID, sess.Model, arg, sess.PermissionMode)
		return SlashResult{Handled: true, Session: updated, Notice: fmt.Sprintf("Effort set to %s", arg)}, err
	case "permission":
		mode := config.PermissionMode(arg)
		if mode != config.PermissionApprove && mode != config.PermissionYolo {
			return SlashResult{Handled: true, Session: sess, Notice: "Usage: /permission approve|yolo"}, nil
		}
		updated, err := c.store.UpdateSessionSettings(ctx, sess.ID, sess.Model, sess.Effort, mode)
		return SlashResult{Handled: true, Session: updated, Notice: fmt.Sprintf("Permission mode set to %s", mode)}, err
	case "name":
		if arg == "" {
			return SlashResult{Handled: true, Session: sess, Notice: "Usage: /name <session name>"}, nil
		}
		updated, err := c.store.UpdateSessionMetadata(ctx, sess.ID, arg, sess.Description, false)
		return SlashResult{Handled: true, Session: updated, Notice: "Session name updated"}, err
	case "describe":
		if arg == "" {
			return SlashResult{Handled: true, Session: sess, Notice: "Usage: /describe <session description>"}, nil
		}
		updated, err := c.store.UpdateSessionMetadata(ctx, sess.ID, sess.Name, arg, false)
		return SlashResult{Handled: true, Session: updated, Notice: "Session description updated"}, err
	case "help":
		return SlashResult{
			Handled: true,
			Session: sess,
			Notice:  "/model <name>, /effort <level>, /permission <approve|yolo>, /name <text>, /describe <text>",
		}, nil
	default:
		return SlashResult{Handled: true, Session: sess, Notice: fmt.Sprintf("Unknown command /%s. Try /help.", command)}, nil
	}
}

func validEffort(effort string) bool {
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}
