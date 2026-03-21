package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bitop-dev/agent/internal/service"
)

type trigger struct {
	profile      string
	event        string
	taskTemplate string
}

// startReactiveTriggers discovers service-mode profiles and subscribes to
// their NATS triggers. When an event matches, it creates a task via the gateway.
func startReactiveTriggers(ctx context.Context, app service.App) {
	natsURL := os.Getenv("NATS_URL")
	gatewayURL := os.Getenv("GATEWAY_URL")
	if natsURL == "" || gatewayURL == "" {
		return // reactive triggers require both NATS and gateway
	}

	profiles, err := app.Profiles.Discover(ctx)
	if err != nil {
		return
	}

	var triggers []trigger
	for _, p := range profiles {
		if p.Manifest.Spec.Mode != "service" {
			continue
		}
		for _, t := range p.Manifest.Spec.Triggers {
			if t.Event == "" {
				continue
			}
			triggers = append(triggers, trigger{
				profile:      p.Manifest.Metadata.Name,
				event:        t.Event,
				taskTemplate: t.TaskTemplate,
			})
		}
	}

	if len(triggers) == 0 {
		return
	}

	// Connect to NATS.
	// Use a lightweight approach — just subscribe via the gateway's SSE endpoint
	// or connect directly. For simplicity, poll the gateway for events.
	// TODO: direct NATS connection for lower latency.
	log.Printf("reactive: %d triggers from %d service-mode profiles", len(triggers), countUniqueServiceProfiles(triggers))

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("reactive: stopped")
				return
			case <-ticker.C:
				// For now, reactive triggers are activated by webhooks and schedules
				// through the gateway. Direct NATS subscription will be added when
				// the nats.go client is added to the agent module.
				// This goroutine keeps the triggers registered for discovery.
			}
		}
	}()

	// Register the triggers with the gateway so it knows what events
	// should create tasks for which profiles.
	for _, t := range triggers {
		registerTriggerAsWebhook(gatewayURL, t.profile, t.event, t.taskTemplate)
	}
}

// registerTriggerAsWebhook creates a webhook on the gateway that maps
// a NATS event topic to a task. This bridges reactive triggers into
// the existing webhook infrastructure.
func registerTriggerAsWebhook(gatewayURL, profile, event, taskTemplate string) {
	// Convert NATS topic to webhook path: agent.alert.fired → alert-fired
	path := strings.ReplaceAll(strings.TrimPrefix(event, "agent."), ".", "-")
	path = "reactive-" + path

	if taskTemplate == "" {
		taskTemplate = fmt.Sprintf("Reactive trigger: %s event fired. Investigate.", event)
	}

	body := map[string]any{
		"name":         "reactive-" + profile + "-" + path,
		"path":         path,
		"profile":      profile,
		"taskTemplate": taskTemplate,
	}
	data, _ := json.Marshal(body)

	adminKey := os.Getenv("GATEWAY_ADMIN_KEY")
	url := strings.TrimRight(gatewayURL, "/") + "/v1/webhooks"
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	if adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+adminKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("reactive: register trigger %s failed: %v", event, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("reactive: registered trigger %s → profile=%s webhook=%s", event, profile, path)
	}
}

func countUniqueServiceProfiles(triggers []trigger) int {
	seen := make(map[string]bool)
	for _, t := range triggers {
		seen[t.profile] = true
	}
	return len(seen)
}
