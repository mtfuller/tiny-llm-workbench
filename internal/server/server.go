// Package server implements the local webserver that serves the embedded
// browser UI, the Models/Dataset/Training JSON API, and a live event stream
// over Server-Sent Events.
package server

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/benchmarks"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/mlxrunner"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
	"github.com/mtfuller/tiny-llm-workbench/web"
)

// modelStore is the subset of registry.Registry the server needs for model
// endpoints.
type modelStore interface {
	ListModels() ([]registry.Model, error)
	GetModel(name string) (registry.Model, error)
	DeleteModel(name string) error
}

// modelRunner is the subset of mlxrunner.Runner the server needs to chat
// with a registry model from the Models detail page's "Run model" modal,
// and to compute its token-probability visualization.
type modelRunner interface {
	Chat(ctx context.Context, model string, messages []mlxrunner.ChatMessage) (string, error)
	TokenProbabilities(ctx context.Context, model, prompt string, maxTokens, topN int) ([]mlxrunner.TokenPosition, error)
}

// datasetStore is the subset of registry.Registry the server needs for
// dataset endpoints.
type datasetStore interface {
	ListDatasets() ([]registry.DatasetSummary, error)
	CreateDataset(name, title, description string) (registry.Dataset, error)
	GetDataset(name string) (registry.Dataset, error)
	DeleteDataset(name string) error
	ListExamples(name string) ([]registry.Example, error)
	AppendExamples(name string, examples []registry.Example) error
	UpdateExample(name string, index int, example registry.Example) error
	DeleteExample(name string, index int) error
}

// variationGenerator is the subset of datasetgen.Generator the server needs.
type variationGenerator interface {
	Variations(ctx context.Context, model string, seed registry.Example, n int) ([]registry.Example, error)
}

// trainingManager is the subset of training.Manager the server needs.
type trainingManager interface {
	StartRun(cfg training.Config) (*training.Run, error)
	CancelRun(id string) error
	ListRuns() []*training.Run
	GetRun(id string) (*training.Run, bool)
}

// environmentStore is the subset of registry.Registry the server needs for
// Environment definitions.
type environmentStore interface {
	ListEnvironments() ([]registry.Environment, error)
	SaveEnvironment(e registry.Environment) error
	DeleteEnvironment(name string) error
}

// environmentManager is the subset of environments.Manager the server needs.
type environmentManager interface {
	Launch(ctx context.Context, environmentName, instanceName string) (environments.Instance, error)
	Stop(ctx context.Context, instanceID string) error
	ListInstances(ctx context.Context) ([]environments.Instance, error)
	StartExec(instanceID, command string) (*environments.Exec, error)
	GetExec(id string) (*environments.Exec, bool)
}

// agentStore is the subset of registry.Registry the server needs for Agent
// definitions.
type agentStore interface {
	ListAgents() ([]registry.Agent, error)
	SaveAgent(a registry.Agent) error
	GetAgent(name string) (registry.Agent, error)
	DeleteAgent(name string) error
}

// agentManager is the subset of agents.Manager the server needs.
type agentManager interface {
	StartRun(agentName string) (*agents.Run, error)
	StopRun(runID string) error
	SendMessage(runID, message string) (agents.ChatMessage, error)
	GetRun(id string) (*agents.Run, bool)
}

// evaluationStore is the subset of registry.Registry the server needs for
// Evaluation definitions.
type evaluationStore interface {
	ListEvaluations() ([]registry.Evaluation, error)
	SaveEvaluation(e registry.Evaluation) error
	GetEvaluation(name string) (registry.Evaluation, error)
	DeleteEvaluation(name string) error
}

// evaluationManager is the subset of evaluations.Manager the server needs.
type evaluationManager interface {
	StartRun(evaluationName string, agentNames []string) (*evaluations.Run, error)
	ListRuns() []*evaluations.Run
	GetRun(id string) (*evaluations.Run, bool)
}

// benchmarkStore is the subset of registry.Registry the server needs for
// Benchmark definitions.
type benchmarkStore interface {
	ListBenchmarks() ([]registry.Benchmark, error)
	SaveBenchmark(b registry.Benchmark) error
	GetBenchmark(name string) (registry.Benchmark, error)
	DeleteBenchmark(name string) error
	AddTestCases(benchmarkName string, tcs []registry.TestCase) error
	UpdateTestCase(benchmarkName string, index int, tc registry.TestCase) error
	DeleteTestCase(benchmarkName string, index int) error
}

