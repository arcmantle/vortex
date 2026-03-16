// Package orchestrator manages job lifecycle with dependency-aware execution.
//
// Jobs can depend on other jobs via the "needs" field. The orchestrator
// evaluates conditions ("success", "failure", "always") before starting each
// job, enabling sequential, parallel, or conditional pipelines similar to
// GitHub Actions job dependencies.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"arcmantle/vortex/internal/config"
	"arcmantle/vortex/internal/terminal"
)

// Status describes a job's lifecycle state.
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
	StatusSkipped Status = "skipped"
)

// Job is a single job with its live runtime state.
type Job struct {
	Spec config.JobSpec

	mu      sync.Mutex
	status  Status
	term    *terminal.Terminal

	// started is closed once proc is set, or when the job reaches a terminal
	// state without ever starting a process (skipped / failed to launch).
	started chan struct{}
	// done is closed when the job reaches any terminal state.
	done chan struct{}
}

func newJob(spec config.JobSpec) *Job {
	return &Job{
		Spec:    spec,
		status:  StatusPending,
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Status returns the current lifecycle state.
func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// Terminal returns the underlying terminal once started, or nil if not yet
// started / skipped.
func (j *Job) Terminal() *terminal.Terminal {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.term
}

// Started returns a channel closed once the process is available (or the job
// reaches a terminal state without starting).
func (j *Job) Started() <-chan struct{} { return j.started }

// Done returns a channel closed when the job reaches any terminal state.
func (j *Job) Done() <-chan struct{} { return j.done }

func (j *Job) setStatus(s Status) {
	j.mu.Lock()
	j.status = s
	j.mu.Unlock()
}

// ---------------------------------------------------------------------------

// Orchestrator manages all jobs and their lifecycle.
type Orchestrator struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	order   []string // declaration order from config
	termMgr *terminal.Manager
	gen     int // incremented on each Restart

	// restarted is closed whenever Restart replaces the job graph, signaling
	// long-lived SSE handlers to re-fetch their job by ID.
	restarted chan struct{}
}

// New creates an Orchestrator from a config. Jobs are not started until Start
// is called.
func New(cfg *config.Config) (*Orchestrator, error) {
	o := &Orchestrator{
		jobs:      make(map[string]*Job),
		termMgr:   terminal.NewManager(),
		restarted: make(chan struct{}),
	}
	for _, spec := range cfg.Jobs {
		label := spec.Label
		if label == "" {
			label = spec.ID
		}
		spec.Label = label
		o.jobs[spec.ID] = newJob(spec)
		o.order = append(o.order, spec.ID)
	}
	// Validate needs (config.Load already checks this, but be defensive).
	for _, spec := range cfg.Jobs {
		for _, need := range spec.Needs {
			if _, ok := o.jobs[need]; !ok {
				return nil, fmt.Errorf("job %q needs unknown job %q", spec.ID, need)
			}
		}
	}
	return o, nil
}

// Start begins executing all jobs according to their dependency graph.
// This is non-blocking; each job runs in its own goroutine.
func (o *Orchestrator) Start(ctx context.Context) {
	for _, id := range o.order {
		go o.runJob(ctx, o.jobs[id])
	}
}

func (o *Orchestrator) runJob(ctx context.Context, job *Job) {
	// Wait for all dependencies to reach a terminal state.
	for _, needID := range job.Spec.Needs {
		dep := o.jobs[needID]
		select {
		case <-dep.Done():
		case <-ctx.Done():
			job.setStatus(StatusSkipped)
			close(job.started)
			close(job.done)
			return
		}
	}

	// Evaluate the "if" condition.
	cond := job.Spec.If
	if cond == "" {
		cond = "success"
	}

	shouldRun := false
	switch cond {
	case "always":
		shouldRun = true
	case "success":
		shouldRun = o.allSucceeded(job.Spec.Needs)
	case "failure":
		shouldRun = o.anyFailed(job.Spec.Needs)
	default:
		log.Printf("[orchestrator] job %q: unknown condition %q, defaulting to success", job.Spec.ID, cond)
		shouldRun = o.allSucceeded(job.Spec.Needs)
	}

	if !shouldRun {
		job.setStatus(StatusSkipped)
		close(job.started)
		close(job.done)
		return
	}

	// Parse command.
	parts := strings.Fields(job.Spec.Command)
	if len(parts) == 0 {
		log.Printf("[orchestrator] job %q has empty command", job.Spec.ID)
		job.setStatus(StatusFailure)
		close(job.started)
		close(job.done)
		return
	}

	// Start the process in a terminal.
	job.setStatus(StatusRunning)
	term, err := o.termMgr.Start(ctx, job.Spec.ID, job.Spec.Label, parts[0], parts[1:])
	if err != nil {
		log.Printf("[orchestrator] failed to start job %q: %v", job.Spec.ID, err)
		job.setStatus(StatusFailure)
		close(job.started)
		close(job.done)
		return
	}
	job.mu.Lock()
	job.term = term
	job.mu.Unlock()
	close(job.started) // terminal is now available

	// Wait for process to exit and record the outcome.
	<-term.Done()
	if term.ExitCode() == 0 {
		job.setStatus(StatusSuccess)
	} else {
		job.setStatus(StatusFailure)
	}
	close(job.done)
}

func (o *Orchestrator) allSucceeded(needs []string) bool {
	for _, id := range needs {
		if o.jobs[id].Status() != StatusSuccess {
			return false
		}
	}
	return true
}

func (o *Orchestrator) anyFailed(needs []string) bool {
	for _, id := range needs {
		if o.jobs[id].Status() == StatusFailure {
			return true
		}
	}
	return false
}

// AllJobs returns all jobs in declaration order.
func (o *Orchestrator) AllJobs() []*Job {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]*Job, 0, len(o.order))
	for _, id := range o.order {
		out = append(out, o.jobs[id])
	}
	return out
}

