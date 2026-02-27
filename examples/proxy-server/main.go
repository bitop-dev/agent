// examples/proxy-server — serves any upstream provider as an HTTP proxy.
//
// Demonstrates:
//   - proxy.NewHandler wrapping an upstream provider
//   - Bearer token authentication
//   - Health check endpoint
//   - Multiple upstreams on different routes
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... PROXY_TOKEN=secret go run ./examples/proxy-server
//
// Then configure a client:
//
//	provider: proxy
//	base_url: http://localhost:8080/anthropic
//	api_key: secret
//	model: claude-sonnet-4-5
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	anthropicProvider "github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
	openaiProvider "github.com/bitop-dev/agent/pkg/ai/providers/openai"
	"github.com/bitop-dev/agent/pkg/ai/providers/proxy"
)

func main() {
	token := os.Getenv("PROXY_TOKEN")
	addr  := envOr("PROXY_ADDR", ":8080")

	mux := http.NewServeMux()

	// ── Health check ────────────────────────────────────────────────────
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// ── Anthropic upstream ──────────────────────────────────────────────
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		upstream := anthropicProvider.New(key)
		handler  := proxy.NewHandler(upstream, token)
		// Register both /stream (direct) and /anthropic/stream (namespaced)
		mux.Handle("/stream", handler)
		mux.Handle("/anthropic/stream", handler)
		log.Println("anthropic upstream: enabled")
	} else {
		log.Println("anthropic upstream: disabled (ANTHROPIC_API_KEY not set)")
	}

	// ── OpenAI upstream ──────────────────────────────────────────────────
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		upstream := openaiProvider.New(key)
		handler  := proxy.NewHandler(upstream, token)
		mux.Handle("/openai/stream", handler)
		log.Println("openai upstream: enabled")
	} else {
		log.Println("openai upstream: disabled (OPENAI_API_KEY not set)")
	}

	// ── Request logging middleware ────────────────────────────────────────
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mux.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})

	// ── Startup ───────────────────────────────────────────────────────────
	if token != "" {
		log.Printf("auth: Bearer token required")
	} else {
		log.Printf("auth: disabled (set PROXY_TOKEN to enable)")
	}
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, logged); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