// benchmarkManager is the subset of benchmarks.Manager the server needs.
type benchmarkManager interface {
	StartRun(benchmarkName string, modelNames []string) (*benchmarks.Run, error)
	ListRuns() []*benchmarks.Run
	GetRun(id string) (*benchmarks.Run, bool)
	ListResults(benchmarkName string) ([]benchmarks.RunResult, error)
}

// testCaseGenerator is the subset of testcasegen.Generator the server needs.
type testCaseGenerator interface {
	Variations(ctx context.Context, model, seedPrompt string, n int) ([]string, error)
}

// Deps are the server's dependencies, all provided by the caller so they can
// be swapped for fakes in tests.
type Deps struct {
	Bus          *eventbus.Bus
	Models       modelStore
	ModelRunner  modelRunner
	Datasets     datasetStore
	Generator    variationGenerator
	Training     trainingManager
	Environments environmentStore
	Instances    environmentManager
	Agents       agentStore
	AgentRuns    agentManager
	Evaluations  evaluationStore
	EvalRuns     evaluationManager
	Benchmarks   benchmarkStore
	BenchRuns    benchmarkManager
	TestCaseGen  testCaseGenerator

	// RegistryRoot is a plain config value (not behavior), shown read-only
	// on the Settings page.
	RegistryRoot string
}

