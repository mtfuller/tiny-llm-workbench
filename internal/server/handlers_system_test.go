package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemInfo(t *testing.T) {
	deps := testDeps()
	deps.RegistryRoot = "/tmp/tlw-test"
	deps.OllamaBaseURL = "http://localhost:11434"

	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/system status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.RegistryRoot != "/tmp/tlw-test" || got.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("GET /api/system body = %+v, want the configured registry root/Ollama URL", got)
	}
	if got.Version == "" {
		t.Error("GET /api/system Version = \"\", want a non-empty version string")
	}
}
