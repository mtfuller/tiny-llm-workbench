package evaluations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeEvaluationReader struct {
	evals map[string]registry.Evaluation
	err   error
}

func (f *fakeEvaluationReader) GetEvaluation(name string) (registry.Evaluation, error) {
	if f.err != nil {
		return registry.Evaluation{}, f.err
	}
	eval, ok := f.evals[name]
	if !ok {
		return registry.Evaluation{}, errors.New("not found")
	}
	return eval, nil
}

// fakeAgentRunner replies to every SendMessage with the next canned reply
// keyed by prompt, so different test cases can get different replies.
type fakeAgentRunner struct {
	repliesByPrompt map[string]string
	startErr        error
	sendErr         error
	startedAgents   []string
	sentPrompts     []string
}

func (f *fakeAgentRunner) StartRun(agentName string) (*agents.Run, error) {
	f.startedAgents = append(f.startedAgents, agentName)
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &agents.Run{ID: "run-" + agentName, AgentName: agentName}, nil
}

func (f *fakeAgentRunner) SendMessage(runID, message string) (agents.ChatMessage, error) {
	f.sentPrompts = append(f.sentPrompts, message)
	if f.sendErr != nil {
		return agents.ChatMessage{}, f.sendErr
	}
	return agents.ChatMessage{Role: "assistant", Content: f.repliesByPrompt[message]}, nil
}

type fakeEnvironmentLauncher struct {
	launchResult environments.Instance
	launchErr    error
	launched     []string
	stoppedIDs   []string
}

func (f *fakeEnvironmentLauncher) Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, environmentName)
	return f.launchResult, f.launchErr
}

func (f *fakeEnvironmentLauncher) Stop(ctx context.Context, instanceID string) error {
	f.stoppedIDs = append(f.stoppedIDs, instanceID)
	return nil
}

func simpleEvaluation(name string) registry.Evaluation {
	return registry.Evaluation{
		Name: name,
		TestCases: []registry.TestCase{
			{ID: "tc1", Prompt: "say hi", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
			{ID: "tc2", Prompt: "say bye", Assertions: []registry.Assertion{{Type: "contains", Value: "bye"}}},
		},
	}
}

func waitForRunStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) *Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if run, ok := m.GetRun(id); ok && run.Status == want {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach status %q within %s", id, want, timeout)
	return nil
}

func TestStartRunSucceedsWithMixedResults(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")}}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "see you later"}}
	m := NewManager(context.Background(), evalReader, runner, &fakeEnvironmentLauncher{}, eventbus.New())

	run, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.Status != StatusRunning {
		t.Errorf("StartRun().Status = %q, want %q", run.Status, StatusRunning)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(finished.AgentResults) != 1 {
		t.Fatalf("finished.AgentResults = %+v, want 1 agent", finished.AgentResults)
	}

	ar := finished.AgentResults[0]
	if ar.AgentName != "greeter" || ar.Total != 2 || ar.Passed != 1 {
		t.Errorf("AgentResult = %+v, want greeter with 1/2 passed (tc2's reply doesn't contain 'bye')", ar)
	}
	if !ar.Results[0].Passed {
		t.Errorf("Results[0] = %+v, want passed (reply contains 'hello')", ar.Results[0])
	}
	if ar.Results[1].Passed {
		t.Errorf("Results[1] = %+v, want failed (reply doesn't contain 'bye')", ar.Results[1])
	}
}

func TestStartRunRequiresAgents(t *testing.T) {
	m := NewManager(context.Background(), &fakeEvaluationReader{}, &fakeAgentRunner{}, &fakeEnvironmentLauncher{}, eventbus.New())

	if _, err := m.StartRun("greeting", nil); err == nil {
		t.Error("StartRun() error = nil, want an error when no agents are given")
	}
}

func TestStartRunUnknownEvaluation(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{}}
	m := NewManager(context.Background(), evalReader, &fakeAgentRunner{}, &fakeEnvironmentLauncher{}, eventbus.New())

	if _, err := m.StartRun("does-not-exist", []string{"greeter"}); err == nil {
		t.Error("StartRun() error = nil, want an error for an unknown evaluation")
	}
}

func TestStartRunEmptyEvaluation(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"empty": {Name: "empty"}}}
	m := NewManager(context.Background(), evalReader, &fakeAgentRunner{}, &fakeEnvironmentLauncher{}, eventbus.New())

	if _, err := m.StartRun("empty", []string{"greeter"}); err == nil {
		t.Error("StartRun() error = nil, want an error for an evaluation with no test cases")
	}
}

func TestStartRunLaunchesAndStopsEnvironment(t *testing.T) {
	eval := simpleEvaluation("greeting")
	eval.Environment = "WebSearch"
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"greeting": eval}}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	envs := &fakeEnvironmentLauncher{launchResult: environments.Instance{ID: "container-1"}}
	m := NewManager(context.Background(), evalReader, runner, envs, eventbus.New())

	run, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if finished.InstanceID != "container-1" {
		t.Errorf("finished.InstanceID = %q, want %q", finished.InstanceID, "container-1")
	}
	if len(envs.launched) != 1 || envs.launched[0] != "WebSearch" {
		t.Errorf("envs.launched = %v, want [WebSearch]", envs.launched)
	}
	if len(envs.stoppedIDs) != 1 || envs.stoppedIDs[0] != "container-1" {
		t.Errorf("envs.stoppedIDs = %v, want [container-1]", envs.stoppedIDs)
	}
}

func TestStartRunEnvironmentLaunchFailureFailsRun(t *testing.T) {
	eval := simpleEvaluation("greeting")
	eval.Environment = "WebSearch"
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"greeting": eval}}
	envs := &fakeEnvironmentLauncher{launchErr: errors.New("docker daemon unreachable")}
	m := NewManager(context.Background(), evalReader, &fakeAgentRunner{}, envs, eventbus.New())

	run, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusFailed, time.Second)
	if finished.Error == "" {
		t.Error("finished.Error is empty, want the launch failure recorded")
	}
}

func TestStartRunAgentStartFailureRecordsErrorPerTestCase(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")}}
	runner := &fakeAgentRunner{startErr: errors.New("no such agent")}
	m := NewManager(context.Background(), evalReader, runner, &fakeEnvironmentLauncher{}, eventbus.New())

	run, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	ar := finished.AgentResults[0]
	if ar.Passed != 0 || ar.Total != 2 {
		t.Errorf("AgentResult = %+v, want 0/2 passed", ar)
	}
	for _, r := range ar.Results {
		if r.Error == "" {
			t.Errorf("TestCaseResult = %+v, want an error recorded", r)
		}
	}
}

func TestListRunsMostRecentFirst(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")}}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := NewManager(context.Background(), evalReader, runner, &fakeEnvironmentLauncher{}, eventbus.New())

	first, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, first.ID, StatusSucceeded, time.Second)

	time.Sleep(2 * time.Millisecond)
	second, err := m.StartRun("greeting", []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() (second) error = %v", err)
	}
	waitForRunStatus(t, m, second.ID, StatusSucceeded, time.Second)

	runs := m.ListRuns()
	if len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Errorf("ListRuns() = %+v, want [second, first]", runs)
	}
}
