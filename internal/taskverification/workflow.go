package taskverification

import (
	"fmt"
	"sort"
	"strings"
)

var supportedPlanningOutputs = map[string]struct{}{
	"selectedFiles":        {},
	"testTarget":           {},
	"lintCommand":          {},
	"expectedFailure":      {},
	"implementationTarget": {},
	"invariants":           {},
}

//nolint:gocyclo // Workflow compilation is the central normalization pass; helper extraction keeps nested rules contained.
func (t *Template) compileWorkflow() (*CompiledWorkflow, error) {
	if err := validateWorkflowDefinition(t); err != nil {
		return nil, err
	}

	compiled := newCompiledWorkflow()
	if err := addCompiledNode(compiled, buildOnboardingNode(t.Workflow.Onboarding)); err != nil {
		return nil, err
	}
	if err := addCompiledNode(compiled, buildPlanningNode(t.Workflow.Planning)); err != nil {
		return nil, err
	}

	seenIDs := make(map[string]string)
	declaredNext := make(map[TaskPhase]string)
	orderedLeafPaths := make([]TaskPhase, 0)
	executionSteps := make([]Step, 0)
	stepNumber := 0

	var compileExecutionNodes func(nodes []ExecutionNodeSpec, ancestorIDs []string, logicalParent TaskPhase) error
	compileExecutionNodes = func(nodes []ExecutionNodeSpec, ancestorIDs []string, logicalParent TaskPhase) error {
		for index := range nodes {
			nodeSpec := nodes[index]
			if strings.TrimSpace(nodeSpec.ID) == "" {
				return fmt.Errorf("workflow.execution node id is required")
			}
			if previous, exists := seenIDs[nodeSpec.ID]; exists {
				return fmt.Errorf("duplicate workflow node id %q in %s and %s", nodeSpec.ID, previous, nodeSpec.ID)
			}
			kind := nodeSpec.Kind
			if kind == "" {
				kind = WorkflowNodeKindExecution
			}
			if kind != WorkflowNodeKindExecution && kind != WorkflowNodeKindWaitingForApproval {
				return fmt.Errorf("workflow.execution node %q has unsupported kind %q", nodeSpec.ID, kind)
			}

			ids := append(append([]string(nil), ancestorIDs...), nodeSpec.ID)
			logicalPath := buildExecutionPath(ids)
			path := logicalPath
			if kind == WorkflowNodeKindWaitingForApproval {
				path = buildApprovalPath(ids)
			}
			seenIDs[nodeSpec.ID] = string(path)

			if len(nodeSpec.SubSteps) > 0 {
				if kind != WorkflowNodeKindExecution {
					return fmt.Errorf("workflow node %q cannot define sub_steps for kind %q", nodeSpec.ID, kind)
				}
				if len(nodeSpec.Checks) > 0 || len(nodeSpec.Invariants) > 0 {
					return fmt.Errorf("workflow node %q cannot define checks or invariants when sub_steps are present", nodeSpec.ID)
				}
				if strings.TrimSpace(nodeSpec.Next) != "" {
					return fmt.Errorf("workflow node %q cannot define next when sub_steps are present", nodeSpec.ID)
				}
				if err := compileExecutionNodes(nodeSpec.SubSteps, ids, logicalPath); err != nil {
					return err
				}
				continue
			}

			if kind == WorkflowNodeKindExecution {
				step := Step{
					ID:           nodeSpec.ID,
					Path:         path,
					ParentPath:   logicalParent,
					Name:         nodeSpec.Name,
					Description:  nodeSpec.Description,
					Instructions: nodeSpec.Instructions,
					AllowedTools: cloneStringSlice(nodeSpec.AllowedTools),
					Checkpoint:   cloneCheckpoint(nodeSpec.Checkpoint),
					Checks:       cloneChecks(nodeSpec.Checks),
					Invariants:   cloneInvariants(nodeSpec.Invariants),
				}
				if err := validateStep(stepNumber, &step, make(map[string]struct{})); err != nil {
					return fmt.Errorf("workflow node %q: %w", nodeSpec.ID, err)
				}
				stepNumber++
				executionSteps = append(executionSteps, step)
				if err := addCompiledNode(compiled, WorkflowNode{
					Path:         path,
					Kind:         WorkflowNodeKindExecution,
					ParentPath:   logicalParent,
					StepNumber:   stepNumber,
					StepID:       nodeSpec.ID,
					Name:         nodeSpec.Name,
					Description:  nodeSpec.Description,
					Instructions: nodeSpec.Instructions,
					AllowedTools: cloneStringSlice(nodeSpec.AllowedTools),
					Checkpoint:   cloneCheckpoint(nodeSpec.Checkpoint),
				}); err != nil {
					return err
				}
			} else {
				if len(nodeSpec.Checks) > 0 || len(nodeSpec.Invariants) > 0 {
					return fmt.Errorf("workflow node %q cannot define checks or invariants for kind %q", nodeSpec.ID, kind)
				}
				if err := addCompiledNode(compiled, WorkflowNode{
					Path:         path,
					Kind:         WorkflowNodeKindWaitingForApproval,
					ParentPath:   logicalParent,
					Name:         nodeSpec.Name,
					Description:  nodeSpec.Description,
					Instructions: nodeSpec.Instructions,
					AllowedTools: cloneStringSlice(nodeSpec.AllowedTools),
					Checkpoint:   cloneCheckpoint(nodeSpec.Checkpoint),
				}); err != nil {
					return err
				}
			}

			orderedLeafPaths = append(orderedLeafPaths, path)
			if next := strings.TrimSpace(nodeSpec.Next); next != "" {
				declaredNext[path] = next
			}
		}
		return nil
	}

	if err := compileExecutionNodes(t.Workflow.Execution, nil, TaskPhaseExecution); err != nil {
		return nil, err
	}
	if len(executionSteps) == 0 {
		return nil, fmt.Errorf("workflow.execution must define at least one executable node")
	}

	compiled.ExecutionSteps = executionSteps
	compiled.FirstExecutablePath = executionSteps[0].Path

	stepByPath := make(map[TaskPhase]int, len(compiled.ExecutionSteps))
	for index := range compiled.ExecutionSteps {
		step := &compiled.ExecutionSteps[index]
		stepByPath[step.Path] = index
	}

	planningNext := strings.TrimSpace(t.Workflow.Planning.Next)
	if planningNext == "" {
		planningNext = string(compiled.FirstExecutablePath)
	}
	if err := setNodeNext(compiled.Nodes, TaskPhasePlanning, planningNext); err != nil {
		return nil, fmt.Errorf("workflow.planning.next: %w", err)
	}

	for index, path := range orderedLeafPaths {
		next := ""
		if explicit, exists := declaredNext[path]; exists {
			next = explicit
		} else if index+1 < len(orderedLeafPaths) {
			next = string(orderedLeafPaths[index+1])
		}
		if err := setNodeNext(compiled.Nodes, path, next); err != nil {
			return nil, fmt.Errorf("workflow node %q next: %w", path, err)
		}
		if stepIndex, exists := stepByPath[path]; exists {
			compiled.ExecutionSteps[stepIndex].NextPath = compiled.Nodes[path].NextPath
		}
	}

	for _, path := range orderedLeafPaths {
		node := compiled.Nodes[path]
		if node.Kind == WorkflowNodeKindWaitingForApproval && node.NextPath == "" {
			return nil, fmt.Errorf("workflow node %q cannot be a terminal waiting_for_approval node", path)
		}
	}

	if err := validatePlanningEditableFields(t, t.Workflow.Planning); err != nil {
		return nil, err
	}
	if err := validatePlanningRequiredOutputs(t.Workflow.Planning.RequiredOutputs); err != nil {
		return nil, err
	}
	if err := validateWorkflowReachability(compiled); err != nil {
		return nil, err
	}
	return compiled, nil
}

