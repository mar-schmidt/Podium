package core

import (
	"context"
	"testing"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/projects"
	"github.com/mar-schmidt/Podium/internal/store"
)

func TestStartTaskCreatesRoadmapSessionWithProvenance(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := c.CreateProject(ctx, projects.Project{ID: "mission-control", Name: "Mission Control"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{ProjectID: "mission-control", Title: "Add dark mode", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	sess, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if sess.Origin != store.OriginRoadmap || sess.TaskID != task.ID {
		t.Fatalf("session provenance wrong: %+v", sess)
	}

	moved, err := c.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if moved.Status != store.TaskInProgress {
		t.Fatalf("task should be in_progress, got %q", moved.Status)
	}

	// The session is discoverable from the task for "Open in chat".
	found, ok, err := c.TaskSession(ctx, task.ID)
	if err != nil || !ok || found.ID != sess.ID {
		t.Fatalf("task session lookup failed: ok=%v err=%v", ok, err)
	}
}

func TestStartTaskRequiresAssignedAgent(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	task, err := c.CreateTask(ctx, store.Task{Title: "unassigned work"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID}); err == nil {
		t.Fatal("expected error starting a task with no assigned agent")
	}
}
