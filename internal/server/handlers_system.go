package server

import (
	"net/http"

	"github.com/mtfuller/tiny-llm-workbench/internal/version"
)

// systemInfo is the GET /api/system response body — plain, read-only
// configuration values shown on the Settings page.
type systemInfo struct {
	Version      string `json:"version"`
	RegistryRoot string `json:"registryRoot"`
}

// systemInfoHandler responds with static server configuration. registryRoot
// is captured at server startup, not looked up per request, since it
// doesn't change while the process is running.
func systemInfoHandler(registryRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, systemInfo{
			Version:      version.GetShortVersion(),
			RegistryRoot: registryRoot,
		})
	}
}
