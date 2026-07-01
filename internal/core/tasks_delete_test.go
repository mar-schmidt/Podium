package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// startedTask creates an agent, a task assigned to it, and starts it so a
// roadmap-origin session exists. It returns the task and its session.
func startedTask(t *testing.T, c *Core, title string) (store.Task, store.Session) {
	t.Helper()
	ctx := context.Background()
	if _, err := c.store.GetAgent(ctx, "jared"); err != nil {
		if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}
	task, err := c.CreateTask(ctx, store.Task{Title: title, AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	return task, sess
}

func TestDeleteTaskKeepsSessions(t *testing.T) {
	ctx := context.Background()
	c, cleanup := newTestCore(t)
	defer cleanup()

	task, sess := startedTask(t, c, "wire up delete")

	// Starting moves it to in_progress, where delete is refused.
	if err := c.DeleteTask(ctx, task.ID); err == nil {
		t.Fatal("expected in_progress task delete to be refused")
	}

	// Move it to review, then delete: the task goes, the session stays.
	moved, err := c.store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	moved.Status = store.TaskReview
	if _, err := c.UpdateTask(ctx, moved); err != nil {
		t.Fatalf("move task to review: %v", err)
	}
	if err := c.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if _, err := c.store.GetTask(ctx, task.ID); err == nil {
		t.Fatal("task should be deleted")
	}
	sessions, err := c.store.ListSessionsByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("list sessions by task: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != sess.ID {
		t.Fatalf("session should be preserved after task delete: %+v", sessions)
	}
}

func TestArchiveDoneTasksWritesAndRemoves(t *testing.T) {
	ctx := context.Background()
	c, cleanup := newTestCore(t)
	defer cleanup()

	task, sess := startedTask(t, c, "finish the feature")
	if _, err := c.store.AppendMessages(ctx, sess.ID, []store.Message{
		{Role: store.RoleUser, Content: "do the thing"},
		{Role: store.RoleAssistant, Content: "done"},
	}); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	done, err := c.store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	done.Status = store.TaskDone
	if _, err := c.UpdateTask(ctx, done); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	result, err := c.ArchiveDoneTasks(ctx, "")
	if err != nil {
		t.Fatalf("archive done: %v", err)
	}
	if result.ArchivedTasks != 1 || result.ArchivedSessions != 1 || result.ArchivePath == "" {
		t.Fatalf("bad archive result: %+v", result)
	}

	files, err := filepath.Glob(filepath.Join(result.ArchivePath, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("archive files = %v, want one JSON file", files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var archived struct {
		Task     store.Task `json:"task"`
		Sessions []struct {
			Session  struct{ ID string } `json:"session"`
			Messages []store.Message     `json:"messages"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &archived); err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	if archived.Task.ID != task.ID {
		t.Fatalf("archived wrong task: %+v", archived.Task)
	}
	if len(archived.Sessions) != 1 || archived.Sessions[0].Session.ID != sess.ID || len(archived.Sessions[0].Messages) != 2 {
		t.Fatalf("archived session/messages wrong: %+v", archived.Sessions)
	}

	// Both the task and its session are removed from the active app.
	if _, err := c.store.GetTask(ctx, task.ID); err == nil {
		t.Fatal("task should be removed after archive")
	}
	sessions, err := c.store.ListSessionsByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("list sessions by task: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions should be removed after archive: %+v", sessions)
	}
}
