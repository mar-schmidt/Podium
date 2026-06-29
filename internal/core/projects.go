package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/mar-schmidt/Podium/internal/projects"
	"github.com/mar-schmidt/Podium/internal/store"
)

// ListProjects returns the shared project ledger (§5.3).
func (c *Core) ListProjects(ctx context.Context) ([]projects.Project, error) {
	return c.ledger.List()
}

// GetProject returns one project by id.
func (c *Core) GetProject(ctx context.Context, id string) (projects.Project, error) {
	return c.ledger.Get(id)
}

// CreateProject adds a project to the shared ledger and creates its directory.
func (c *Core) CreateProject(ctx context.Context, p projects.Project) (projects.Project, error) {
	return c.ledger.Create(p)
}

// ListTasks returns all roadmap tasks.
func (c *Core) ListTasks(ctx context.Context) ([]store.Task, error) {
	return c.store.ListTasks(ctx)
}

// GetTask returns one task.
func (c *Core) GetTask(ctx context.Context, id string) (store.Task, error) {
	return c.store.GetTask(ctx, id)
}

// CreateTask creates a roadmap task. A title is required; status defaults to
// backlog.
func (c *Core) CreateTask(ctx context.Context, task store.Task) (store.Task, error) {
	if strings.TrimSpace(task.Title) == "" {
		return store.Task{}, fmt.Errorf("task title is required")
	}
	if task.AssignedAgent != "" {
		if _, err := c.store.GetAgent(ctx, task.AssignedAgent); err != nil {
			return store.Task{}, fmt.Errorf("assigned agent %q: %w", task.AssignedAgent, err)
		}
	}
	return c.store.CreateTask(ctx, task)
}

// UpdateTask stores task changes (assignment, status, body, title, pickup).
func (c *Core) UpdateTask(ctx context.Context, task store.Task) (store.Task, error) {
	if task.AssignedAgent != "" {
		if _, err := c.store.GetAgent(ctx, task.AssignedAgent); err != nil {
			return store.Task{}, fmt.Errorf("assigned agent %q: %w", task.AssignedAgent, err)
		}
	}
	return c.store.UpdateTask(ctx, task)
}

// TaskSession returns the most recent roadmap session started from a task, if
// any. The boolean is false when the task has not been started.
func (c *Core) TaskSession(ctx context.Context, taskID string) (store.Session, bool, error) {
	sessions, err := c.store.ListSessionsByTask(ctx, taskID)
	if err != nil {
		return store.Session{}, false, err
	}
	if len(sessions) == 0 {
		return store.Session{}, false, nil
	}
	return sessions[0], true, nil
}

// StartTaskRequest starts work on a roadmap task.
type StartTaskRequest struct {
	TaskID string
	// Unattended runs the first turn server-side under the preapproved policy
	// (used by scheduled pickup); on-demand starts leave the first turn to the
	// caller (the web client sends it interactively).
	Unattended bool
}

// StartTask creates a durable roadmap-origin session bound to the task's
// assigned agent (with provenance back to the task), moves the task to
// in_progress, and — for unattended pickups — runs the task as one turn.
func (c *Core) StartTask(ctx context.Context, req StartTaskRequest) (store.Session, error) {
	task, err := c.store.GetTask(ctx, req.TaskID)
	if err != nil {
		return store.Session{}, err
	}
	if task.AssignedAgent == "" {
		return store.Session{}, fmt.Errorf("task %q has no assigned agent", task.ID)
	}

	sess, err := c.CreateSession(ctx, CreateSessionRequest{
		AgentName: task.AssignedAgent,
		Origin:    store.OriginRoadmap,
		TaskID:    task.ID,
	})
	if err != nil {
		return store.Session{}, err
	}

	task.Status = store.TaskInProgress
	if _, err := c.store.UpdateTask(ctx, task); err != nil {
		return store.Session{}, err
	}

	if req.Unattended {
		events, err := c.StreamTurn(ctx, sess.ID, TaskPrompt(task), TurnOptions{
			PermissionTurnID: sess.ID,
			PermissionRelay:  NewAllowListRelay(nil),
			Unattended:       true,
		})
		if err != nil {
			return sess, err
		}
		for range events { // drain
		}
	}
	return sess, nil
}

// TaskPrompt renders a task into the prompt used to seed its session.
func TaskPrompt(task store.Task) string {
	if strings.TrimSpace(task.Body) == "" {
		return task.Title
	}
	return task.Title + "\n\n" + task.Body
}
