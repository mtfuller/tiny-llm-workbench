package evaluations

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
)

type fakeEvaluationReader struct {
	evals    map[string]registry.Evaluation
	versions map[string]map[int]registry.EvaluationVersion
	err      error
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

func (f *fakeEvaluationReader) GetEvaluationVersion(name string, version int) (registry.EvaluationVersion, error) {
	byVersion, ok := f.versions[name]
	if !ok {
		return registry.EvaluationVersion{}, errors.New("no such evaluation")
	}
	v, ok := byVersion[version]
	if !ok {
		return registry.EvaluationVersion{}, errors.New("no such version")
	}
	return v, nil
}

// fakeAgentRunner replies to every SendMessage with the next canned reply
// keyed by prompt, so different test cases can get different replies.
type fakeAgentRunner struct {
	repliesByPrompt map[string]string
	startErr        error
	sendErr         error
	startedAgents   []string
	startedInInst   []string // instanceIDs passed to StartRunInInstance
	sentPrompts     []string
	stoppedRunIDs   []string
}

func (f *fakeAgentRunner) StartRun(agentName, workspaceOverride string) (*agents.Run, error) {
	f.startedAgents = append(f.startedAgents, agentName)
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &agents.Run{ID: "run-" + agentName, AgentName: agentName}, nil
}

func (f *fakeAgentRunner) StartRunInInstance(agentName, instanceID string) (*agents.Run, error) {
	f.startedAgents = append(f.startedAgents, agentName)
	f.startedInInst = append(f.startedInInst, instanceID)
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &agents.Run{ID: "run-" + agentName, AgentName: agentName, InstanceID: instanceID}, nil
}

func (f *fakeAgentRunner) SendMessage(runID, message string) (agents.ChatMessage, error) {
	f.sentPrompts = append(f.sentPrompts, message)
	if f.sendErr != nil {
		return agents.ChatMessage{}, f.sendErr
	}
	return agents.ChatMessage{Role: "assistant", Content: f.repliesByPrompt[message]}, nil
}

func (f *fakeAgentRunner) StopRun(runID string) error {
	f.stoppedRunIDs = append(f.stoppedRunIDs, runID)
	return nil
}

type fakeWorkspaceLauncher struct {
	launchResult environments.Instance
	launchErr    error
	launched     []string
	stoppedIDs   []string

	// toolOutputByCommand, if set, returns a canned output for a given
	// command; commands not present return ("", nil).
	toolOutputByCommand map[string]string
	toolErrByCommand    map[string]error
	ranCommands         []string
}

func (f *fakeWorkspaceLauncher) Launch(ctx context.Context, workspaceName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, workspaceName)
	return f.launchResult, f.launchErr
}

func (f *fakeWorkspaceLauncher) Stop(ctx context.Context, instanceID string) error {
	f.stoppedIDs = append(f.stoppedIDs, instanceID)
	return nil
}

func (f *fakeWorkspaceLauncher) RunToolSync(ctx context.Context, instanceID, command string) (string, error) {
	f.ranCommands = append(f.ranCommands, command)
	if f.toolErrByCommand != nil {
		if err, ok := f.toolErrByCommand[command]; ok {
			return f.toolOutputByCommand[command], err
		}
	}
	return f.toolOutputByCommand[command], nil
}

func simpleEvaluation(name string) registry.Evaluation {
	return registry.Evaluation{Name: name, Version: 1}
}

