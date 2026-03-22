package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/events"
)

// gatewayEventSink wraps an existing sink and forwards tool events to the gateway.
type gatewayEventSink struct {
	inner      events.Sink
	gatewayURL string
	taskID     string
	client     *http.Client
}

func newGatewayEventSink(inner events.Sink, taskID string) events.Sink {
	gwURL := os.Getenv("GATEWAY_URL")
	if gwURL == "" || taskID == "" {
		return inner // no gateway, just use inner sink
	}
	return &gatewayEventSink{
		inner:      inner,
		gatewayURL: gwURL,
		taskID:     taskID,
		client:     &http.Client{Timeout: 2 * time.Second},
	}
}

func (s *gatewayEventSink) Publish(ctx context.Context, event events.Event) error {
	// Always forward to inner sink first
	if err := s.inner.Publish(ctx, event); err != nil {
		return err
	}

	// Forward tool events and sub-agent events to gateway (best-effort)
	var toolName, msg string
	eventType := ""

	switch event.Type {
	case events.TypeToolRequested:
		eventType = "tool_requested"
		toolName = event.Message
		msg = fmt.Sprintf("Calling %s", event.Message)
	case events.TypeToolStarted:
		eventType = "tool_started"
		toolName = event.Message
		msg = fmt.Sprintf("Running %s", event.Message)
	case events.TypeToolFinished:
		eventType = "tool_finished"
		toolName = event.Message
		// Truncate output for display
		output := event.Message
		if len(output) > 100 {
			output = output[:100] + "..."
		}
		msg = output
	case events.TypeTurnStarted:
		eventType = "turn_started"
		msg = event.Message
	case events.TypeTurnFinished:
		eventType = "turn_finished"
		msg = event.Message
	default:
		// Check for sub-agent prefix in message
		if strings.HasPrefix(event.Message, "[sub:") {
			eventType = "sub_agent"
			msg = event.Message
		} else {
			return nil // don't forward other events
		}
	}

	// POST to gateway
	body, _ := json.Marshal(map[string]string{
		"type":    eventType,
		"tool":    toolName,
		"message": msg,
		"time":    event.Time.Format(time.RFC3339),
	})

	url := strings.TrimRight(s.gatewayURL, "/") + "/v1/tasks/" + s.taskID + "/events"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil // best-effort
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	return nil
}
