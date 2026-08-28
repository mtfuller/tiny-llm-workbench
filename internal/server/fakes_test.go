package server

import (
	"context"
	"fmt"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/benchmarks"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/huggingface"
	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
)

type fakeModelStore struct {
	list      []registry.Model
	err       error
	getErr    error
	saveErr   error
	saved     []registry.Model
	deleteErr error
	deleted   []string
}

func (f *fakeModelStore) ListModels() ([]registry.Model, error) {
	return f.list, f.err
}

func (f *fakeModelStore) SaveModel(m registry.Model) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, m)
	f.list = append(f.list, m)
	return nil
}

func (f *fakeModelStore) GetModel(name string) (registry.Model, error) {
	if f.getErr != nil {
		return registry.Model{}, f.getErr
	}
	for _, m := range f.list {
		if m.Name == name {
			return m, nil
		}
	}
	return registry.Model{}, fmt.Errorf("model %q not found", name)
}

func (f *fakeModelStore) DeleteModel(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeModelRunner struct {
	completion string
	err        error
	calls      []fakeModelRunnerCall

	positions      []mlxrunner.TokenPosition
	tokenProbErr   error
	tokenProbCalls []fakeTokenProbCall
}

type fakeModelRunnerCall struct {
	model    string
	messages []mlxrunner.ChatMessage
}

type fakeTokenProbCall struct {
	model     string
	prompt    string
	maxTokens int
	topN      int
}

func (f *fakeModelRunner) Chat(ctx context.Context, model string, messages []mlxrunner.ChatMessage) (string, error) {
	f.calls = append(f.calls, fakeModelRunnerCall{model: model, messages: messages})
	if f.err != nil {
		return "", f.err
	}
	return f.completion, nil
}

func (f *fakeModelRunner) TokenProbabilities(ctx context.Context, model, prompt string, maxTokens, topN int) ([]mlxrunner.TokenPosition, error) {
	f.tokenProbCalls = append(f.tokenProbCalls, fakeTokenProbCall{model: model, prompt: prompt, maxTokens: maxTokens, topN: topN})
	if f.tokenProbErr != nil {
		return nil, f.tokenProbErr
	}
	return f.positions, nil
}

type fakeDatasetStore struct {
	datasets     []registry.DatasetSummary
	examples     map[string][]registry.Example
	metadata     map[string]registry.Dataset
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

func (f *fakeDatasetStore) CreateDataset(name, title, description string) (registry.Dataset, error) {
	if f.createErr != nil {
		return registry.Dataset{}, f.createErr
	}
	f.created = append(f.created, name)
	dataset := registry.Dataset{Name: name, Title: title, Description: description}
	if f.metadata == nil {
		f.metadata = make(map[string]registry.Dataset)
	}
	f.metadata[name] = dataset
	return dataset, nil
}

func (f *fakeDatasetStore) GetDataset(name string) (registry.Dataset, error) {
	if d, ok := f.metadata[name]; ok {
		return d, nil
	}
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
	get       registry.Environment
	getErr    error
	deleteErr error
	deleted   []string

	updateConfigErr error
	updatedConfigs  []registry.Environment
	attachErr       error
	attached        []string
	detachErr       error
	detached        []string
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

func (f *fakeEnvironmentStore) GetEnvironment(name string) (registry.Environment, error) {
	return f.get, f.getErr
}

func (f *fakeEnvironmentStore) DeleteEnvironment(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeEnvironmentStore) UpdateConfig(name, image string, mounts []registry.Mount) error {
	if f.updateConfigErr != nil {
		return f.updateConfigErr
	}
	f.updatedConfigs = append(f.updatedConfigs, registry.Environment{Name: name, Image: image, Mounts: mounts})
	return nil
}

func (f *fakeEnvironmentStore) AttachTool(name, toolName string) error {
	if f.attachErr != nil {
		return f.attachErr
	}
	f.attached = append(f.attached, toolName)
	return nil
}

func (f *fakeEnvironmentStore) DetachTool(name, toolName string) error {
	if f.detachErr != nil {
		return f.detachErr
	}
	f.detached = append(f.detached, toolName)
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

	tryToolResult *environments.Exec
	tryToolErr    error
	tryToolCalls  []string
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

func (f *fakeEnvironmentManager) TryTool(instanceID string, tool registry.Tool, args map[string]string) (*environments.Exec, error) {
	f.tryToolCalls = append(f.tryToolCalls, instanceID)
	return f.tryToolResult, f.tryToolErr
}

type fakeToolStore struct {
	list      []registry.Tool
	listErr   error
	saveErr   error
	saved     []registry.Tool
	get       registry.Tool
	getErr    error
	deleteErr error
	deleted   []string
}

func (f *fakeToolStore) ListTools() ([]registry.Tool, error) {
	return f.list, f.listErr
}

func (f *fakeToolStore) SaveTool(tool registry.Tool) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, tool)
	return nil
}

func (f *fakeToolStore) GetTool(name string) (registry.Tool, error) {
	return f.get, f.getErr
}

func (f *fakeToolStore) DeleteTool(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeKnowledgeStore struct {
	list      []registry.KnowledgeBase
	listErr   error
	saveErr   error
	saved     []registry.KnowledgeBase
	get       registry.KnowledgeBase
	getErr    error
	deleteErr error
	deleted   []string

	addRecordsErr    error
	addedRecords     [][]registry.KnowledgeRecord
	updateRecordErr  error
	updatedRecords   []registry.KnowledgeRecord
	updatedRecordIdx []int
	deleteRecordErr  error
	deletedRecordIdx []int
}

func (f *fakeKnowledgeStore) ListKnowledgeBases() ([]registry.KnowledgeBase, error) {
	return f.list, f.listErr
}

func (f *fakeKnowledgeStore) SaveKnowledgeBase(kb registry.KnowledgeBase) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, kb)
	return nil
}

func (f *fakeKnowledgeStore) GetKnowledgeBase(name string) (registry.KnowledgeBase, error) {
	return f.get, f.getErr
}

func (f *fakeKnowledgeStore) DeleteKnowledgeBase(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeKnowledgeStore) AddRecords(name string, records []registry.KnowledgeRecord) error {
	if f.addRecordsErr != nil {
		return f.addRecordsErr
	}
	f.addedRecords = append(f.addedRecords, records)
	return nil
}

func (f *fakeKnowledgeStore) UpdateRecord(name string, index int, record registry.KnowledgeRecord) error {
	if f.updateRecordErr != nil {
		return f.updateRecordErr
	}
	f.updatedRecords = append(f.updatedRecords, record)
	f.updatedRecordIdx = append(f.updatedRecordIdx, index)
	return nil
}

func (f *fakeKnowledgeStore) DeleteRecord(name string, index int) error {
	if f.deleteRecordErr != nil {
		return f.deleteRecordErr
	}
	f.deletedRecordIdx = append(f.deletedRecordIdx, index)
	return nil
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

	debugStartResult   *agents.DebugState
	debugStartErr      error
	debugStarted       []string
	debugMessageResult *agents.DebugState
	debugMessageErr    error
	debugMessages      []string
	debugStepResult    *agents.DebugState
	debugStepErr       error
	debugRetryResult   *agents.DebugState
	debugRetryErr      error
	debugGetResult     *agents.DebugState
	debugGetOK         bool
	debugStopErr       error
	debugStoppedRuns   []string
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

func (f *fakeAgentManager) StartDebugRun(agentName string, graph registry.Graph, environment string) (*agents.DebugState, error) {
	f.debugStarted = append(f.debugStarted, agentName)
	return f.debugStartResult, f.debugStartErr
}

func (f *fakeAgentManager) SendDebugMessage(id, message string) (*agents.DebugState, error) {
	f.debugMessages = append(f.debugMessages, message)
	return f.debugMessageResult, f.debugMessageErr
}

func (f *fakeAgentManager) StepDebugRun(id string) (*agents.DebugState, error) {
	return f.debugStepResult, f.debugStepErr
}

func (f *fakeAgentManager) RetryDebugRun(id string) (*agents.DebugState, error) {
	return f.debugRetryResult, f.debugRetryErr
}

func (f *fakeAgentManager) GetDebugRun(id string) (*agents.DebugState, bool) {
	return f.debugGetResult, f.debugGetOK
}

func (f *fakeAgentManager) StopDebugRun(id string) error {
	f.debugStoppedRuns = append(f.debugStoppedRuns, id)
	return f.debugStopErr
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

	updateEnvErr    error
	updatedEnv      string
	updatedEnvEvals []string

	addTestCasesErr error
	addedTestCases  [][]registry.TestCase

	updateTestCaseErr error
	updatedTestCase   registry.TestCase
	updatedIndex      int

	deleteTestCaseErr error
	deletedIndex      int

	publishErr    error
	publishResult registry.EvaluationVersion
	published     []string

	versions    []registry.EvaluationVersion
	versionsErr error
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

func (f *fakeEvaluationStore) UpdateEnvironment(name, environment string) (registry.Evaluation, error) {
	if f.updateEnvErr != nil {
		return registry.Evaluation{}, f.updateEnvErr
	}
	f.updatedEnvEvals = append(f.updatedEnvEvals, name)
	f.updatedEnv = environment
	f.get.Environment = environment
	return f.get, nil
}

func (f *fakeEvaluationStore) AddEvaluationTestCases(evaluationName string, tcs []registry.TestCase) error {
	if f.addTestCasesErr != nil {
		return f.addTestCasesErr
	}
	f.addedTestCases = append(f.addedTestCases, tcs)
	return nil
}

func (f *fakeEvaluationStore) UpdateEvaluationTestCase(evaluationName string, index int, tc registry.TestCase) error {
	if f.updateTestCaseErr != nil {
		return f.updateTestCaseErr
	}
	f.updatedIndex = index
	f.updatedTestCase = tc
	return nil
}

func (f *fakeEvaluationStore) DeleteEvaluationTestCase(evaluationName string, index int) error {
	if f.deleteTestCaseErr != nil {
		return f.deleteTestCaseErr
	}
	f.deletedIndex = index
	return nil
}

func (f *fakeEvaluationStore) PublishEvaluationVersion(evaluationName string) (registry.EvaluationVersion, error) {
	if f.publishErr != nil {
		return registry.EvaluationVersion{}, f.publishErr
	}
	f.published = append(f.published, evaluationName)
	return f.publishResult, nil
}

func (f *fakeEvaluationStore) ListEvaluationVersions(evaluationName string) ([]registry.EvaluationVersion, error) {
	return f.versions, f.versionsErr
}

type fakeEvaluationManager struct {
	startResult *evaluations.Run
	startErr    error
	started     []string

	runs []*evaluations.Run

	getResult *evaluations.Run
	getOK     bool

	results    []evaluations.RunResult
	resultsErr error
}

func (f *fakeEvaluationManager) StartRun(evaluationName string, version int, agentNames []string) (*evaluations.Run, error) {
	f.started = append(f.started, evaluationName)
	return f.startResult, f.startErr
}

func (f *fakeEvaluationManager) ListRuns() []*evaluations.Run {
	return f.runs
}

func (f *fakeEvaluationManager) GetRun(id string) (*evaluations.Run, bool) {
	return f.getResult, f.getOK
}

func (f *fakeEvaluationManager) ListResults(evaluationName string) ([]evaluations.RunResult, error) {
	return f.results, f.resultsErr
}

type fakeBenchmarkStore struct {
	list      []registry.Benchmark
	listErr   error
	saveErr   error
	saved     []registry.Benchmark
	get       registry.Benchmark
	getErr    error
	deleteErr error
	deleted   []string

	addTestCasesErr error
	addedTestCases  [][]registry.TestCase

	updateTestCaseErr error
	updatedTestCase   registry.TestCase
	updatedIndex      int

	deleteTestCaseErr error
	deletedIndex      int

	publishErr    error
	publishResult registry.BenchmarkVersion
	published     []string

	versions    []registry.BenchmarkVersion
	versionsErr error
}

func (f *fakeBenchmarkStore) ListBenchmarks() ([]registry.Benchmark, error) {
	return f.list, f.listErr
}

func (f *fakeBenchmarkStore) SaveBenchmark(b registry.Benchmark) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, b)
	return nil
}

func (f *fakeBenchmarkStore) GetBenchmark(name string) (registry.Benchmark, error) {
	return f.get, f.getErr
}

func (f *fakeBenchmarkStore) DeleteBenchmark(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeBenchmarkStore) AddTestCases(benchmarkName string, tcs []registry.TestCase) error {
	if f.addTestCasesErr != nil {
		return f.addTestCasesErr
	}
	f.addedTestCases = append(f.addedTestCases, tcs)
	return nil
}

func (f *fakeBenchmarkStore) UpdateTestCase(benchmarkName string, index int, tc registry.TestCase) error {
	if f.updateTestCaseErr != nil {
		return f.updateTestCaseErr
	}
	f.updatedIndex = index
	f.updatedTestCase = tc
	return nil
}

func (f *fakeBenchmarkStore) DeleteTestCase(benchmarkName string, index int) error {
	if f.deleteTestCaseErr != nil {
		return f.deleteTestCaseErr
	}
	f.deletedIndex = index
	return nil
}

func (f *fakeBenchmarkStore) PublishVersion(benchmarkName string) (registry.BenchmarkVersion, error) {
	if f.publishErr != nil {
		return registry.BenchmarkVersion{}, f.publishErr
	}
	f.published = append(f.published, benchmarkName)
	return f.publishResult, nil
}

func (f *fakeBenchmarkStore) ListVersions(benchmarkName string) ([]registry.BenchmarkVersion, error) {
	return f.versions, f.versionsErr
}

type fakeTestCaseGenerator struct {
	prompts  []string
	err      error
	gotModel string
	gotSeed  string
	gotN     int
}

func (f *fakeTestCaseGenerator) Variations(ctx context.Context, model, seedPrompt string, n int) ([]string, error) {
	f.gotModel = model
	f.gotSeed = seedPrompt
	f.gotN = n
	return f.prompts, f.err
}

type fakeBenchmarkManager struct {
	startResult *benchmarks.Run
	startErr    error
	started     []string

	runs []*benchmarks.Run

	getResult *benchmarks.Run
	getOK     bool

	results    []benchmarks.RunResult
	resultsErr error
}

func (f *fakeBenchmarkManager) StartRun(benchmarkName string, version int, modelNames []string) (*benchmarks.Run, error) {
	f.started = append(f.started, benchmarkName)
	return f.startResult, f.startErr
}

func (f *fakeBenchmarkManager) ListRuns() []*benchmarks.Run {
	return f.runs
}

func (f *fakeBenchmarkManager) GetRun(id string) (*benchmarks.Run, bool) {
	return f.getResult, f.getOK
}

func (f *fakeBenchmarkManager) ListResults(benchmarkName string) ([]benchmarks.RunResult, error) {
	return f.results, f.resultsErr
}

// testDeps builds a minimal Deps with working fakes for tests that don't
// care about the Models/Dataset/Training/Environments/Agents/Evaluations/
// Benchmarks API surface.
type fakeHFSearcher struct {
	results []huggingface.Model
	err     error
	queries []string
}

func (f *fakeHFSearcher) SearchModels(ctx context.Context, query string) ([]huggingface.Model, error) {
	f.queries = append(f.queries, query)
	return f.results, f.err
}

func testDeps() Deps {
	return Deps{
		Bus:          eventbus.New(),
		Models:       &fakeModelStore{},
		ModelRunner:  &fakeModelRunner{},
		HuggingFace:  &fakeHFSearcher{},
		Datasets:     newFakeDatasetStore(),
		Generator:    &fakeGenerator{},
		Training:     &fakeTrainingManager{},
		Environments: &fakeEnvironmentStore{},
		Instances:    &fakeEnvironmentManager{},
		Tools:        &fakeToolStore{},
		Knowledge:    &fakeKnowledgeStore{},
		Agents:       &fakeAgentStore{},
		AgentRuns:    &fakeAgentManager{},
		Evaluations:  &fakeEvaluationStore{},
		EvalRuns:     &fakeEvaluationManager{},
		Benchmarks:   &fakeBenchmarkStore{},
		BenchRuns:    &fakeBenchmarkManager{},
		TestCaseGen:  &fakeTestCaseGenerator{},
	}
}