func buildExecutionPath(ids []string) TaskPhase {
	return TaskPhase(strings.Join(append([]string{string(TaskPhaseExecution)}, ids...), "."))
}

func validateWorkflowDefinition(t *Template) error {
	if t == nil {
		return fmt.Errorf("template is required")
	}
	if t.Workflow == nil {
		return fmt.Errorf("workflow is required")
	}
	if t.Workflow.Onboarding == nil {
		return fmt.Errorf("workflow.onboarding is required")
	}
	if t.Workflow.Planning == nil {
		return fmt.Errorf("workflow.planning is required")
	}
	if len(t.Workflow.Execution) == 0 {
		return fmt.Errorf("workflow.execution must define at least one node")
	}
	return nil
}

func newCompiledWorkflow() *CompiledWorkflow {
	return &CompiledWorkflow{
		Nodes:          make(map[TaskPhase]WorkflowNode),
		OnboardingPath: TaskPhaseOnboarding,
		PlanningPath:   TaskPhasePlanning,
	}
}

func addCompiledNode(compiled *CompiledWorkflow, node WorkflowNode) error {
	if _, exists := compiled.Nodes[node.Path]; exists {
		return fmt.Errorf("duplicate workflow path %q", node.Path)
	}
	compiled.Nodes[node.Path] = node
	return nil
}