func simpleVersion() registry.EvaluationVersion {
	return registry.EvaluationVersion{
		Version: 1,
		TestCases: []registry.TestCase{
			{ID: "tc1", Prompt: "say hi", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
			{ID: "tc2", Prompt: "say bye", Assertions: []registry.Assertion{{Type: "contains", Value: "bye"}}},
		},
	}
}

func newTestManager(t *testing.T, evalReader evaluationReader, runner agentRunner, envs workspaceLauncher) *Manager {
	t.Helper()
	return NewManager(context.Background(), evalReader, runner, envs, eventbus.New(), filepath.Join(t.TempDir(), "results"))
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
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")},
		versions: map[string]map[int]registry.EvaluationVersion{"greeting": {1: simpleVersion()}},
	}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "see you later"}}
	m := newTestManager(t, evalReader, runner, &fakeWorkspaceLauncher{})

	run, err := m.StartRun("greeting", 1, []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if run.Status != StatusRunning {
		t.Errorf("StartRun().Status = %q, want %q", run.Status, StatusRunning)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	if len(finished.Results) != 1 {
		t.Fatalf("finished.Results = %+v, want 1 agent", finished.Results)
	}

	ar := finished.Results[0]
	if ar.AgentName != "greeter" || ar.Total != 2 || ar.Passed != 1 {
		t.Errorf("RunResult = %+v, want greeter with 1/2 passed (tc2's reply doesn't contain 'bye')", ar)
	}
	if !ar.Results[0].Passed {
		t.Errorf("Results[0] = %+v, want passed (reply contains 'hello')", ar.Results[0])
	}
	if ar.Results[1].Passed {
		t.Errorf("Results[1] = %+v, want failed (reply doesn't contain 'bye')", ar.Results[1])
	}

	// No workspace on the test cases, so agents run via plain StartRun.
	if len(runner.startedInInst) != 0 {
		t.Errorf("runner.startedInInst = %v, want none — no workspace configured", runner.startedInInst)
	}
}

func TestStartRunRequiresAgents(t *testing.T) {
	m := newTestManager(t, &fakeEvaluationReader{}, &fakeAgentRunner{}, &fakeWorkspaceLauncher{})

	if _, err := m.StartRun("greeting", 1, nil); err == nil {
		t.Error("StartRun() error = nil, want an error when no agents are given")
	}
}

func TestStartRunUnknownEvaluation(t *testing.T) {
	evalReader := &fakeEvaluationReader{evals: map[string]registry.Evaluation{}}
	m := newTestManager(t, evalReader, &fakeAgentRunner{}, &fakeWorkspaceLauncher{})

	if _, err := m.StartRun("does-not-exist", 1, []string{"greeter"}); err == nil {
		t.Error("StartRun() error = nil, want an error for an unknown evaluation")
	}
}

func TestStartRunUnknownVersion(t *testing.T) {
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")},
		versions: map[string]map[int]registry.EvaluationVersion{},
	}
	m := newTestManager(t, evalReader, &fakeAgentRunner{}, &fakeWorkspaceLauncher{})

	if _, err := m.StartRun("greeting", 1, []string{"greeter"}); err == nil {
		t.Error("StartRun() error = nil, want an error for a version that hasn't been published")
	}
}

func TestStartRunEmptyVersion(t *testing.T) {
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"empty": {Name: "empty", Version: 1}},
		versions: map[string]map[int]registry.EvaluationVersion{"empty": {1: {Version: 1}}},
	}
	m := newTestManager(t, evalReader, &fakeAgentRunner{}, &fakeWorkspaceLauncher{})

	if _, err := m.StartRun("empty", 1, []string{"greeter"}); err == nil {
		t.Error("StartRun() error = nil, want an error for a version with no test cases")
	}
}

