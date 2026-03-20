package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bitop-dev/agent/internal/service"
	pkghost "github.com/bitop-dev/agent/pkg/host"
)

// ── HTTP request/response types ───────────────────────────────────────────────

type taskRequest struct {
	Profile  string         `json:"profile"`
	Task     string         `json:"task"`
	Context  map[string]any `json:"context,omitempty"`
	MaxTurns int            `json:"maxTurns,omitempty"`
}

type taskResponse struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Output    string  `json:"output,omitempty"`
	Error     string  `json:"error,omitempty"`
	SessionID string  `json:"sessionId,omitempty"`
	Duration  float64 `json:"duration"`
}

type agentInfoResponse struct {
	Agents []pkghost.AgentInfo `json:"agents"`
}

type healthResponse struct {
	OK       bool   `json:"ok"`
	Profiles int    `json:"profiles"`
	Plugins  int    `json:"plugins"`
	Tools    int    `json:"tools"`
	Mode     string `json:"mode"`
	Uptime   int64  `json:"uptime"`
}

// ── HTTP server ───────────────────────────────────────────────────────────────

// serveHTTP starts the HTTP worker server. If fixedProfile is set, only that
// profile is accepted. If empty, any profile can be requested per-task.
func serveHTTP(ctx context.Context, app service.App, addr, fixedProfile string) error {
	startedAt := time.Now()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		profiles, _ := app.Profiles.Discover(ctx)
		mode := "dynamic"
		if fixedProfile != "" {
			mode = "fixed:" + fixedProfile
		}
		writeHTTPJSON(w, http.StatusOK, healthResponse{
			OK:       true,
			Profiles: len(profiles),
			Plugins:  len(app.PluginManifests.List()),
			Tools:    len(app.Tools.List()),
			Mode:     mode,
			Uptime:   int64(time.Since(startedAt).Seconds()),
		})
	})

	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		profiles, err := app.Profiles.Discover(ctx)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var agents []pkghost.AgentInfo
		for _, p := range profiles {
			agents = append(agents, pkghost.AgentInfo{
				Name:         p.Manifest.Metadata.Name,
				Version:      p.Manifest.Metadata.Version,
				Description:  p.Manifest.Metadata.Description,
				Capabilities: p.Manifest.Metadata.Capabilities,
				Accepts:      p.Manifest.Metadata.Accepts,
				Returns:      p.Manifest.Metadata.Returns,
				Tools:        p.Manifest.Spec.Tools.Enabled,
			})
		}
		writeHTTPJSON(w, http.StatusOK, agentInfoResponse{Agents: agents})
	})

	mux.HandleFunc("/v1/task", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req taskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Task) == "" {
			writeHTTPError(w, http.StatusBadRequest, "task is required")
			return
		}
		// Determine which profile to use.
		profileRef := req.Profile
		if fixedProfile != "" {
			if profileRef != "" && profileRef != fixedProfile {
				writeHTTPError(w, http.StatusBadRequest,
					fmt.Sprintf("this worker only serves profile %q", fixedProfile))
				return
			}
			profileRef = fixedProfile
		}
		if profileRef == "" {
			writeHTTPError(w, http.StatusBadRequest, "profile is required (this is a dynamic worker)")
			return
		}

		start := time.Now()
		arguments := map[string]any{"task": req.Task}
		if len(req.Context) > 0 {
			arguments["context"] = req.Context
		}

		output, err := runTaskForServe(r.Context(), app, profileRef, arguments)
		duration := time.Since(start).Seconds()

		if err != nil {
			writeHTTPJSON(w, http.StatusOK, taskResponse{
				Status:   "failed",
				Error:    err.Error(),
				Duration: duration,
			})
			return
		}
		writeHTTPJSON(w, http.StatusOK, taskResponse{
			Status:   "completed",
			Output:   output,
			Duration: duration,
		})
	})

	mode := "dynamic"
	if fixedProfile != "" {
		mode = "fixed:" + fixedProfile
	}
	log.Printf("agent worker started on %s (mode=%s)", addr, mode)
	log.Printf("  POST /v1/task     — submit a task")
	log.Printf("  GET  /v1/agents   — list available agents")
	log.Printf("  GET  /v1/health   — health check")

	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	return server.ListenAndServe()
}

func writeHTTPJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeHTTPError(w http.ResponseWriter, status int, message string) {
	writeHTTPJSON(w, status, map[string]string{"error": message})
}