func buildOnboardingNode(spec *LifecycleNodeSpec) WorkflowNode {
	return WorkflowNode{
		Path:         TaskPhaseOnboarding,
		Kind:         WorkflowNodeKindOnboarding,
		NextPath:     TaskPhasePlanning,
		Instructions: spec.Instructions,
		AllowedTools: cloneStringSlice(spec.AllowedTools),
		Checkpoint:   cloneCheckpoint(spec.Checkpoint),
	}
}

func buildPlanningNode(spec *PlanningNodeSpec) WorkflowNode {
	return WorkflowNode{
		Path:                    TaskPhasePlanning,
		Kind:                    WorkflowNodeKindPlanning,
		Instructions:            spec.Instructions,
		AllowedTools:            cloneStringSlice(spec.AllowedTools),
		Checkpoint:              cloneCheckpoint(spec.Checkpoint),
		EditableFields:          cloneStringSlice(spec.EditableFields),
		RequiredPlanningOutputs: cloneStringSlice(spec.RequiredOutputs),
	}
}

func buildApprovalPath(ids []string) TaskPhase {
	return TaskPhase(strings.Join(append([]string{string(TaskPhaseWaitingForApproval)}, ids...), "."))
}

func setNodeNext(nodes map[TaskPhase]WorkflowNode, path TaskPhase, next string) error {
	node, exists := nodes[path]
	if !exists {
		return fmt.Errorf("unknown workflow node %q", path)
	}
	if strings.TrimSpace(next) == "" {
		node.NextPath = ""
		nodes[path] = node
		return nil
	}
	nextPath := TaskPhase(strings.TrimSpace(next))
	target, exists := nodes[nextPath]
	if !exists {
		return fmt.Errorf("unknown workflow node %q", nextPath)
	}
	switch target.Kind {
	case WorkflowNodeKindExecution, WorkflowNodeKindWaitingForApproval:
	default:
		return fmt.Errorf("workflow node %q cannot target %q", path, nextPath)
	}
	if nextPath == path {
		return fmt.Errorf("workflow node %q cannot target itself", path)
	}
	node.NextPath = nextPath
	nodes[path] = node
	return nil
}

func validatePlanningEditableFields(t *Template, planning *PlanningNodeSpec) error {
	if planning == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(planning.EditableFields))
	defined := make(map[string]struct{}, len(t.Parameters))
	for _, parameter := range t.ParameterDefinitions() {
		defined[parameter.Name] = struct{}{}
	}
	for index, field := range planning.EditableFields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return fmt.Errorf("workflow.planning.editable_fields[%d] is required", index)
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("workflow.planning.editable_fields contains duplicate value %q", trimmed)
		}
		seen[trimmed] = struct{}{}
		if !strings.HasPrefix(trimmed, "parameters.") {
			return fmt.Errorf("workflow.planning.editable_fields %q is unsupported", trimmed)
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "parameters."))
		if name == "" {
			return fmt.Errorf("workflow.planning.editable_fields %q is invalid", trimmed)
		}
		if _, exists := defined[name]; !exists {
			return fmt.Errorf("workflow.planning.editable_fields references unknown parameter %q", name)
		}
	}
	return nil
}

