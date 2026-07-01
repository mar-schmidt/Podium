package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestConnectedRepoContextIsSentToRoadmapSession(t *testing.T) {
	ctx := context.Background()
	c, fake, cleanup := newScheduledTestCore(t)
	defer cleanup()

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderCodex}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	repo := projects.SnapshotRepo("mar-schmidt", "Podium", "https://github.com/mar-schmidt/Podium", "main", "main")
	if _, err := c.CreateProject(ctx, projects.Project{ID: "mission-control", Name: "Mission Control", Repo: &repo}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{ProjectID: "mission-control", Title: "Inspect repo", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if _, err := c.AppendTurn(ctx, sess.ID, TaskPrompt(task)); err != nil {
		t.Fatalf("append turn: %v", err)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("fake requests = %d, want 1", len(fake.Requests))
	}
	root := filepath.Join(c.paths.ProjectsDir, "mission-control")
	req := fake.Requests[0]
	if !strings.Contains(req.Message, "local source snapshot") || !strings.Contains(req.Message, root) {
		t.Fatalf("request missing repo context:\n%s", req.Message)
	}
	// Every session gets the shared projects ledger dir; a roadmap session bound
	// to a repo additionally gets its downloaded project snapshot.
	wantDirs := []string{c.paths.ProjectsDir, root}
	if !reflect.DeepEqual(req.Settings.ExtraWorkspaceDirs, wantDirs) {
		t.Fatalf("extra workspace dirs = %#v, want %#v", req.Settings.ExtraWorkspaceDirs, wantDirs)
	}
	if _, err := c.AppendTurn(ctx, sess.ID, "Continue with repo context"); err != nil {
		t.Fatalf("append second turn: %v", err)
	}
	if len(fake.Requests) != 2 {
		t.Fatalf("fake requests after second turn = %d, want 2", len(fake.Requests))
	}
	if !strings.Contains(fake.Requests[1].Message, "local source snapshot") || !strings.Contains(fake.Requests[1].Message, "Continue with repo context") {
		t.Fatalf("second request missing repo context:\n%s", fake.Requests[1].Message)
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

func TestDescribeTaskPromptEmbedsExpandedProjectContext(t *testing.T) {
	ctx := context.Background()
	c, fake, cleanup := newScheduledTestCore(t)
	defer cleanup()
	fake.Responses = []string{"Build the settings page with clear save and cancel flows."}

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "writer", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := c.CreateProject(ctx, projects.Project{
		ID:          "mission-control",
		Name:        "Mission Control",
		Description: "Operations dashboard for coordinating work.",
		Notes:       "Prefer quiet, utilitarian interfaces.",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	nearby, err := c.CreateTask(ctx, store.Task{
		ProjectID:     "mission-control",
		Title:         "Add project filters",
		Body:          "Let users filter by active projects.",
		AssignedAgent: "writer",
		Status:        store.TaskDone,
	})
	if err != nil {
		t.Fatalf("create nearby task: %v", err)
	}

	body, err := c.DescribeTask(ctx, DescribeTaskRequest{
		AgentName:     "writer",
		ProjectID:     "mission-control",
		Title:         "Add settings page",
		Body:          "Need a settings page.",
		AssignedAgent: "writer",
	})
	if err != nil {
		t.Fatalf("describe task: %v", err)
	}
	if body == "" {
		t.Fatal("expected drafted body")
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("expected one model request, got %d", len(fake.Requests))
	}
	prompt := fake.Requests[0].Message
	for _, want := range []string{
		"projects.yaml under the Podium data directory",
		"Project context:",
		"id: mission-control",
		"Operations dashboard for coordinating work.",
		nearby.ID,
		"Add project filters",
		"Let users filter by active projects.",
		`Task title: "Add settings page"`,
		"Need a settings page.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "~/.podium") {
		t.Fatalf("prompt should not hardcode a Unix home path:\n%s", prompt)
	}
}

func TestUpdateTaskLocksContentAfterSessionButAllowsStatus(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{Title: "Draft docs", Body: "Initial body", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	task.Body = "Updated before start"
	task, err = c.UpdateTask(ctx, task)
	if err != nil {
		t.Fatalf("update before session: %v", err)
	}
	if _, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID}); err != nil {
		t.Fatalf("start task: %v", err)
	}

	task.Body = "Updated after start"
	if _, err := c.UpdateTask(ctx, task); err == nil {
		t.Fatal("expected content update after session to fail")
	}

	started, err := c.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get started task: %v", err)
	}
	started.Status = store.TaskDone
	if _, err := c.UpdateTask(ctx, started); err != nil {
		t.Fatalf("status-only update after session should succeed: %v", err)
	}
}

func TestProjectRoadmapsSyncAndBacklogIsDropped(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	raw := []byte(`projects:
    - id: alpha
      name: Alpha
      description: Alpha project.
      path: alpha
      status: active
      stack: []
      repo: ""
      backlog: ["legacy"]
      roadmap: []
      notes: ""
    - id: beta
      name: Beta
      description: Beta project.
      path: beta
      status: active
      stack: []
      repo: ""
      roadmap: []
      notes: ""
`)
	if err := os.WriteFile(c.paths.ProjectsYAML, raw, 0o644); err != nil {
		t.Fatalf("write legacy projects.yaml: %v", err)
	}
	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{ProjectID: "alpha", Title: "Alpha task", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	projectsList, err := c.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if got := roadmapFor(projectsList, "alpha"); len(got) != 1 || got[0] != task.ID {
		t.Fatalf("alpha roadmap not synced: %+v", projectsList)
	}
	if got := roadmapFor(projectsList, "beta"); len(got) != 0 {
		t.Fatalf("beta roadmap should be empty: %+v", got)
	}

	task.ProjectID = "beta"
	if _, err := c.UpdateTask(ctx, task); err != nil {
		t.Fatalf("reassign task: %v", err)
	}
	projectsList, err = c.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if got := roadmapFor(projectsList, "alpha"); len(got) != 0 {
		t.Fatalf("alpha roadmap should be empty after reassignment: %+v", got)
	}
	if got := roadmapFor(projectsList, "beta"); len(got) != 1 || got[0] != task.ID {
		t.Fatalf("beta roadmap not synced: %+v", got)
	}
	out, err := os.ReadFile(c.paths.ProjectsYAML)
	if err != nil {
		t.Fatalf("read projects.yaml: %v", err)
	}
	if strings.Contains(string(out), "backlog:") {
		t.Fatalf("legacy backlog field should be dropped after ledger write:\n%s", out)
	}
}

func TestRoadmapQuestionMovesTaskReviewAndRestores(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{Title: "Clarify scope", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}

	moved, err := c.MoveRoadmapSessionTaskForQuestion(ctx, sess.ID)
	if err != nil {
		t.Fatalf("move for question: %v", err)
	}
	if !moved {
		t.Fatal("expected in_progress task to move to review")
	}
	got, err := c.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != store.TaskReview {
		t.Fatalf("task should be review, got %q", got.Status)
	}

	if err := c.RestoreRoadmapSessionTaskAfterQuestion(ctx, sess.ID); err != nil {
		t.Fatalf("restore after question: %v", err)
	}
	got, err = c.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != store.TaskInProgress {
		t.Fatalf("task should be in_progress, got %q", got.Status)
	}
}

func roadmapFor(list []projects.Project, id string) []string {
	for _, project := range list {
		if project.ID == id {
			return project.Roadmap
		}
	}
	return nil
}

func TestRoadmapQuestionDoesNotMoveTaskAlreadyInReview(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newScheduledTestCore(t)
	defer cleanup()

	if _, err := c.CreateAgent(ctx, CreateAgentRequest{Name: "jared", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := c.CreateTask(ctx, store.Task{Title: "Already reviewing", AssignedAgent: "jared"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := c.StartTask(ctx, StartTaskRequest{TaskID: task.ID})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	task.Status = store.TaskReview
	if _, err := c.UpdateTask(ctx, task); err != nil {
		t.Fatalf("set review: %v", err)
	}

	moved, err := c.MoveRoadmapSessionTaskForQuestion(ctx, sess.ID)
	if err != nil {
		t.Fatalf("move for question: %v", err)
	}
	if moved {
		t.Fatal("task already in review should not be marked as moved")
	}
}
