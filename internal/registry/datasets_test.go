package registry

import (
	"testing"
)

func TestCreateAndListDatasets(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings"); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	datasets, err := reg.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(datasets) != 1 {
		t.Fatalf("ListDatasets() returned %d datasets, want 1", len(datasets))
	}
	if datasets[0].Name != "greetings" {
		t.Errorf("ListDatasets()[0].Name = %q, want %q", datasets[0].Name, "greetings")
	}
	if datasets[0].PairCount != 0 {
		t.Errorf("ListDatasets()[0].PairCount = %d, want 0 for a fresh dataset", datasets[0].PairCount)
	}
}

func TestAppendAndListExamples(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings"); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	want := []Example{
		{Input: "hi", Output: "hello!"},
		{Input: "hey", Output: "hey there!"},
	}
	if err := reg.AppendExamples("greetings", want); err != nil {
		t.Fatalf("AppendExamples() error = %v", err)
	}

	got, err := reg.ListExamples("greetings")
	if err != nil {
		t.Fatalf("ListExamples() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ListExamples() returned %d examples, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListExamples()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListDatasetsReflectsPairCount(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings"); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}
	if err := reg.AppendExamples("greetings", []Example{{Input: "hi", Output: "hello!"}}); err != nil {
		t.Fatalf("AppendExamples() error = %v", err)
	}

	datasets, err := reg.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(datasets) != 1 || datasets[0].PairCount != 1 {
		t.Fatalf("ListDatasets() = %+v, want a single dataset with PairCount 1", datasets)
	}
}

func TestListDatasetsEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	datasets, err := reg.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(datasets) != 0 {
		t.Errorf("ListDatasets() = %v, want empty", datasets)
	}
}

func TestListExamplesUnknownDataset(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.ListExamples("does-not-exist"); err == nil {
		t.Error("ListExamples() error = nil, want an error for an unknown dataset")
	}
}