func validatePlanningRequiredOutputs(outputs []string) error {
	seen := make(map[string]struct{}, len(outputs))
	for index, output := range outputs {
		trimmed := strings.TrimSpace(output)
		if trimmed == "" {
			return fmt.Errorf("workflow.planning.required_outputs[%d] is required", index)
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("workflow.planning.required_outputs contains duplicate value %q", trimmed)
		}
		seen[trimmed] = struct{}{}
		if _, supported := supportedPlanningOutputs[trimmed]; !supported {
			return fmt.Errorf("workflow.planning.required_outputs %q is unsupported", trimmed)
		}
	}
	return nil
}

func validateWorkflowReachability(compiled *CompiledWorkflow) error {
	if compiled == nil {
		return fmt.Errorf("compiled workflow is required")
	}
	visited := make(map[TaskPhase]bool, len(compiled.Nodes))
	inStack := make(map[TaskPhase]bool, len(compiled.Nodes))
	var visit func(TaskPhase) error
	visit = func(path TaskPhase) error {
		if inStack[path] {
			return fmt.Errorf("workflow cycle detected at %q", path)
		}
		if visited[path] {
			return nil
		}
		node, exists := compiled.Nodes[path]
		if !exists {
			return fmt.Errorf("workflow references unknown path %q", path)
		}
		inStack[path] = true
		visited[path] = true
		if node.NextPath != "" {
			if err := visit(node.NextPath); err != nil {
				return err
			}
		}
		delete(inStack, path)
		return nil
	}

	if err := visit(compiled.OnboardingPath); err != nil {
		return err
	}

	paths := make([]string, 0, len(compiled.Nodes))
	for path := range compiled.Nodes {
		if !visited[path] {
			paths = append(paths, string(path))
		}
	}
	if len(paths) > 0 {
		sort.Strings(paths)
		return fmt.Errorf("workflow node %q is unreachable", paths[0])
	}
	return nil
}

func cloneChecks(checks []Check) []Check {
	if len(checks) == 0 {
		return nil
	}
	cloned := make([]Check, 0, len(checks))
	for _, check := range checks {
		cloned = append(cloned, Check{
			ID:             check.ID,
			Command:        check.Command,
			PreConditions:  append([]Condition(nil), check.PreConditions...),
			PostConditions: append([]Condition(nil), check.PostConditions...),
		})
	}
	return cloned
}

func cloneInvariants(invariants []Invariant) []Invariant {
	if len(invariants) == 0 {
		return nil
	}
	cloned := make([]Invariant, len(invariants))
	copy(cloned, invariants)
	return cloned
}

func cloneCheckpoint(checkpoint *CheckpointHint) *CheckpointHint {
	if checkpoint == nil {
		return nil
	}
	cloned := *checkpoint
	return &cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

// WorkflowNode returns one compiled workflow node by path.
func (t *Template) WorkflowNode(path TaskPhase) (WorkflowNode, bool) {
	if t == nil || t.CompiledWorkflow == nil {
		return WorkflowNode{}, false
	}
	node, exists := t.CompiledWorkflow.Nodes[path]
	return node, exists
}

// CurrentNode returns the active compiled workflow node for the run.
func (r *RunState) CurrentNode() (WorkflowNode, bool) {
	template := r.currentTemplate()
	if template == nil {
		return WorkflowNode{}, false
	}
	return template.WorkflowNode(r.Phase)
}

// NextNodePath returns the deterministic next workflow node when known.
func (r *RunState) NextNodePath() TaskPhase {
	node, exists := r.CurrentNode()
	if !exists {
		return ""
	}
	return node.NextPath
}

func (r *RunState) currentTemplate() *Template {
	if r == nil {
		return nil
	}
	if r.ExecutionTemplate != nil {
		return r.ExecutionTemplate
	}
	return &r.SelectedTemplate
}
