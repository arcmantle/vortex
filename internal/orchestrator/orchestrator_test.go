package orchestrator

import (
	"testing"

	"arcmantle/vortex/internal/config"
)

func TestShouldCarryPersistentJob(t *testing.T) {
	runningJob := func(spec config.JobSpec) *Job {
		j := newJob(spec)
		j.setStatus(StatusRunning)
		return j
	}
	tests := []struct {
		name               string
		oldJob             *Job
		newSpec            config.JobSpec
		nodeRuntimeChanged bool
		want               bool
	}{
		{
			name:               "carry plain persistent job when runtime unchanged",
			oldJob:             runningJob(config.JobSpec{ID: "plain", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "plain", Restart: boolPtr(false)},
			nodeRuntimeChanged: false,
			want:               true,
		},
		{
			name:               "carry plain persistent job when node runtime changed",
			oldJob:             runningJob(config.JobSpec{ID: "plain", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "plain", Restart: boolPtr(false)},
			nodeRuntimeChanged: true,
			want:               true,
		},
		{
			name:               "restart shared node job when runtime changed",
			oldJob:             runningJob(config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)},
			nodeRuntimeChanged: true,
			want:               false,
		},
		{
			name:               "carry shared node job when runtime unchanged",
			oldJob:             runningJob(config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)},
			nodeRuntimeChanged: false,
			want:               true,
		},
		{
			name:               "do not carry normal restarting job",
			oldJob:             runningJob(config.JobSpec{ID: "restart", Restart: boolPtr(true)}),
			newSpec:            config.JobSpec{ID: "restart", Restart: boolPtr(true)},
			nodeRuntimeChanged: false,
			want:               false,
		},
		{
			name:               "do not carry exited persistent job",
			oldJob:             newJob(config.JobSpec{ID: "exited", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "exited", Restart: boolPtr(false)},
			nodeRuntimeChanged: false,
			want:               false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldCarryPersistentJob(tc.oldJob, tc.newSpec, tc.nodeRuntimeChanged)
			if got != tc.want {
				t.Fatalf("shouldCarryPersistentJob() = %v, want %v", got, tc.want)
			}
		})
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestCloseStartedAndDoneAreIdempotent(t *testing.T) {
	job := newJob(config.JobSpec{ID: "safe-close"})
	// Calling close helpers multiple times must not panic.
	job.closeStarted()
	job.closeStarted()
	job.closeDone()
	job.closeDone()
}

func TestUpdateSpecIsSafeForConcurrentReads(t *testing.T) {
	job := newJob(config.JobSpec{ID: "orig", Label: "Original"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			spec := job.Spec()
			_ = spec.ID
			_ = spec.Label
		}
	}()

	for i := 0; i < 1000; i++ {
		job.updateSpec(config.JobSpec{ID: "updated", Label: "Updated"})
	}
	<-done
}
