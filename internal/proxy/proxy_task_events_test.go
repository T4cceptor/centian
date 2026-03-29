package proxy

import (
	"testing"

	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

func TestSnapshotTaskRunCopiesMutableState(t *testing.T) {
	run := &taskverification.RunState{
		RunID:  "run-1",
		Status: taskverification.TaskStatusActive,
		Phase:  taskverification.TaskPhasePlanning,
		SelectedTemplate: taskverification.Template{
			CompiledWorkflow: &taskverification.CompiledWorkflow{
				Nodes: map[taskverification.TaskPhase]taskverification.WorkflowNode{
					taskverification.TaskPhasePlanning: {
						Path: taskverification.TaskPhasePlanning,
						Kind: taskverification.WorkflowNodeKindPlanning,
					},
				},
			},
		},
	}

	snapshot := snapshotTaskRun(run)
	run.RunID = "run-2"
	run.Status = taskverification.TaskStatusFailed
	run.Phase = taskverification.TaskPhaseExecution

	assert.Equal(t, snapshot.RunID, "run-1")
	assert.Equal(t, snapshot.Status, taskverification.TaskStatusActive)
	assert.Equal(t, snapshot.Phase, taskverification.TaskPhasePlanning)
	assert.Equal(t, snapshot.NodeKind, taskverification.WorkflowNodeKindPlanning)
}
