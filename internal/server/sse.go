package server

import (
	"fmt"
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/eventbus"
)

// sseHandler streams events published on bus to the client as
// Server-Sent Events until the client disconnects.
func sseHandler(bus *eventbus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events, unsubscribe := bus.Subscribe()
		defer unsubscribe()

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
				flusher.Flush()
			}
		}
	}
}
