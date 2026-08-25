package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
)

func TestServeIndexAtRoot(t *testing.T) {
	handler, err := New(testDeps())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("GET / body does not look like the SPA shell: %s", rec.Body.String())
	}
}

func TestServeIndexForUnknownRouteSPAFallback(t *testing.T) {
	handler, err := New(testDeps())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/models/42", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /models/42 status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("GET /models/42 body does not look like the SPA shell: %s", rec.Body.String())
	}
}

func TestServeStaticAsset(t *testing.T) {
	handler, err := New(testDeps())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.svg status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSSEStreamsPublishedEvents(t *testing.T) {
	bus := eventbus.New()
	deps := testDeps()
	deps.Bus = bus
	handler, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	// Give the handler a moment to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)
	bus.Publish(eventbus.Event{Type: "heartbeat", Data: "hello"})

	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	var lines []string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if line != "" {
			lines = append(lines, line)
		}
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			break
		}
	}

	got := strings.Join(lines, "")
	if !strings.Contains(got, "event: heartbeat") || !strings.Contains(got, "data: hello") {
		t.Errorf("SSE stream = %q, want it to contain the published heartbeat event", got)
	}
}
