package registry

import "testing"

func TestSaveAndGetKnowledgeBase(t *testing.T) {
	reg := New(t.TempDir())

	want := KnowledgeBase{
		Name:        "faq",
		Description: "Frequently asked questions",
		Records:     []KnowledgeRecord{{Title: "Refunds", Content: "Refunds take 3-5 business days."}},
	}
	if err := reg.SaveKnowledgeBase(want); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if got.Name != want.Name || got.Description != want.Description || len(got.Records) != 1 {
		t.Errorf("GetKnowledgeBase() = %+v, want %+v", got, want)
	}
}

func TestSaveKnowledgeBaseSetsCreatedAtOnFirstSave(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if got.CreatedAt.IsZero() {
		t.Error("GetKnowledgeBase().CreatedAt is zero, want it set on first save")
	}
}

func TestSaveKnowledgeBasePreservesCreatedAtOnOverwrite(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}
	first, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq", Description: "updated"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() (update) error = %v", err)
	}
	second, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed on overwrite: first = %v, second = %v", first.CreatedAt, second.CreatedAt)
	}
	if second.Description != "updated" {
		t.Errorf("Description = %q, want %q", second.Description, "updated")
	}
}

func TestGetKnowledgeBaseUnknown(t *testing.T) {
	reg := New(t.TempDir())

	if _, err := reg.GetKnowledgeBase("does-not-exist"); err == nil {
		t.Error("GetKnowledgeBase() error = nil, want an error for an unknown knowledge base")
	}
}

func TestListKnowledgeBasesEmptyRegistry(t *testing.T) {
	reg := New(t.TempDir())

	bases, err := reg.ListKnowledgeBases()
	if err != nil {
		t.Fatalf("ListKnowledgeBases() error = %v", err)
	}
	if len(bases) != 0 {
		t.Errorf("ListKnowledgeBases() = %v, want empty", bases)
	}
}

func TestListKnowledgeBasesSortedByName(t *testing.T) {
	reg := New(t.TempDir())

	for _, name := range []string{"zeta", "alpha"} {
		if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: name}); err != nil {
			t.Fatalf("SaveKnowledgeBase(%q) error = %v", name, err)
		}
	}

	bases, err := reg.ListKnowledgeBases()
	if err != nil {
		t.Fatalf("ListKnowledgeBases() error = %v", err)
	}
	if len(bases) != 2 || bases[0].Name != "alpha" || bases[1].Name != "zeta" {
		t.Errorf("ListKnowledgeBases() = %+v, want [alpha, zeta]", bases)
	}
}

func TestDeleteKnowledgeBase(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "throwaway"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.DeleteKnowledgeBase("throwaway"); err != nil {
		t.Fatalf("DeleteKnowledgeBase() error = %v", err)
	}

	if _, err := reg.GetKnowledgeBase("throwaway"); err == nil {
		t.Error("GetKnowledgeBase() error = nil, want an error after delete")
	}
}

func TestDeleteKnowledgeBaseNotFound(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.DeleteKnowledgeBase("does-not-exist"); err == nil {
		t.Error("DeleteKnowledgeBase() error = nil, want an error for an unknown knowledge base")
	}
}

func TestAddRecords(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.AddRecords("faq", []KnowledgeRecord{
		{Title: "Refunds", Content: "Refunds take 3-5 business days."},
		{Title: "Shipping", Content: "We ship worldwide."},
	}); err != nil {
		t.Fatalf("AddRecords() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("GetKnowledgeBase().Records = %+v, want 2 records", got.Records)
	}
	if got.Records[0].ID == "" || got.Records[1].ID == "" || got.Records[0].ID == got.Records[1].ID {
		t.Errorf("GetKnowledgeBase().Records IDs = [%q, %q], want two distinct, non-empty server-assigned IDs", got.Records[0].ID, got.Records[1].ID)
	}
}

func TestAddRecordsIgnoresClientSuppliedID(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.AddRecords("faq", []KnowledgeRecord{{ID: "client-supplied", Title: "X", Content: "Y"}}); err != nil {
		t.Fatalf("AddRecords() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if len(got.Records) != 1 || got.Records[0].ID == "client-supplied" {
		t.Errorf("GetKnowledgeBase().Records = %+v, want a server-assigned ID overriding the client's", got.Records)
	}
}

func TestUpdateRecord(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq", Records: []KnowledgeRecord{{ID: "rec-1", Title: "Old", Content: "old content"}}}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.UpdateRecord("faq", 0, KnowledgeRecord{Title: "New", Content: "new content"}); err != nil {
		t.Fatalf("UpdateRecord() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if len(got.Records) != 1 || got.Records[0].Title != "New" || got.Records[0].ID != "rec-1" {
		t.Errorf("GetKnowledgeBase().Records = %+v, want title updated with the original ID preserved", got.Records)
	}
}

func TestUpdateRecordOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.UpdateRecord("faq", 0, KnowledgeRecord{Title: "X"}); err == nil {
		t.Error("UpdateRecord() error = nil, want an error for an out-of-range index")
	}
}

func TestDeleteRecord(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq", Records: []KnowledgeRecord{
		{ID: "rec-1", Title: "A"},
		{ID: "rec-2", Title: "B"},
	}}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.DeleteRecord("faq", 0); err != nil {
		t.Fatalf("DeleteRecord() error = %v", err)
	}

	got, err := reg.GetKnowledgeBase("faq")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if len(got.Records) != 1 || got.Records[0].ID != "rec-2" {
		t.Errorf("GetKnowledgeBase().Records = %+v, want only rec-2 to remain", got.Records)
	}
}

func TestDeleteRecordOutOfRange(t *testing.T) {
	reg := New(t.TempDir())

	if err := reg.SaveKnowledgeBase(KnowledgeBase{Name: "faq"}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	if err := reg.DeleteRecord("faq", 0); err == nil {
		t.Error("DeleteRecord() error = nil, want an error for an out-of-range index")
	}
}
