package orchestrator

import (
	"testing"

	"arcmantle/vortex/internal/config"
)

func TestShouldCarryPersistentJob(t *testing.T) {
	tests := []struct {
		name               string
		oldJob             *Job
		newSpec            config.JobSpec
		nodeRuntimeChanged bool
		want               bool
	}{
		{
			name:               "carry plain persistent job when runtime unchanged",
			oldJob:             newJob(config.JobSpec{ID: "plain", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "plain", Restart: boolPtr(false)},
			nodeRuntimeChanged: false,
			want:               true,
		},
		{
			name:               "carry plain persistent job when node runtime changed",
			oldJob:             newJob(config.JobSpec{ID: "plain", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "plain", Restart: boolPtr(false)},
			nodeRuntimeChanged: true,
			want:               true,
		},
		{
			name:               "restart shared node job when runtime changed",
			oldJob:             newJob(config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)},
			nodeRuntimeChanged: true,
			want:               false,
		},
		{
			name:               "carry shared node job when runtime unchanged",
			oldJob:             newJob(config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)}),
			newSpec:            config.JobSpec{ID: "node-job", Use: "node", Restart: boolPtr(false)},
			nodeRuntimeChanged: false,
			want:               true,
		},
		{
			name:               "do not carry normal restarting job",
			oldJob:             newJob(config.JobSpec{ID: "restart", Restart: boolPtr(true)}),
			newSpec:            config.JobSpec{ID: "restart", Restart: boolPtr(true)},
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