// GetJob returns a job by ID.
func (o *Orchestrator) GetJob(id string) (*Job, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	j, ok := o.jobs[id]
	return j, ok
}

// Restarted returns a channel that is closed when the job graph is replaced.
// After the channel fires, callers should re-fetch their jobs by ID.
func (o *Orchestrator) Restarted() <-chan struct{} {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.restarted
}

// Generation returns a counter that increments on each Restart call.
func (o *Orchestrator) Generation() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.gen
}

// AddAndStart adds a new simple job (no dependencies, no group) and starts it
// immediately. Used when a second instance forwards args via handoff.
func (o *Orchestrator) AddAndStart(ctx context.Context, id, label, command string, args []string) {
	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}
	spec := config.JobSpec{ID: id, Label: label, Command: fullCmd}
	job := newJob(spec)

	o.mu.Lock()
	o.jobs[id] = job
	o.order = append(o.order, id)
	o.mu.Unlock()

	go o.runJob(ctx, job)
}

// Restart kills all running processes, replaces the job graph with the new
// config, and re-runs the full dependency graph. Existing job IDs are reused
// so the UI can show updates in the same terminal panels.
func (o *Orchestrator) Restart(ctx context.Context, cfg *config.Config) {
	o.mu.Lock()

	// 1. Kill every running process.
	for _, id := range o.order {
		job := o.jobs[id]
		if term := job.Terminal(); term != nil {
			term.Kill()
		}
	}
	// Wait for all to reach terminal state (under unlock so drain goroutines
	// can acquire the job mutex).
	doneChans := make([]<-chan struct{}, 0, len(o.order))
	for _, id := range o.order {
		doneChans = append(doneChans, o.jobs[id].Done())
	}
	o.mu.Unlock()

	for _, ch := range doneChans {
		<-ch
	}

	// 2. Rebuild the job graph from the new config, keeping the same procMgr
	//    so new processes inherit old output buffers.
	o.mu.Lock()
	newJobs := make(map[string]*Job, len(cfg.Jobs))
	newOrder := make([]string, 0, len(cfg.Jobs))
	for _, spec := range cfg.Jobs {
		label := spec.Label
		if label == "" {
			label = spec.ID
		}
		spec.Label = label
		newJobs[spec.ID] = newJob(spec)
		newOrder = append(newOrder, spec.ID)
	}
	o.jobs = newJobs
	o.order = newOrder

	// Signal SSE handlers to re-fetch their jobs.
	close(o.restarted)
	o.restarted = make(chan struct{})
	o.gen++
	o.mu.Unlock()

	// 3. Start the new dependency graph.
	log.Printf("[orchestrator] restarting with %d jobs", len(cfg.Jobs))
	o.Start(ctx)
}

// Shutdown kills every running process tree managed by this orchestrator.
func (o *Orchestrator) Shutdown() {
	o.mu.RLock()
	for _, id := range o.order {
		job := o.jobs[id]
		if term := job.Terminal(); term != nil {
			term.Kill()
		}
	}
	o.mu.RUnlock()
}
