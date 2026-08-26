package server

import (
	"context"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

type fakeModelStore struct {
	list      []registry.Model
	err       error
	deleteErr error
	deleted   []string
}

func (f *fakeModelStore) ListModels() ([]registry.Model, error) {
	return f.list, f.err
}

func (f *fakeModelStore) DeleteModel(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeDatasetStore struct {
	datasets     []registry.DatasetSummary
	examples     map[string][]registry.Example
	createErr    error
	created      []string
	appendErr    error
	appended     map[string][]registry.Example
	listErr      error
	examplesErrs map[string]error
	deleteErr    error
	deleted      []string

	updateExampleErr error
	updatedExamples  map[int]registry.Example
	deleteExampleErr error
	deletedExamples  []int
}

func newFakeDatasetStore() *fakeDatasetStore {
	return &fakeDatasetStore{
		examples:        make(map[string][]registry.Example),
		appended:        make(map[string][]registry.Example),
		updatedExamples: make(map[int]registry.Example),
	}
}

func (f *fakeDatasetStore) ListDatasets() ([]registry.DatasetSummary, error) {
	return f.datasets, f.listErr
}

func (f *fakeDatasetStore) CreateDataset(name string) (registry.Dataset, error) {
	if f.createErr != nil {
		return registry.Dataset{}, f.createErr
	}
	f.created = append(f.created, name)
	return registry.Dataset{Name: name}, nil
}

func (f *fakeDatasetStore) ListExamples(name string) ([]registry.Example, error) {
	if err, ok := f.examplesErrs[name]; ok {
		return nil, err
	}
	return f.examples[name], nil
}

func (f *fakeDatasetStore) DeleteDataset(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeDatasetStore) AppendExamples(name string, examples []registry.Example) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.appended[name] = append(f.appended[name], examples...)
	return nil
}

func (f *fakeDatasetStore) UpdateExample(name string, index int, example registry.Example) error {
	if f.updateExampleErr != nil {
		return f.updateExampleErr
	}
	f.updatedExamples[index] = example
	return nil
}

func (f *fakeDatasetStore) DeleteExample(name string, index int) error {
	if f.deleteExampleErr != nil {
		return f.deleteExampleErr
	}
	f.deletedExamples = append(f.deletedExamples, index)
	return nil
}

type fakeGenerator struct {
	result []registry.Example
	err    error
}

func (f *fakeGenerator) Variations(ctx context.Context, model string, seed registry.Example, n int) ([]registry.Example, error) {
	return f.result, f.err
}

type fakeTrainingManager struct {
	startErr error
	started  []training.Config
	run      *training.Run
	runs     []*training.Run
	getOK    bool

	cancelErr     error
	cancelledRuns []string
}

func (f *fakeTrainingManager) StartRun(cfg training.Config) (*training.Run, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.started = append(f.started, cfg)
	if f.run != nil {
		return f.run, nil
	}
	return &training.Run{ID: "run-1", Config: cfg, Status: training.StatusRunning}, nil
}

func (f *fakeTrainingManager) CancelRun(id string) error {
	if f.cancelErr != nil {
		return f.cancelErr
	}
	f.cancelledRuns = append(f.cancelledRuns, id)
	return nil
}

func (f *fakeTrainingManager) ListRuns() []*training.Run {
	return f.runs
}

func (f *fakeTrainingManager) GetRun(id string) (*training.Run, bool) {
	if f.run != nil && f.run.ID == id {
		return f.run, true
	}
	return nil, f.getOK
}

type fakeEnvironmentStore struct {
	list      []registry.Environment
	listErr   error
	saveErr   error
	saved     []registry.Environment
	deleteErr error
	deleted   []string
}

func (f *fakeEnvironmentStore) ListEnvironments() ([]registry.Environment, error) {
	return f.list, f.listErr
}

func (f *fakeEnvironmentStore) SaveEnvironment(e registry.Environment) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, e)
	return nil
}

func (f *fakeEnvironmentStore) DeleteEnvironment(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeEnvironmentManager struct {
	launchResult environments.Instance
	launchErr    error
	launched     []string

	stopErr    error
	stoppedIDs []string

	listResult []environments.Instance
	listErr    error

	execResult *environments.Exec
	execErr    error
	execCalls  []string

	getExecResult *environments.Exec
	getExecOK     bool
}

func (f *fakeEnvironmentManager) Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error) {
	f.launched = append(f.launched, environmentName)
	return f.launchResult, f.launchErr
}

