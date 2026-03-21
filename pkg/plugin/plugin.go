package plugin

type Category string

const (
	CategoryAsset         Category = "asset"
	CategoryIntegration   Category = "integration"
	CategoryBridge        Category = "bridge"
	CategoryOrchestration Category = "orchestration"
)

type RuntimeType string

const (
	RuntimeAsset   RuntimeType = "asset"
	RuntimeHTTP    RuntimeType = "http"
	RuntimeCommand RuntimeType = "command"
	RuntimeMCP     RuntimeType = "mcp"
	RuntimeRPC     RuntimeType = "rpc"
	RuntimeHost    RuntimeType = "host"
)

type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type Spec struct {
	Category     Category      `yaml:"category"`
	Runtime      Runtime       `yaml:"runtime"`
	Contributes  Contributions `yaml:"contributes"`
	ConfigSchema Schema        `yaml:"configSchema"`
	Permissions  Permissions   `yaml:"permissions"`
	Requires     Requirements  `yaml:"requires"`
}

type Runtime struct {
	Type       RuntimeType       `yaml:"type"`
	Command    []string          `yaml:"command,omitempty"`
	Endpoint   string            `yaml:"endpoint,omitempty"`
	Protocol   string            `yaml:"protocol,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	// EnvMapping maps plugin config keys to environment variable names for MCP
	// and command runtimes. For example:
	//   envMapping:
	//     grafanaURL: GRAFANA_URL
	//     grafanaAPIKey: GRAFANA_API_KEY
	// This allows plugin config values to be injected as env vars without
	// requiring users to set a nested "env" object in their config.
	EnvMapping map[string]string `yaml:"envMapping,omitempty"`
}

type Schema struct {
	Type       string              `yaml:"type,omitempty"`
	Properties map[string]Property `yaml:"properties,omitempty"`
	Required   []string            `yaml:"required,omitempty"`
}

type Property struct {
	Type    string   `yaml:"type,omitempty"`
	Enum    []string `yaml:"enum,omitempty"`
	Secret  bool     `yaml:"secret,omitempty"`
	Default string   `yaml:"default,omitempty"` // default value if not set by user
	EnvVar  string   `yaml:"envVar,omitempty"`  // env var to read from (e.g. SMTP_HOST)
}

type Contributions struct {
	Tools            []Contribution `yaml:"tools"`
	Providers        []Contribution `yaml:"providers,omitempty"`
	Prompts          []Contribution `yaml:"prompts"`
	ProfileTemplates []Contribution `yaml:"profileTemplates"`
	Policies         []Contribution `yaml:"policies"`
	ApprovalAdapters []Contribution `yaml:"approvalAdapters,omitempty"`
	EventObservers   []Contribution `yaml:"eventObservers,omitempty"`
	HostCapabilities []Contribution `yaml:"hostCapabilities,omitempty"`
}

type Contribution struct {
	ID         string `yaml:"id"`
	Path       string `yaml:"path,omitempty"`
	Entrypoint string `yaml:"entrypoint,omitempty"`
}

type Permissions struct {
	Network          NetworkPermissions `yaml:"network,omitempty"`
	SensitiveActions []string           `yaml:"sensitiveActions,omitempty"`
	HostCapabilities []string           `yaml:"hostCapabilities,omitempty"`
}

type NetworkPermissions struct {
	Outbound []string `yaml:"outbound,omitempty"`
}

type Requirements struct {
	Framework string   `yaml:"framework"`
	Plugins   []string `yaml:"plugins"`
}

type Reference struct {
	Name    string
	Path    string
	Enabled bool
	Bundled bool
}

type ToolDescriptor struct {
	ID          string         `yaml:"id"`
	Kind        string         `yaml:"kind,omitempty"`
	Description string         `yaml:"description"`
	InputSchema map[string]any `yaml:"inputSchema,omitempty"`
	Execution   ToolExecution  `yaml:"execution,omitempty"`
	Risk        ToolRisk       `yaml:"risk,omitempty"`
}

type ToolExecution struct {
	Mode      string   `yaml:"mode,omitempty"`
	Operation string   `yaml:"operation,omitempty"`
	Argv      []string `yaml:"argv,omitempty"`
	Timeout   int      `yaml:"timeout,omitempty"`
}

type ToolRisk struct {
	Level string `yaml:"level,omitempty"`
}