func TestRunTestCaseLaunchesWorkspaceAndVerifiesInSameInstance(t *testing.T) {
	ver := registry.EvaluationVersion{
		Version: 1,
		TestCases: []registry.TestCase{
			{
				ID:         "tc1",
				Prompt:     "write hello.txt",
				Workspace:  "repo-scenario",
				Assertions: []registry.Assertion{{Type: "contains", Value: "done"}},
				VerifyCommands: []registry.VerifyStep{
					{Command: "cat /workspace/hello.txt", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
				},
			},
		},
	}
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"coding": {Name: "coding", Version: 1}},
		versions: map[string]map[int]registry.EvaluationVersion{"coding": {1: ver}},
	}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"write hello.txt": "done"}}
	envs := &fakeWorkspaceLauncher{
		launchResult:        environments.Instance{ID: "container-1"},
		toolOutputByCommand: map[string]string{"cat /workspace/hello.txt": "hello world"},
	}
	m := newTestManager(t, evalReader, runner, envs)

	run, err := m.StartRun("coding", 1, []string{"coder"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	ar := finished.Results[0]
	if ar.Passed != 1 || ar.Total != 1 {
		t.Fatalf("RunResult = %+v, want 1/1 passed", ar)
	}
	tcResult := ar.Results[0]
	if tcResult.InstanceID != "container-1" {
		t.Errorf("TestCaseResult.InstanceID = %q, want %q", tcResult.InstanceID, "container-1")
	}
	if len(tcResult.VerifyResults) != 1 || !tcResult.VerifyResults[0].Passed {
		t.Errorf("TestCaseResult.VerifyResults = %+v, want a single passing verify step", tcResult.VerifyResults)
	}

	// The agent's turn used the SAME instance the workspace was launched into
	// (via StartRunInInstance), and Verify ran there too.
	if len(envs.ranCommands) != 1 || envs.ranCommands[0] != "cat /workspace/hello.txt" {
		t.Errorf("envs.ranCommands = %v, want [cat /workspace/hello.txt]", envs.ranCommands)
	}
	if len(runner.startedInInst) != 1 || runner.startedInInst[0] != "container-1" {
		t.Errorf("runner.startedInInst = %v, want [container-1] — the agent must act in the same instance Verify uses", runner.startedInInst)
	}
	if len(envs.launched) != 1 || envs.launched[0] != "repo-scenario" {
		t.Errorf("envs.launched = %v, want [repo-scenario]", envs.launched)
	}
	if len(envs.stoppedIDs) != 1 || envs.stoppedIDs[0] != "container-1" {
		t.Errorf("envs.stoppedIDs = %v, want [container-1] — Evaluations owns this instance's lifecycle", envs.stoppedIDs)
	}
	if len(runner.stoppedRunIDs) != 1 {
		t.Errorf("runner.stoppedRunIDs = %v, want the agent run cleaned up too", runner.stoppedRunIDs)
	}
}

func TestRunTestCaseWorkspaceLaunchFailureFailsTestCaseWithoutRunningAgent(t *testing.T) {
	ver := registry.EvaluationVersion{
		Version: 1,
		TestCases: []registry.TestCase{
			{ID: "tc1", Prompt: "write hello.txt", Workspace: "repo-scenario"},
		},
	}
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"coding": {Name: "coding", Version: 1}},
		versions: map[string]map[int]registry.EvaluationVersion{"coding": {1: ver}},
	}
	runner := &fakeAgentRunner{}
	envs := &fakeWorkspaceLauncher{launchErr: errors.New("docker daemon unreachable")}
	m := newTestManager(t, evalReader, runner, envs)

	run, err := m.StartRun("coding", 1, []string{"coder"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	tcResult := finished.Results[0].Results[0]
	if tcResult.Error == "" {
		t.Error("TestCaseResult.Error is empty, want the workspace launch failure recorded")
	}
	if len(runner.startedAgents) != 0 {
		t.Errorf("runner.startedAgents = %v, want none — a failed workspace launch must not start the agent", runner.startedAgents)
	}
}

func TestRunTestCaseVerifyWithoutWorkspaceFailsWithClearError(t *testing.T) {
	ver := registry.EvaluationVersion{
		Version: 1,
		TestCases: []registry.TestCase{
			{
				ID:     "tc1",
				Prompt: "write hello.txt",
				VerifyCommands: []registry.VerifyStep{
					{Command: "cat /workspace/hello.txt", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
				},
			},
		},
	}
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"coding": {Name: "coding", Version: 1}},
		versions: map[string]map[int]registry.EvaluationVersion{"coding": {1: ver}},
	}
	runner := &fakeAgentRunner{}
	envs := &fakeWorkspaceLauncher{}
	m := newTestManager(t, evalReader, runner, envs)

	run, err := m.StartRun("coding", 1, []string{"coder"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	tcResult := finished.Results[0].Results[0]
	if tcResult.Error == "" {
		t.Error("TestCaseResult.Error is empty, want a clear misconfiguration error")
	}
	if len(envs.launched) != 0 {
		t.Errorf("envs.launched = %v, want none — there's no workspace to launch", envs.launched)
	}
	if len(runner.startedAgents) != 0 {
		t.Errorf("runner.startedAgents = %v, want none", runner.startedAgents)
	}
}

func TestRunTestCaseVerifyCommandFailingAssertionFailsTestCaseEvenWithPassingReply(t *testing.T) {
	ver := registry.EvaluationVersion{
		Version: 1,
		TestCases: []registry.TestCase{
			{
				ID:         "tc1",
				Prompt:     "write hello.txt",
				Workspace:  "repo-scenario",
				Assertions: []registry.Assertion{{Type: "contains", Value: "done"}},
				VerifyCommands: []registry.VerifyStep{
					{Command: "cat /workspace/hello.txt", Assertions: []registry.Assertion{{Type: "contains", Value: "hello"}}},
				},
			},
		},
	}
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"coding": {Name: "coding", Version: 1}},
		versions: map[string]map[int]registry.EvaluationVersion{"coding": {1: ver}},
	}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"write hello.txt": "done"}}
	envs := &fakeWorkspaceLauncher{
		launchResult:        environments.Instance{ID: "container-1"},
		toolOutputByCommand: map[string]string{"cat /workspace/hello.txt": "the file is empty"},
	}
	m := newTestManager(t, evalReader, runner, envs)

	run, err := m.StartRun("coding", 1, []string{"coder"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	tcResult := finished.Results[0].Results[0]
	if tcResult.Passed {
		t.Error("TestCaseResult.Passed = true, want false — the verify command's assertion failed")
	}
	if len(tcResult.VerifyResults) != 1 || tcResult.VerifyResults[0].Passed {
		t.Errorf("TestCaseResult.VerifyResults = %+v, want a single failing verify step", tcResult.VerifyResults)
	}
}

func TestStartRunAgentStartFailureRecordsErrorPerTestCase(t *testing.T) {
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")},
		versions: map[string]map[int]registry.EvaluationVersion{"greeting": {1: simpleVersion()}},
	}
	runner := &fakeAgentRunner{startErr: errors.New("no such agent")}
	m := newTestManager(t, evalReader, runner, &fakeWorkspaceLauncher{})

	run, err := m.StartRun("greeting", 1, []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	finished := waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)
	ar := finished.Results[0]
	if ar.Passed != 0 || ar.Total != 2 {
		t.Errorf("RunResult = %+v, want 0/2 passed", ar)
	}
	for _, r := range ar.Results {
		if r.Error == "" {
			t.Errorf("TestCaseResult = %+v, want an error recorded", r)
		}
	}
}

