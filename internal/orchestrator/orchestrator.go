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
	"time"

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
	mu     sync.Mutex
	spec   config.JobSpec
	status Status
	term   *terminal.Terminal

	// started is closed once proc is set, or when the job reaches a terminal
	// state without ever starting a process (skipped / failed to launch).
	started     chan struct{}
	startedOnce sync.Once
	// done is closed when the job reaches any terminal state.
	done     chan struct{}
	doneOnce sync.Once
}

func newJob(spec config.JobSpec) *Job {
	return &Job{
		spec:    spec,
		status:  StatusPending,
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func shouldCarryPersistentJob(oldJob *Job, newSpec config.JobSpec, nodeRuntimeChanged, bunRuntimeChanged, denoRuntimeChanged, csharpRuntimeChanged, goRuntimeChanged bool) bool {
	if oldJob == nil {
		return false
	}
	// Don't carry forward jobs that have already exited — their channels are
	// closed and reuse would panic.
	if oldJob.Status() != StatusRunning {
		return false
	}
	if oldJob.Spec().ShouldRestart() || newSpec.ShouldRestart() {
		return false
	}
	if nodeRuntimeChanged && newSpec.UsesNodeRuntime() {
		return false
	}
	if bunRuntimeChanged && newSpec.UsesBunRuntime() {
		return false
	}
	if denoRuntimeChanged && newSpec.UsesDenoRuntime() {
		return false
	}
	if csharpRuntimeChanged && newSpec.UsesCSharpRuntime() {
		return false
	}
	if goRuntimeChanged && newSpec.UsesGoRuntime() {
		return false
	}
	return true
}

// Spec returns a snapshot of the job spec, safe for concurrent use.
func (j *Job) Spec() config.JobSpec {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.spec
}

// updateSpec replaces the spec under the job mutex, used by Restart to update
// metadata on carried-forward persistent jobs.
func (j *Job) updateSpec(spec config.JobSpec) {
	j.mu.Lock()
	j.spec = spec
	j.mu.Unlock()
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

// closeStarted safely closes the started channel at most once.
func (j *Job) closeStarted() { j.startedOnce.Do(func() { close(j.started) }) }

// closeDone safely closes the done channel at most once.
func (j *Job) closeDone() { j.doneOnce.Do(func() { close(j.done) }) }

func (j *Job) setStatus(s Status) {
	j.mu.Lock()
	j.status = s
	j.mu.Unlock()
}

// ---------------------------------------------------------------------------

// Orchestrator manages all jobs and their lifecycle.
type Orchestrator struct {
	mu      sync.RWMutex
	cfg     *config.Config
	jobs    map[string]*Job
	order   []string // declaration order from config
	termMgr *terminal.Manager
	workDir string
	gen     int // incremented on each Restart

	// restarted is closed whenever Restart replaces the job graph, signaling
	// long-lived SSE handlers to re-fetch their job by ID.
	restarted chan struct{}
}

// New creates an Orchestrator from a config. Jobs are not started until Start
// is called.
func New(cfg *config.Config) (*Orchestrator, error) {
	o := &Orchestrator{
		cfg:       cfg,
		jobs:      make(map[string]*Job),
		termMgr:   terminal.NewManager(),
		workDir:   cfg.WorkingDir,
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

// jobLaunch bundles a job with its pre-resolved dependency pointers.
// Dependencies are resolved under the orchestrator lock so that runJob never
// touches o.jobs directly and is therefore safe against concurrent Restarts.
type jobLaunch struct {
	job  *Job
	deps []*Job
	cfg  *config.Config
}

// resolveLaunchesLocked builds a jobLaunch slice for the given job IDs.
// Caller must hold at least o.mu.RLock.
func (o *Orchestrator) resolveLaunchesLocked(ids []string) []jobLaunch {
	launches := make([]jobLaunch, 0, len(ids))
	for _, id := range ids {
		job, ok := o.jobs[id]
		if !ok {
			continue
		}
		spec := job.Spec()
		deps := make([]*Job, 0, len(spec.Needs))
		for _, needID := range spec.Needs {
			if dep, ok := o.jobs[needID]; ok {
				deps = append(deps, dep)
			}
		}
		launches = append(launches, jobLaunch{job: job, deps: deps, cfg: o.cfg})
	}
	return launches
}

// Start begins executing all jobs according to their dependency graph.
// This is non-blocking; each job runs in its own goroutine.
func (o *Orchestrator) Start(ctx context.Context) {
	o.mu.RLock()
	launches := o.resolveLaunchesLocked(o.order)
	o.mu.RUnlock()
	for _, l := range launches {
		go o.runJob(ctx, l.job, l.deps, l.cfg)
	}
}

// runJob executes a single job after waiting for its pre-resolved deps.
// deps must be resolved under the orchestrator lock before calling runJob
// to avoid data races against concurrent Restart operations.
func (o *Orchestrator) runJob(ctx context.Context, job *Job, deps []*Job, cfg *config.Config) {
	// Skip jobs that are already running (persistent jobs carried over from
	// a previous generation).
	if job.Status() == StatusRunning {
		return
	}

	// Snapshot the spec once so all reads are consistent and lock-free.
	spec := job.Spec()

	// Wait for all dependencies to reach a terminal state.
	// Persistent running jobs (restart: false) are treated as satisfied.
	for _, dep := range deps {
		if !dep.Spec().ShouldRestart() && dep.Status() == StatusRunning {
			continue // persistent job still running — treat as satisfied
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-dep.Done():
			timer.Stop()
		case <-timer.C:
			log.Printf("[orchestrator] job %q: still waiting on dependency %q after 30s", spec.ID, dep.Spec().ID)
			select {
			case <-dep.Done():
			case <-ctx.Done():
				job.setStatus(StatusSkipped)
				job.closeStarted()
				job.closeDone()
				return
			}
		case <-ctx.Done():
			timer.Stop()
			job.setStatus(StatusSkipped)
			job.closeStarted()
			job.closeDone()
			return
		}
	}

	// Evaluate the "if" condition.
	cond := spec.If
	if cond == "" {
		cond = "success"
	}

	shouldRun := false
	switch cond {
	case "always":
		shouldRun = true
	case "success":
		shouldRun = o.allSucceeded(deps)
	case "failure":
		shouldRun = o.anyFailed(deps)
	default:
		log.Printf("[orchestrator] job %q: unknown condition %q, defaulting to success", spec.ID, cond)
		shouldRun = o.allSucceeded(deps)
	}

	if !shouldRun {
		job.setStatus(StatusSkipped)
		job.closeStarted()
		job.closeDone()
		return
	}

	command, args, err := cfg.PrepareJobCommand(spec)
	if err != nil {
		log.Printf("[orchestrator] failed to resolve job %q command: %v", spec.ID, err)
		job.setStatus(StatusFailure)
		job.closeStarted()
		job.closeDone()
		return
	}

	// Start the process in a terminal.
	job.setStatus(StatusRunning)
	term, err := o.termMgr.Start(ctx, spec.ID, spec.Label, command, args, o.workDir)
	if err != nil {
		log.Printf("[orchestrator] failed to start job %q: %v", spec.ID, err)
		job.setStatus(StatusFailure)
		job.closeStarted()
		job.closeDone()
		return
	}
	job.mu.Lock()
	job.term = term
	job.mu.Unlock()
	job.closeStarted() // terminal is now available

	// Wait for process to exit and record the outcome.
	<-term.Done()
	if term.ExitCode() == 0 {
		job.setStatus(StatusSuccess)
	} else {
		job.setStatus(StatusFailure)
	}
	job.closeDone()
}

func (o *Orchestrator) allSucceeded(deps []*Job) bool {
	for _, j := range deps {
		// Persistent running jobs count as succeeded for dependency purposes.
		if !j.Spec().ShouldRestart() && j.Status() == StatusRunning {
			continue
		}
		if j.Status() != StatusSuccess {
			return false
		}
	}
	return true
}

func (o *Orchestrator) anyFailed(deps []*Job) bool {
	for _, j := range deps {
		if !j.Spec().ShouldRestart() && j.Status() == StatusRunning {
			continue
		}
		if j.Status() == StatusFailure {
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

// WorkDir returns the working directory used for all jobs in this run.
func (o *Orchestrator) WorkDir() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.workDir
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
	if _, exists := o.jobs[id]; exists {
		o.mu.Unlock()
		log.Printf("[orchestrator] AddAndStart: job %q already exists, ignoring duplicate", id)
		return
	}
	o.jobs[id] = job
	o.order = append(o.order, id)
	cfg := o.cfg
	o.mu.Unlock()

	go o.runJob(ctx, job, nil, cfg) // no declared dependencies for dynamically added jobs
}

// Restart kills all running processes, replaces the job graph with the new
// config, and re-runs the full dependency graph. Existing job IDs are reused
// so the UI can show updates in the same terminal panels.
func (o *Orchestrator) Restart(ctx context.Context, cfg *config.Config) {
	o.mu.Lock()
	keepIDs := make(map[string]struct{}, len(cfg.Jobs))
	nodeRuntimeChanged := false
	bunRuntimeChanged := false
	denoRuntimeChanged := false
	csharpRuntimeChanged := false
	goRuntimeChanged := false
	if o.cfg != nil {
		nodeRuntimeChanged = !o.cfg.Node.Equal(cfg.Node)
		bunRuntimeChanged = !o.cfg.Bun.Equal(cfg.Bun)
		denoRuntimeChanged = !o.cfg.Deno.Equal(cfg.Deno)
		csharpRuntimeChanged = !o.cfg.CSharp.Equal(cfg.CSharp)
		goRuntimeChanged = !o.cfg.Go.Equal(cfg.Go)
	}
	for _, spec := range cfg.Jobs {
		keepIDs[spec.ID] = struct{}{}
	}

	// 1. Kill running processes — but only for jobs that should restart.
	//    Persistent jobs (restart: false) keep running.
	var toWait []<-chan struct{}
	persistent := make(map[string]*Job) // old jobs to carry forward
	nextSpecs := make(map[string]config.JobSpec, len(cfg.Jobs))
	for _, spec := range cfg.Jobs {
		nextSpecs[spec.ID] = spec
	}
	for _, id := range o.order {
		job := o.jobs[id]
		nextSpec, stillPresent := nextSpecs[id]
		if stillPresent && shouldCarryPersistentJob(job, nextSpec, nodeRuntimeChanged, bunRuntimeChanged, denoRuntimeChanged, csharpRuntimeChanged, goRuntimeChanged) {
			persistent[id] = job
			continue
		}
		if term := job.Terminal(); term != nil {
			term.Kill()
		}
		toWait = append(toWait, job.Done())
	}
	o.mu.Unlock()

	// Wait for killed jobs to reach terminal state.
	for _, ch := range toWait {
		<-ch
	}

	// 2. Rebuild the job graph. Persistent jobs that still exist in the new
	//    config are carried over; everything else gets a fresh Job.
	o.mu.Lock()
	newJobs := make(map[string]*Job, len(cfg.Jobs))
	newOrder := make([]string, 0, len(cfg.Jobs))
	o.workDir = cfg.WorkingDir
	for _, spec := range cfg.Jobs {
		label := spec.Label
		if label == "" {
			label = spec.ID
		}
		spec.Label = label

		if old, ok := persistent[spec.ID]; ok && !spec.ShouldRestart() {
			// Carry forward the running job but update its spec so metadata
			// (label, env, etc.) reflects the new config.
			old.updateSpec(spec)
			newJobs[spec.ID] = old
		} else {
			newJobs[spec.ID] = newJob(spec)
		}
		newOrder = append(newOrder, spec.ID)
	}
	o.jobs = newJobs
	o.order = newOrder
	o.termMgr.Prune(keepIDs)
	o.cfg = cfg

	// Signal SSE handlers to re-fetch their jobs.
	close(o.restarted)
	o.restarted = make(chan struct{})
	o.gen++
	o.mu.Unlock()

	// 3. Start the new dependency graph. Persistent jobs that are already
	//    running will be skipped by runJob (their done channel is not closed
	//    until they actually exit).
	log.Printf("[orchestrator] restarting with %d jobs", len(cfg.Jobs))
	o.Start(ctx)
}

// Rerun re-executes a single job and every downstream job that depends on it,
// while leaving unrelated jobs untouched.
func (o *Orchestrator) Rerun(ctx context.Context, id string) error {
	o.mu.RLock()
	if _, ok := o.jobs[id]; !ok {
		o.mu.RUnlock()
		return fmt.Errorf("job %q not found", id)
	}
	affectedSet := o.collectDownstreamLocked(id)
	affectedOrder := make([]string, 0, len(o.order))
	toWait := make([]<-chan struct{}, 0, len(affectedSet))

	// Save specs and collect channels to wait on while we still hold the lock.
	savedSpecs := make(map[string]config.JobSpec, len(affectedSet))
	for _, jobID := range o.order {
		if _, ok := affectedSet[jobID]; !ok {
			continue
		}
		affectedOrder = append(affectedOrder, jobID)
		job := o.jobs[jobID]
		savedSpecs[jobID] = job.Spec()
		if term := job.Terminal(); term != nil && job.Status() == StatusRunning {
			term.Kill()
			toWait = append(toWait, job.Done())
		}
	}
	o.mu.RUnlock()

	// Wait outside the lock for killed jobs to reach a terminal state.
	for _, ch := range toWait {
		<-ch
	}

	// Rebuild affected jobs and resolve their deps under the write lock so
	// there is no window for a concurrent Restart to invalidate the map.
	o.mu.Lock()
	launches := make([]jobLaunch, 0, len(affectedOrder))
	for _, jobID := range affectedOrder {
		spec, ok := savedSpecs[jobID]
		if !ok {
			continue
		}
		if _, exists := o.jobs[jobID]; !exists {
			// Removed by a concurrent Restart — skip.
			continue
		}
		j := newJob(spec)
		o.jobs[jobID] = j
		deps := make([]*Job, 0, len(spec.Needs))
		for _, needID := range spec.Needs {
			if dep, ok := o.jobs[needID]; ok {
				deps = append(deps, dep)
			}
		}
		launch := jobLaunch{job: j, deps: deps, cfg: o.cfg}
		launches = append(launches, launch)
	}
	close(o.restarted)
	o.restarted = make(chan struct{})
	o.gen++
	o.mu.Unlock()

	for _, l := range launches {
		go o.runJob(ctx, l.job, l.deps, l.cfg)
	}
	return nil
}

func (o *Orchestrator) collectDownstreamLocked(id string) map[string]struct{} {
	affected := map[string]struct{}{id: {}}
	queue := []string{id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, candidateID := range o.order {
			if _, seen := affected[candidateID]; seen {
				continue
			}
			job := o.jobs[candidateID]
			for _, needID := range job.Spec().Needs {
				if needID != current {
					continue
				}
				affected[candidateID] = struct{}{}
				queue = append(queue, candidateID)
				break
			}
		}
	}
	return affected
}

// KillOnCloseJobs kills all running jobs that have kill_on_close: true in their
// spec. Called when the GUI window is closed (detach model).
func (o *Orchestrator) KillOnCloseJobs() {
	o.mu.RLock()
	var terms []*terminal.Terminal
	for _, id := range o.order {
		job := o.jobs[id]
		if !job.Spec().ShouldKillOnClose() {
			continue
		}
		if term := job.Terminal(); term != nil && term.PID() != 0 {
			terms = append(terms, term)
		}
	}
	o.mu.RUnlock()

	for _, term := range terms {
		term.Kill()
	}
}

// Shutdown gracefully stops every running process managed by this orchestrator.
// It first sends SIGTERM (on Unix) and waits up to 5 seconds, then force-kills
// any processes that haven't exited.
func (o *Orchestrator) Shutdown() {
	o.mu.RLock()
	var terms []*terminal.Terminal
	var doneChans []<-chan struct{}
	var pendingJobs []*Job
	for _, id := range o.order {
		job := o.jobs[id]
		if term := job.Terminal(); term != nil {
			terms = append(terms, term)
			doneChans = append(doneChans, term.Done())
		} else {
			// Job has no terminal — either pending or skipped.
			pendingJobs = append(pendingJobs, job)
		}
	}
	o.mu.RUnlock()

	// Close started/done channels on pending jobs so their runJob goroutines
	// can unblock and exit.
	for _, job := range pendingJobs {
		job.closeStarted()
		job.closeDone()
	}

	if len(terms) == 0 {
		return
	}

	// Phase 1: graceful stop (SIGTERM on Unix, no-op on Windows).
	for _, term := range terms {
		term.Stop()
	}

	// Wait up to 5 seconds for all processes to exit gracefully.
	allDone := make(chan struct{})
	go func() {
		for _, ch := range doneChans {
			<-ch
		}
		close(allDone)
	}()

	select {
	case <-allDone:
		return
	case <-time.After(5 * time.Second):
	}

	// Phase 2: force kill remaining.
	for _, term := range terms {
		term.Kill()
	}
}
