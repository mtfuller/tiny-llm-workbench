package registry

import "testing"

func testGraph() Graph {
	return Graph{
		Nodes: []Node{
			{ID: "1", Type: "input", Position: Position{X: 0, Y: 0}},
			{ID: "2", Type: "prompt", Position: Position{X: 100, Y: 0}, Data: NodeData{Model: "qwen2.5:0.5b", SystemPrompt: "Be nice."}},
			{ID: "3", Type: "output", Position: Position{X: 200, Y: 0}},
		},
		Edges: []Edge{
			{ID: "e1", Source: "1", Target: "2"},
			{ID: "e2", Source: "2", Target: "3"},
		},
	}
}

func TestSaveAndGetAgent(t *testing.T) {
	reg := New(t.TempDir())

	want := Agent{Name: "greeter", Graph: testGraph()}
	if err := reg.SaveAgent(want); err != nil {
		t.Fatalf("SaveAgent() error = %v", err)
	}

	got, err := reg.GetAgent("greeter")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.Name != want.Name || len(got.Graph.Nodes) != 3 || len(got.Graph.Edges) != 2 {
		t.Errorf("GetAgent() = %+v, want %+v", got, want)
	}
	if got.Graph.Nodes[1].Data.Model != "qwen2.5:0.5b" {
		t.Errorf("GetAgent().Graph.Nodes[1].Data.Model = %q, want %q", got.Graph.Nodes[1].Data.Model, "qwen2.5:0.5b")
	}
}

func TestSaveAgentOverwrites(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveAgent(Agent{Name: "greeter", Graph: testGraph()}); err != nil {
		t.Fatalf("SaveAgent() error = %v", err)
	}

	updated := Agent{Name: "greeter", Graph: Graph{Nodes: []Node{{ID: "1", Type: "input"}}}}
	if err := reg.SaveAgent(updated); err != nil {
		t.Fatalf("SaveAgent() (update) error = %v", err)
	}

	got, err := reg.GetAgent("greeter")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if len(got.Graph.Nodes) != 1 {
		t.Errorf("GetAgent().Graph.Nodes = %+v, want the overwritten single-node graph", got.Graph.Nodes)
	}
}

func TestGetAgentUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetAgent("does-not-exist"); err == nil {
		t.Error("GetAgent() error = nil, want an error for an unknown agent")
	}
}

func TestListAgentsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	agents, err := reg.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("ListAgents() = %v, want empty", agents)
	}
}

func TestListAgentsSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveAgent(Agent{Name: name, Graph: testGraph()}); err != nil {
			t.Fatalf("SaveAgent(%q) error = %v", name, err)
		}
	}

	agents, err := reg.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(agents) != 2 || agents[0].Name != "alpha" || agents[1].Name != "zeta" {
		t.Errorf("ListAgents() = %+v, want [alpha, zeta]", agents)
	}
}

func TestDeleteAgent(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveAgent(Agent{Name: "throwaway"}); err != nil {
		t.Fatalf("SaveAgent() error = %v", err)
	}

	if err := reg.DeleteAgent("throwaway"); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	if _, err := reg.GetAgent("throwaway"); err == nil {
		t.Error("GetAgent() error = nil, want an error after delete")
	}
}

func TestDeleteAgentNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteAgent("does-not-exist"); err == nil {
		t.Error("DeleteAgent() error = nil, want an error for an unknown agent")
	}
}
