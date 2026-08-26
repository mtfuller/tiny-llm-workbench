package registry

import (
	"reflect"
	"testing"
)

func TestCreateAndListDatasets(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
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

func TestCreateDatasetWithTitleAndDescriptionRoundTrips(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "Greetings", "Casual hello/goodbye pairs"); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	got, err := reg.GetDataset("greetings")
	if err != nil {
		t.Fatalf("GetDataset() error = %v", err)
	}
	if got.Title != "Greetings" || got.Description != "Casual hello/goodbye pairs" {
		t.Errorf("GetDataset() = %+v, want Title=Greetings Description=%q", got, "Casual hello/goodbye pairs")
	}

	datasets, err := reg.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(datasets) != 1 || datasets[0].Title != "Greetings" {
		t.Errorf("ListDatasets() = %+v, want a single entry with Title=Greetings", datasets)
	}
}

func TestAppendAndListExamples(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
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
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("ListExamples()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAppendAndListExamplesWithDescriptionAndTags(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	want := Example{Input: "hi", Output: "hello!", Description: "a friendly greeting", Tags: []string{"casual", "greeting"}}
	if err := reg.AppendExamples("greetings", []Example{want}); err != nil {
		t.Fatalf("AppendExamples() error = %v", err)
	}

	got, err := reg.ListExamples("greetings")
	if err != nil {
		t.Fatalf("ListExamples() error = %v", err)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("ListExamples() = %+v, want %+v", got, []Example{want})
	}
}

func TestUpdateExample(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}
	if err := reg.AppendExamples("greetings", []Example{
		{Input: "hi", Output: "hello!"},
		{Input: "hey", Output: "hey there!"},
	}); err != nil {
		t.Fatalf("AppendExamples() error = %v", err)
	}

	if err := reg.UpdateExample("greetings", 1, Example{Input: "hey", Output: "yo!"}); err != nil {
		t.Fatalf("UpdateExample() error = %v", err)
	}

	got, err := reg.ListExamples("greetings")
	if err != nil {
		t.Fatalf("ListExamples() error = %v", err)
	}
	if len(got) != 2 || got[0].Output != "hello!" || got[1].Output != "yo!" {
		t.Errorf("ListExamples() = %+v, want index 0 unchanged and index 1 updated", got)
	}
}

func TestUpdateExampleOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	if err := reg.UpdateExample("greetings", 0, Example{Input: "hi", Output: "hello!"}); err == nil {
		t.Error("UpdateExample() error = nil, want an error for an out-of-range index")
	}
}

func TestDeleteExample(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}
	if err := reg.AppendExamples("greetings", []Example{
		{Input: "hi", Output: "hello!"},
		{Input: "hey", Output: "hey there!"},
	}); err != nil {
		t.Fatalf("AppendExamples() error = %v", err)
	}

	if err := reg.DeleteExample("greetings", 0); err != nil {
		t.Fatalf("DeleteExample() error = %v", err)
	}

	got, err := reg.ListExamples("greetings")
	if err != nil {
		t.Fatalf("ListExamples() error = %v", err)
	}
	if len(got) != 1 || got[0].Input != "hey" {
		t.Errorf("ListExamples() = %+v, want only the second example to remain", got)
	}
}

func TestDeleteExampleOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	if err := reg.DeleteExample("greetings", 0); err == nil {
		t.Error("DeleteExample() error = nil, want an error for an out-of-range index")
	}
}

func TestListDatasetsReflectsPairCount(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("greetings", "", ""); err != nil {
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

func TestDeleteDataset(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.CreateDataset("throwaway", "", ""); err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}

	if err := reg.DeleteDataset("throwaway"); err != nil {
		t.Fatalf("DeleteDataset() error = %v", err)
	}

	datasets, err := reg.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(datasets) != 0 {
		t.Errorf("ListDatasets() = %+v, want empty after delete", datasets)
	}
}

func TestDeleteDatasetNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteDataset("does-not-exist"); err == nil {
		t.Error("DeleteDataset() error = nil, want an error for an unknown dataset")
	}
}