// New builds the HTTP handler for the TLW webserver: the embedded browser UI
// at "/", the JSON API under "/api/", and the live event stream at
// "/api/events".
func New(deps Deps) (http.Handler, error) {
	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/", spaHandler(dist))
	mux.HandleFunc("GET /api/events", sseHandler(deps.Bus))
	mux.HandleFunc("GET /api/system", systemInfoHandler(deps.RegistryRoot))
	mux.HandleFunc("GET /api/models", listModelsHandler(deps.Models))
	mux.HandleFunc("GET /api/models/{name}", getModelHandler(deps.Models, deps.Training))
	mux.HandleFunc("DELETE /api/models/{name}", deleteModelHandler(deps.Models))
	mux.HandleFunc("POST /api/models/{name}/chat", chatWithModelHandler(deps.Models, deps.ModelRunner))
	mux.HandleFunc("GET /api/models/{name}/architecture", getModelArchitectureHandler(deps.Models))
	mux.HandleFunc("GET /api/models/{name}/heatmap", getModelHeatmapHandler(deps.Models))
	mux.HandleFunc("POST /api/models/{name}/token-probabilities", tokenProbabilitiesHandler(deps.Models, deps.ModelRunner))
	mux.HandleFunc("GET /api/datasets", listDatasetsHandler(deps.Datasets))
	mux.HandleFunc("POST /api/datasets", createDatasetHandler(deps.Datasets))
	mux.HandleFunc("GET /api/datasets/{name}", getDatasetHandler(deps.Datasets))
	mux.HandleFunc("DELETE /api/datasets/{name}", deleteDatasetHandler(deps.Datasets))
	mux.HandleFunc("POST /api/datasets/{name}/variations", generateVariationsHandler(deps.Datasets, deps.Generator))
	mux.HandleFunc("POST /api/datasets/{name}/examples", addExamplesHandler(deps.Datasets))
	mux.HandleFunc("PUT /api/datasets/{name}/examples/{index}", updateExampleHandler(deps.Datasets))
	mux.HandleFunc("DELETE /api/datasets/{name}/examples/{index}", deleteExampleHandler(deps.Datasets))
	mux.HandleFunc("GET /api/datasets/{name}/export", exportDatasetHandler(deps.Datasets))
	mux.HandleFunc("POST /api/datasets/{name}/import", importDatasetHandler(deps.Datasets))
	mux.HandleFunc("POST /api/training/runs", startTrainingRunHandler(deps.Training))
	mux.HandleFunc("GET /api/training/runs", listTrainingRunsHandler(deps.Training))
	mux.HandleFunc("GET /api/training/runs/{id}", getTrainingRunHandler(deps.Training))
	mux.HandleFunc("POST /api/training/runs/{id}/cancel", cancelTrainingRunHandler(deps.Training))
	mux.HandleFunc("GET /api/environments", listEnvironmentsHandler(deps.Environments))
	mux.HandleFunc("POST /api/environments", createEnvironmentHandler(deps.Environments))
	mux.HandleFunc("DELETE /api/environments/{name}", deleteEnvironmentHandler(deps.Environments))
	mux.HandleFunc("POST /api/environments/{name}/launch", launchEnvironmentHandler(deps.Instances))
	mux.HandleFunc("GET /api/environments/instances", listInstancesHandler(deps.Instances))
	mux.HandleFunc("POST /api/environments/instances/{id}/stop", stopInstanceHandler(deps.Instances))
	mux.HandleFunc("POST /api/environments/instances/{id}/exec", startExecHandler(deps.Instances))
	mux.HandleFunc("GET /api/environments/instances/{id}/execs/{execId}", getExecHandler(deps.Instances))
	mux.HandleFunc("GET /api/agents", listAgentsHandler(deps.Agents))
	mux.HandleFunc("POST /api/agents", saveAgentHandler(deps.Agents))
	mux.HandleFunc("GET /api/agents/{name}", getAgentHandler(deps.Agents))
	mux.HandleFunc("DELETE /api/agents/{name}", deleteAgentHandler(deps.Agents))
	mux.HandleFunc("POST /api/agents/{name}/runs", startAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("POST /api/agents/runs/{id}/messages", sendAgentMessageHandler(deps.AgentRuns))
	mux.HandleFunc("GET /api/agents/runs/{id}", getAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("POST /api/agents/runs/{id}/stop", stopAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("GET /api/evaluations", listEvaluationsHandler(deps.Evaluations))
	mux.HandleFunc("POST /api/evaluations", saveEvaluationHandler(deps.Evaluations))
	mux.HandleFunc("GET /api/evaluations/{name}", getEvaluationHandler(deps.Evaluations))
	mux.HandleFunc("DELETE /api/evaluations/{name}", deleteEvaluationHandler(deps.Evaluations))
	mux.HandleFunc("POST /api/evaluations/{name}/runs", startEvaluationRunHandler(deps.EvalRuns))
	mux.HandleFunc("GET /api/evaluations/runs", listEvaluationRunsHandler(deps.EvalRuns))
	mux.HandleFunc("GET /api/evaluations/runs/{id}", getEvaluationRunHandler(deps.EvalRuns))
	mux.HandleFunc("GET /api/benchmarks", listBenchmarksHandler(deps.Benchmarks))
	mux.HandleFunc("POST /api/benchmarks", saveBenchmarkHandler(deps.Benchmarks))
	mux.HandleFunc("GET /api/benchmarks/{name}", getBenchmarkHandler(deps.Benchmarks))
	mux.HandleFunc("DELETE /api/benchmarks/{name}", deleteBenchmarkHandler(deps.Benchmarks))
	mux.HandleFunc("POST /api/benchmarks/{name}/test-cases", addTestCasesHandler(deps.Benchmarks))
	mux.HandleFunc("PUT /api/benchmarks/{name}/test-cases/{index}", updateTestCaseHandler(deps.Benchmarks))
	mux.HandleFunc("DELETE /api/benchmarks/{name}/test-cases/{index}", deleteTestCaseHandler(deps.Benchmarks))
	mux.HandleFunc("POST /api/benchmarks/{name}/test-cases/generate", generateTestCasesHandler(deps.Benchmarks, deps.TestCaseGen))
	mux.HandleFunc("POST /api/benchmarks/{name}/runs", startBenchmarkRunHandler(deps.BenchRuns))
	mux.HandleFunc("GET /api/benchmarks/runs", listBenchmarkRunsHandler(deps.BenchRuns))
	mux.HandleFunc("GET /api/benchmarks/runs/{id}", getBenchmarkRunHandler(deps.BenchRuns))
	// A sibling of /api/benchmarks/{name} rather than nested under it
	// (/api/benchmarks/{name}/results) because that shape is genuinely
	// ambiguous with /api/benchmarks/runs/{id} to Go's ServeMux — both are
	// 2-segment GET patterns with the wildcard in a different position, and
	// e.g. "/api/benchmarks/runs/results" would match either.
	mux.HandleFunc("GET /api/benchmark-results/{name}", listBenchmarkResultsHandler(deps.BenchRuns))

	return mux, nil
}

// spaHandler serves static files from fsys, falling back to index.html for
// any path that doesn't match a file so client-side routing works.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(fsys, strippedPath(r.URL.Path)); err != nil {
			r = withPath(r, "/")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// strippedPath converts a URL path to the form fs.Stat expects (no leading
// slash, "index.html" for the root).
func strippedPath(p string) string {
	p = p[1:] // drop leading "/"
	if p == "" {
		p = "index.html"
	}
	return p
}

// withPath returns a shallow copy of r with its URL path replaced.
func withPath(r *http.Request, path string) *http.Request {
	clone := r.Clone(r.Context())
	clone.URL.Path = path
	return clone
}