func (f *fakeEnvironmentManager) Stop(ctx context.Context, instanceID string) error {
	f.stoppedIDs = append(f.stoppedIDs, instanceID)
	return f.stopErr
}

func (f *fakeEnvironmentManager) ListInstances(ctx context.Context) ([]environments.Instance, error) {
	return f.listResult, f.listErr
}

func (f *fakeEnvironmentManager) StartExec(instanceID, command string) (*environments.Exec, error) {
	f.execCalls = append(f.execCalls, command)
	return f.execResult, f.execErr
}

func (f *fakeEnvironmentManager) GetExec(id string) (*environments.Exec, bool) {
	return f.getExecResult, f.getExecOK
}

type fakeAgentStore struct {
	list      []registry.Agent
	listErr   error
	saveErr   error
	saved     []registry.Agent
	get       registry.Agent
	getErr    error
	deleteErr error
	deleted   []string
}

func (f *fakeAgentStore) ListAgents() ([]registry.Agent, error) {
	return f.list, f.listErr
}

func (f *fakeAgentStore) SaveAgent(a registry.Agent) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, a)
	return nil
}

func (f *fakeAgentStore) GetAgent(name string) (registry.Agent, error) {
	return f.get, f.getErr
}

func (f *fakeAgentStore) DeleteAgent(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeAgentManager struct {
	startResult *agents.Run
	startErr    error
	started     []string

	stopErr     error
	stoppedRuns []string

	messageResult agents.ChatMessage
	messageErr    error
	messages      []string

	getResult *agents.Run
	getOK     bool
}

func (f *fakeAgentManager) StartRun(agentName string) (*agents.Run, error) {
	f.started = append(f.started, agentName)
	return f.startResult, f.startErr
}

func (f *fakeAgentManager) StopRun(runID string) error {
	f.stoppedRuns = append(f.stoppedRuns, runID)
	return f.stopErr
}

func (f *fakeAgentManager) SendMessage(runID, message string) (agents.ChatMessage, error) {
	f.messages = append(f.messages, message)
	return f.messageResult, f.messageErr
}

func (f *fakeAgentManager) GetRun(id string) (*agents.Run, bool) {
	return f.getResult, f.getOK
}

type fakeEvaluationStore struct {
	list      []registry.Evaluation
	listErr   error
	saveErr   error
	saved     []registry.Evaluation
	get       registry.Evaluation
	getErr    error
	deleteErr error
	deleted   []string
}

func (f *fakeEvaluationStore) ListEvaluations() ([]registry.Evaluation, error) {
	return f.list, f.listErr
}

func (f *fakeEvaluationStore) SaveEvaluation(e registry.Evaluation) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, e)
	return nil
}

func (f *fakeEvaluationStore) GetEvaluation(name string) (registry.Evaluation, error) {
	return f.get, f.getErr
}

func (f *fakeEvaluationStore) DeleteEvaluation(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeEvaluationManager struct {
	startResult *evaluations.Run
	startErr    error
	started     []string

	runs []*evaluations.Run

	getResult *evaluations.Run
	getOK     bool
}

func (f *fakeEvaluationManager) StartRun(evaluationName string, agentNames []string) (*evaluations.Run, error) {
	f.started = append(f.started, evaluationName)
	return f.startResult, f.startErr
}

func (f *fakeEvaluationManager) ListRuns() []*evaluations.Run {
	return f.runs
}

func (f *fakeEvaluationManager) GetRun(id string) (*evaluations.Run, bool) {
	return f.getResult, f.getOK
}

// testDeps builds a minimal Deps with working fakes for tests that don't
// care about the Models/Dataset/Training/Environments/Agents/Evaluations API
// surface.
func testDeps() Deps {
	return Deps{
		Bus:          eventbus.New(),
		Models:       &fakeModelStore{},
		Datasets:     newFakeDatasetStore(),
		Generator:    &fakeGenerator{},
		Training:     &fakeTrainingManager{},
		Environments: &fakeEnvironmentStore{},
		Instances:    &fakeEnvironmentManager{},
		Agents:       &fakeAgentStore{},
		AgentRuns:    &fakeAgentManager{},
		Evaluations:  &fakeEvaluationStore{},
		EvalRuns:     &fakeEvaluationManager{},
	}
}
