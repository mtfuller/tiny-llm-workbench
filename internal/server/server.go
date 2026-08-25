// Package server implements the local webserver that serves the embedded
// browser UI, the Models/Dataset/Training JSON API, and a live event stream
// over Server-Sent Events.
package server

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/agents"
	"github.com/mtfuller/tiny-llm-workbench/internal/environments"
	"github.com/mtfuller/tiny-llm-workbench/internal/evaluations"
	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
	"github.com/mtfuller/tiny-llm-workbench/internal/models"
	"github.com/mtfuller/tiny-llm-workbench/internal/registry"
	"github.com/mtfuller/tiny-llm-workbench/internal/training"
	"github.com/mtfuller/tiny-llm-workbench/web"
)

// modelCatalog is the subset of models.Catalog the server needs.
type modelCatalog interface {
	List(ctx context.Context) ([]models.Model, error)
}

// datasetStore is the subset of registry.Registry the server needs for
// dataset endpoints.
type datasetStore interface {
	ListDatasets() ([]registry.DatasetSummary, error)
	CreateDataset(name string) (registry.Dataset, error)
	ListExamples(name string) ([]registry.Example, error)
	AppendExamples(name string, examples []registry.Example) error
}

// variationGenerator is the subset of datasetgen.Generator the server needs.
type variationGenerator interface {
	Variations(ctx context.Context, model string, seed registry.Example, n int) ([]registry.Example, error)
}

// trainingManager is the subset of training.Manager the server needs.
type trainingManager interface {
	StartRun(cfg training.Config) (*training.Run, error)
	ListRuns() []*training.Run
	GetRun(id string) (*training.Run, bool)
}

// environmentStore is the subset of registry.Registry the server needs for
// Environment definitions.
type environmentStore interface {
	ListEnvironments() ([]registry.Environment, error)
	SaveEnvironment(e registry.Environment) error
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
}

// evaluationManager is the subset of evaluations.Manager the server needs.
type evaluationManager interface {
	StartRun(evaluationName string, agentNames []string) (*evaluations.Run, error)
	ListRuns() []*evaluations.Run
	GetRun(id string) (*evaluations.Run, bool)
}

// Deps are the server's dependencies, all provided by the caller so they can
// be swapped for fakes in tests.
type Deps struct {
	Bus          *eventbus.Bus
	Catalog      modelCatalog
	Datasets     datasetStore
	Generator    variationGenerator
	Training     trainingManager
	Environments environmentStore
	Instances    environmentManager
	Agents       agentStore
	AgentRuns    agentManager
	Evaluations  evaluationStore
	EvalRuns     evaluationManager
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
	mux.HandleFunc("GET /api/models", listModelsHandler(deps.Catalog))
	mux.HandleFunc("GET /api/datasets", listDatasetsHandler(deps.Datasets))
	mux.HandleFunc("POST /api/datasets", createDatasetHandler(deps.Datasets))
	mux.HandleFunc("GET /api/datasets/{name}", getDatasetHandler(deps.Datasets))
	mux.HandleFunc("POST /api/datasets/{name}/variations", generateVariationsHandler(deps.Datasets, deps.Generator))
	mux.HandleFunc("POST /api/training/runs", startTrainingRunHandler(deps.Training))
	mux.HandleFunc("GET /api/training/runs", listTrainingRunsHandler(deps.Training))
	mux.HandleFunc("GET /api/training/runs/{id}", getTrainingRunHandler(deps.Training))
	mux.HandleFunc("GET /api/environments", listEnvironmentsHandler(deps.Environments))
	mux.HandleFunc("POST /api/environments", createEnvironmentHandler(deps.Environments))
	mux.HandleFunc("POST /api/environments/{name}/launch", launchEnvironmentHandler(deps.Instances))
	mux.HandleFunc("GET /api/environments/instances", listInstancesHandler(deps.Instances))
	mux.HandleFunc("POST /api/environments/instances/{id}/stop", stopInstanceHandler(deps.Instances))
	mux.HandleFunc("POST /api/environments/instances/{id}/exec", startExecHandler(deps.Instances))
	mux.HandleFunc("GET /api/environments/instances/{id}/execs/{execId}", getExecHandler(deps.Instances))
	mux.HandleFunc("GET /api/agents", listAgentsHandler(deps.Agents))
	mux.HandleFunc("POST /api/agents", saveAgentHandler(deps.Agents))
	mux.HandleFunc("GET /api/agents/{name}", getAgentHandler(deps.Agents))
	mux.HandleFunc("POST /api/agents/{name}/runs", startAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("POST /api/agents/runs/{id}/messages", sendAgentMessageHandler(deps.AgentRuns))
	mux.HandleFunc("GET /api/agents/runs/{id}", getAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("POST /api/agents/runs/{id}/stop", stopAgentRunHandler(deps.AgentRuns))
	mux.HandleFunc("GET /api/evaluations", listEvaluationsHandler(deps.Evaluations))
	mux.HandleFunc("POST /api/evaluations", saveEvaluationHandler(deps.Evaluations))
	mux.HandleFunc("GET /api/evaluations/{name}", getEvaluationHandler(deps.Evaluations))
	mux.HandleFunc("POST /api/evaluations/{name}/runs", startEvaluationRunHandler(deps.EvalRuns))
	mux.HandleFunc("GET /api/evaluations/runs", listEvaluationRunsHandler(deps.EvalRuns))
	mux.HandleFunc("GET /api/evaluations/runs/{id}", getEvaluationRunHandler(deps.EvalRuns))

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
