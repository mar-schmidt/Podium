package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/store"
	cron "github.com/robfig/cron/v3"
)

// resyncInterval controls how often the scheduler rescans the schedules
// directory so newly dropped, edited, or removed files take effect without a
// daemon restart (R7.2).
const resyncInterval = 15 * time.Second

// recentRunLimit bounds how many runs List reports per schedule.
const recentRunLimit = 7

// Options configures a Scheduler.
type Options struct {
	Dir    string
	Core   *core.Core
	Store  *store.Store
	Logger *slog.Logger
}

// Scheduler scans schedule files, registers cron jobs inside podiumd, and runs
// each fired job as a normal Podium session (R7.1 / R7.3a).
type Scheduler struct {
	dir   string
	core  *core.Core
	store *store.Store
	log   *slog.Logger
	cron  *cron.Cron

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	jobs      map[string]*job
	parseErrs map[string]error
}

type job struct {
	spec    string
	entryID cron.EntryID
}

// New constructs a Scheduler. Call Start to begin firing.
func New(opts Options) *Scheduler {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		dir:       opts.Dir,
		core:      opts.Core,
		store:     opts.Store,
		log:       log,
		cron:      cron.New(),
		ctx:       ctx,
		cancel:    cancel,
		jobs:      map[string]*job{},
		parseErrs: map[string]error{},
	}
}

// Start performs an initial scan, starts the cron loop, and begins periodic
// resyncing of the schedules directory.
func (s *Scheduler) Start() {
	s.Sync()
	s.cron.Start()
	go s.resyncLoop()
}

// Stop cancels in-flight runs and stops the cron loop.
func (s *Scheduler) Stop() {
	s.cancel()
	s.cron.Stop()
}

func (s *Scheduler) resyncLoop() {
	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.Sync()
			s.pickupDueTasks(s.ctx)
		}
	}
}

// pickupDueTasks starts any backlog task whose scheduled pickup time has passed,
// running it unattended under the preapproved policy. Starting a task moves it to
// in_progress, so it is not picked up twice.
func (s *Scheduler) pickupDueTasks(ctx context.Context) {
	now := time.Now().UTC().Format(time.RFC3339)
	due, err := s.store.ListDueTasks(ctx, now)
	if err != nil {
		s.log.Warn("list due tasks", "error", err)
		return
	}
	for _, task := range due {
		sess, err := s.core.StartTask(ctx, core.StartTaskRequest{TaskID: task.ID, Unattended: true})
		if err != nil {
			s.log.Warn("task pickup failed", "task", task.ID, "error", err)
			continue
		}
		s.log.Info("task picked up", "task", task.ID, "session", sess.ID)
	}
}

