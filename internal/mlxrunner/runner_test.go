package mlxrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFreePortReturnsBindablePort(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort() error = %v", err)
	}
	if port <= 0 {
		t.Errorf("freePort() = %d, want a positive port", port)
	}
}

func TestCompleteReturnsFirstChoiceMessageContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello there"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	text, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if text != "hello there" {
		t.Errorf("complete() = %q, want %q", text, "hello there")
	}
}

func TestCompleteSendsPromptAsUserMessage(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	if _, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" || gotBody.Messages[0].Content != "hi" {
		t.Errorf("request messages = %+v, want a single user message with the prompt", gotBody.Messages)
	}
}

func TestCompleteSendsFullMessageHistoryVerbatim(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	history := []chatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello!"},
		{Role: "user", Content: "how are you?"},
	}

	r := &Runner{httpClient: server.Client()}
	if _, err := r.complete(context.Background(), server.URL, history); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if len(gotBody.Messages) != len(history) {
		t.Fatalf("request messages = %+v, want all %d turns of history forwarded", gotBody.Messages, len(history))
	}
	for i, m := range history {
		if gotBody.Messages[i] != m {
			t.Errorf("request messages[%d] = %+v, want %+v", i, gotBody.Messages[i], m)
		}
	}
}

func TestCompleteSendsMaxTokensCap(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client(), MaxTokens: 64}
	if _, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if gotBody.MaxTokens != 64 {
		t.Errorf("request max_tokens = %d, want the configured MaxTokens (64)", gotBody.MaxTokens)
	}
}

func TestCompleteDefaultsMaxTokensWhenUnset(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	if _, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if gotBody.MaxTokens != defaultMaxTokens {
		t.Errorf("request max_tokens = %d, want defaultMaxTokens (%d) when unset", gotBody.MaxTokens, defaultMaxTokens)
	}
}

func TestCompleteSendsRepetitionPenalty(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client(), RepetitionPenalty: 1.15}
	if _, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if gotBody.RepetitionPenalty != 1.15 {
		t.Errorf("request repetition_penalty = %v, want the configured value (1.15)", gotBody.RepetitionPenalty)
	}
}

func TestCompleteDefaultsRepetitionPenaltyWhenUnset(t *testing.T) {
	var gotBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	if _, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if gotBody.RepetitionPenalty != defaultRepetitionPenalty {
		t.Errorf("request repetition_penalty = %v, want defaultRepetitionPenalty (%v) when unset", gotBody.RepetitionPenalty, defaultRepetitionPenalty)
	}
}

func TestCompleteSurfacesNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not loaded"))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	_, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("complete() error = %v, want it to include the response body", err)
	}
}

func TestCompleteErrorsOnNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	r := &Runner{httpClient: server.Client()}
	_, err := r.complete(context.Background(), server.URL, []chatMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Error("complete() error = nil, want an error when the response has no choices")
	}
}

func TestWaitReadySucceedsOnceServerResponds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	srv := &serverProc{baseURL: server.URL, ready: make(chan struct{}), done: make(chan struct{})}
	var stderr bytes.Buffer

	done := make(chan struct{})
	go func() {
		waitReady(srv, "test-model", &stderr)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waitReady() did not return after the server started responding")
	}
	if srv.readyErr != nil {
		t.Errorf("srv.readyErr = %v, want nil once the server responds 200", srv.readyErr)
	}
}

func TestWaitReadyFailsIfProcessExitsFirst(t *testing.T) {
	srv := &serverProc{baseURL: "http://127.0.0.1:1", ready: make(chan struct{}), done: make(chan struct{})}
	var stderr bytes.Buffer
	stderr.WriteString("ModuleNotFoundError: No module named 'mlx_lm'")
	close(srv.done) // simulate the process having already exited

	waitReady(srv, "test-model", &stderr)

	if srv.readyErr == nil || !strings.Contains(srv.readyErr.Error(), "No module named 'mlx_lm'") {
		t.Errorf("srv.readyErr = %v, want it to include the process's stderr", srv.readyErr)
	}
}

func TestGenerateCommandNotFound(t *testing.T) {
	r := New(context.Background())
	r.Command = "tlw-definitely-not-a-real-command"

	_, err := r.Generate(context.Background(), "some-model", "hi")
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("Generate() error = %v, want a clear \"not found on PATH\" message", err)
	}
}
