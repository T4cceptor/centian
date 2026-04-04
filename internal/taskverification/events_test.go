package taskverification

import (
	"encoding/json"
	"testing"

	"gotest.tools/assert"
)

func TestRecordTaskEventAddsRunStatusToPayload(t *testing.T) {
	store := NewInMemoryEventStore()
	service := &Service{EventStore: store}
	run := &RunState{
		RunID:      "tr_1742947200123_0000000001",
		TemplateID: "python_tdd_demo",
		Status:     TaskStatusCompleted,
		Phase:      TaskPhaseExecution,
	}

	// Given: a task event with existing payload fields but no explicit status.
	err := service.RecordTaskEvent(
		run,
		"session-1",
		"principal-1",
		"",
		"",
		TaskPhaseExecution,
		WorkflowNodeKindExecution,
		TaskPhaseExecution,
		WorkflowNodeKindExecution,
		TaskEventTypeStepCompleted,
		TaskEventOutcomeSucceeded,
		"req_1742947200123_0000000002",
		map[string]any{"step": 2},
	)

	// Then: the persisted payload includes the current run status.
	assert.NilError(t, err)
	events, err := store.TaskEvents()
	assert.NilError(t, err)
	assert.Equal(t, len(events), 1)

	var payload map[string]any
	err = json.Unmarshal(events[0].Payload, &payload)
	assert.NilError(t, err)
	assert.Equal(t, payload["status"], string(TaskStatusCompleted))
	assert.Equal(t, int(payload["step"].(float64)), 2)
}
