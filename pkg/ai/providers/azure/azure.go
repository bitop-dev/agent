// Package azure implements ai.Provider for Azure OpenAI.
//
// Azure OpenAI uses the same wire format as the OpenAI Chat Completions API but:
//   - URL:  https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version={version}
//   - Auth: "api-key: {key}" header instead of "Authorization: Bearer {key}"
//
// Configure in agent.yaml:
//
//	provider: azure
//	model:    gpt-4o              # deployment name
//	base_url: https://myresource.openai.azure.com/openai/deployments/gpt-4o
//	api_key:  ${AZURE_OPENAI_API_KEY}
//	api_version: 2024-12-01-preview   # optional, defaults to stable version
package azure

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/ai/providers/openai"
)

const defaultAPIVersion = "2024-12-01-preview"

// Provider wraps the OpenAI completions provider with Azure-specific
// URL construction and the api-key authentication header.
type Provider struct {
	// DeploymentURL is the full Azure deployment endpoint, e.g.
	// https://myresource.openai.azure.com/openai/deployments/gpt-4o
	DeploymentURL string
	APIVersion    string

	inner *openai.Provider
}

// New creates an Azure OpenAI provider.
//
//	deploymentURL — full endpoint up to the deployment name (no trailing slash)
//	apiVersion    — e.g. "2024-12-01-preview"; pass "" for the default
func New(deploymentURL, apiVersion string) *Provider {
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	// Strip trailing slash
	deploymentURL = strings.TrimRight(deploymentURL, "/")

	// Build the full chat/completions URL
	completionsURL := deploymentURL
	if !strings.HasSuffix(completionsURL, "/chat/completions") {
		completionsURL += "/chat/completions"
	}
	// Inject api-version query param into the base URL so our generic
	// HTTP client always appends it.
	baseURL := completionsURL + "?api-version=" + apiVersion

	inner := openai.New("")
	inner.BaseURL = baseURL // override — we build the full URL manually
	inner.HTTPClient = &http.Client{Timeout: 10 * time.Minute}

	return &Provider{
		DeploymentURL: deploymentURL,
		APIVersion:    apiVersion,
		inner:         inner,
	}
}

func (p *Provider) Name() string { return "azure" }

func (p *Provider) Stream(
	ctx context.Context,
	model string, // model is unused — the deployment is encoded in the URL
	llmCtx ai.Context,
	opts ai.StreamOptions,
) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	// Swap auth header: Azure uses "api-key" not "Authorization: Bearer"
	// We do this by injecting a request interceptor via a custom transport.
	origTransport := p.inner.HTTPClient.Transport
	if origTransport == nil {
		origTransport = http.DefaultTransport
	}
	p.inner.HTTPClient.Transport = &azureTransport{
		apiKey: opts.APIKey,
		inner:  origTransport,
	}
	defer func() {
		p.inner.HTTPClient.Transport = origTransport
	}()

	// Pass empty API key so the inner provider does not set "Authorization: Bearer"
	azureOpts := opts
	azureOpts.APIKey = "" // transport handles auth

	return p.inner.Stream(ctx, model, llmCtx, azureOpts)
}

// azureTransport replaces the Authorization header with the Azure api-key header.
type azureTransport struct {
	apiKey string
	inner  http.RoundTripper
}

func (t *azureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request so we don't modify the original
	req2 := req.Clone(req.Context())
	req2.Header.Del("Authorization")
	if t.apiKey != "" {
		req2.Header.Set("api-key", t.apiKey)
	}
	return t.inner.RoundTrip(req2)
}

// BuildDeploymentURL is a helper that constructs the Azure deployment URL from
// its components — useful when callers have them separately.
//
//	resource   — e.g. "myresource"
//	deployment — e.g. "gpt-4o"
func BuildDeploymentURL(resource, deployment string) string {
	return fmt.Sprintf("https://%s.openai.azure.com/openai/deployments/%s", resource, deployment)
}