// Sync reconciles registered cron jobs with the current contents of the
// schedules directory: new or changed enabled files are (re)registered, and
// disabled or removed files are unregistered so they no longer fire (R7.2a).
func (s *Scheduler) Sync() {
	schedules, parseErrs, err := ScanDir(s.dir)
	if err != nil {
		s.log.Warn("scan schedules directory", "dir", s.dir, "error", err)
		return
	}

	desired := make(map[string]Schedule, len(schedules))
	for _, sc := range schedules {
		desired[sc.Name] = sc
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.parseErrs = parseErrs

	// Drop jobs that are gone, now disabled, or whose timing changed.
	for name, j := range s.jobs {
		sc, ok := desired[name]
		if !ok || !sc.Enabled || sc.CronSpec() != j.spec {
			s.cron.Remove(j.entryID)
			delete(s.jobs, name)
		}
	}

	// Register new or re-enabled jobs.
	for name, sc := range desired {
		if !sc.Enabled {
			continue
		}
		if _, ok := s.jobs[name]; ok {
			continue
		}
		spec := sc.CronSpec()
		jobName := name
		entryID, err := s.cron.AddFunc(spec, func() { s.fire(jobName) })
		if err != nil {
			s.parseErrs[name] = fmt.Errorf("invalid schedule timing %q: %w", spec, err)
			continue
		}
		s.jobs[name] = &job{spec: spec, entryID: entryID}
	}
}

// Status is a schedule's current state for the CLI/API: its frontmatter, whether
// it is registered, its next fire time, any parse error, and recent runs.
type Status struct {
	Name          string              `json:"name"`
	Path          string              `json:"path"`
	Agent         string              `json:"agent"`
	Model         string              `json:"model"`
	Effort        string              `json:"effort"`
	Cron          string              `json:"cron"`
	Every         string              `json:"every"`
	RunPermission RunPermission       `json:"run_permission"`
	AllowedTools  []string            `json:"allowed_tools"`
	Enabled       bool                `json:"enabled"`
	NextRun       *time.Time          `json:"next_run,omitempty"`
	ParseError    string              `json:"parse_error,omitempty"`
	Runs          []store.ScheduleRun `json:"runs"`
}

// List returns the current state of every schedule file, newest-run-aware and
// sorted by name. It resyncs first so freshly dropped files appear.
func (s *Scheduler) List(ctx context.Context) ([]Status, error) {
	s.Sync()
	schedules, parseErrs, err := ScanDir(s.dir)
	if err != nil {
		return nil, err
	}

	var out []Status
	for _, sc := range schedules {
		status := Status{
			Name:          sc.Name,
			Path:          sc.Path,
			Agent:         sc.Agent,
			Model:         sc.Model,
			Effort:        sc.Effort,
			Cron:          sc.Cron,
			Every:         sc.Every,
			RunPermission: sc.RunPermission,
			AllowedTools:  sc.AllowedTools,
			Enabled:       sc.Enabled,
		}
		if next := s.nextRun(sc.Name); next != nil {
			status.NextRun = next
		}
		runs, err := s.store.ListScheduleRuns(ctx, sc.Name, recentRunLimit)
		if err != nil {
			return nil, err
		}
		status.Runs = runs
		out = append(out, status)
	}
	for name, perr := range parseErrs {
		out = append(out, Status{Name: name, ParseError: perr.Error()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Scheduler) nextRun(name string) *time.Time {
	s.mu.Lock()
	j, ok := s.jobs[name]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	entry := s.cron.Entry(j.entryID)
	if entry.Next.IsZero() {
		return nil
	}
	next := entry.Next
	return &next
}

// RunNow triggers a manual run of a schedule immediately. A disabled schedule
// can still be run manually; only automatic firing is suppressed when disabled.
func (s *Scheduler) RunNow(ctx context.Context, name string) (store.ScheduleRun, error) {
	s.Sync()
	if _, err := os.Stat(s.pathFor(name)); err != nil {
		return store.ScheduleRun{}, fmt.Errorf("schedule %q not found", name)
	}
	return s.run(ctx, name, store.TriggerManual)
}

// fire is the cron callback. It runs the job in the scheduler's lifetime context
// so a daemon shutdown cancels it.
func (s *Scheduler) fire(name string) {
	if _, err := s.run(s.ctx, name, store.TriggerCron); err != nil {
		s.log.Warn("scheduled run failed", "schedule", name, "error", err)
	}
}

// run executes one scheduled run end to end: re-parse the file (honoring the
// latest edits and the enabled switch), record a run, execute it as a normal
// Podium session, and persist the terminal status.
func (s *Scheduler) run(ctx context.Context, name string, trigger store.RunTrigger) (store.ScheduleRun, error) {
	sched, err := Parse(s.pathFor(name))
	if err != nil {
		return store.ScheduleRun{}, err
	}
	// Defensive: a disabled file must never fire automatically even if a cron
	// entry briefly outlives a resync (R7.2a).
	if trigger == store.TriggerCron && !sched.Enabled {
		return store.ScheduleRun{}, nil
	}

	run, err := s.store.CreateScheduleRun(ctx, store.ScheduleRun{
		ScheduleName: name,
		Trigger:      trigger,
		Status:       store.RunRunning,
	})
	if err != nil {
		return store.ScheduleRun{}, err
	}
	s.log.Info("scheduled run started", "schedule", name, "run", run.ID, "trigger", trigger)

	sess, runErr := s.core.RunScheduled(ctx, core.ScheduledRunRequest{
		ScheduleName: name,
		RunID:        run.ID,
		AgentName:    sched.Agent,
		Model:        sched.Model,
		Effort:       sched.Effort,
		Yolo:         sched.RunPermission == PermissionYolo,
		AllowedTools: sched.AllowedTools,
		Task:         sched.Body,
	})

	status := store.RunSuccess
	errMsg := ""
	if runErr != nil {
		status = store.RunError
		errMsg = runErr.Error()
	}
	finished, ferr := s.store.FinishScheduleRun(ctx, run.ID, sess.ID, status, errMsg)
	if ferr != nil {
		return run, ferr
	}
	s.log.Info("scheduled run finished",
		"schedule", name, "run", run.ID, "status", status, "session", sess.ID)
	return finished, runErr
}

func (s *Scheduler) pathFor(name string) string {
	return filepath.Join(s.dir, name+".md")
}