func TestListRunsMostRecentFirst(t *testing.T) {
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")},
		versions: map[string]map[int]registry.EvaluationVersion{"greeting": {1: simpleVersion()}},
	}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := newTestManager(t, evalReader, runner, &fakeWorkspaceLauncher{})

	first, err := m.StartRun("greeting", 1, []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, first.ID, StatusSucceeded, time.Second)

	time.Sleep(2 * time.Millisecond)
	second, err := m.StartRun("greeting", 1, []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() (second) error = %v", err)
	}
	waitForRunStatus(t, m, second.ID, StatusSucceeded, time.Second)

	runs := m.ListRuns()
	if len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Errorf("ListRuns() = %+v, want [second, first]", runs)
	}
}

func TestListResultsPersistsAcrossRuns(t *testing.T) {
	evalReader := &fakeEvaluationReader{
		evals:    map[string]registry.Evaluation{"greeting": simpleEvaluation("greeting")},
		versions: map[string]map[int]registry.EvaluationVersion{"greeting": {1: simpleVersion()}},
	}
	runner := &fakeAgentRunner{repliesByPrompt: map[string]string{"say hi": "hello!", "say bye": "bye!"}}
	m := newTestManager(t, evalReader, runner, &fakeWorkspaceLauncher{})

	run, err := m.StartRun("greeting", 1, []string{"greeter"})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	waitForRunStatus(t, m, run.ID, StatusSucceeded, time.Second)

	results, err := m.ListResults("greeting")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 1 || results[0].AgentName != "greeter" || results[0].EvaluationVersion != 1 {
		t.Errorf("ListResults() = %+v, want a single greeter/v1 result", results)
	}
}

func TestListResultsEmptyForUnknownEvaluation(t *testing.T) {
	m := newTestManager(t, &fakeEvaluationReader{}, &fakeAgentRunner{}, &fakeWorkspaceLauncher{})

	results, err := m.ListResults("never-run")
	if err != nil {
		t.Fatalf("ListResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ListResults() = %+v, want empty", results)
	}
}
