package profile

import "context"

type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Description  string   `yaml:"description"`
	Capabilities []string `yaml:"capabilities,omitempty"` // discoverable capability tags
	Accepts      string   `yaml:"accepts,omitempty"`      // what input this agent expects
	Returns      string   `yaml:"returns,omitempty"`      // what output this agent produces
}

type Spec struct {
	Instructions Instructions  `yaml:"instructions"`
	Provider     ProviderSpec  `yaml:"provider"`
	Tools        ToolSpec      `yaml:"tools"`
	Approval     ApprovalSpec  `yaml:"approval"`
	Workspace    WorkspaceSpec `yaml:"workspace"`
	Session      SessionSpec   `yaml:"session"`
	Policy       PolicySpec    `yaml:"policy"`
}

type Instructions struct {
	System []string `yaml:"system"`
}

type ProviderSpec struct {
	Default string `yaml:"default"`
	Model   string `yaml:"model"`
}

type ToolSpec struct {
	Enabled []string `yaml:"enabled"`
}

type ApprovalSpec struct {
	Mode       string   `yaml:"mode"`
	RequireFor []string `yaml:"requireFor"`
}

type WorkspaceSpec struct {
	Required   bool   `yaml:"required"`
	WriteScope string `yaml:"writeScope"`
}

type SessionSpec struct {
	Persistence string `yaml:"persistence"`
	Compaction  string `yaml:"compaction"`
}

type PolicySpec struct {
	Overlays []string `yaml:"overlays"`
}

type Reference struct {
	Name    string
	Path    string
	Bundled bool
}

type Source interface {
	Discover(ctx context.Context) ([]Reference, error)
	Load(ctx context.Context, ref string) (Manifest, string, error)
}
