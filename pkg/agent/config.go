package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)


// FileConfig is the YAML structure of the agent config file.
type FileConfig struct {
	// Provider: "openai" | "anthropic" (or any openai-compatible via BaseURL)
	Provider string `yaml:"provider"`

	// Model ID to use (e.g. "gpt-4o", "claude-opus-4-5")
	Model string `yaml:"model"`

	// BaseURL overrides the default provider endpoint (e.g. for OpenRouter,
	// Azure, local Ollama, etc.). Only used by openai-compatible providers.
	BaseURL string `yaml:"base_url"`

	// APIKey can be a literal key or "${ENV_VAR}" to read from environment.
	APIKey string `yaml:"api_key"`

	// SystemPrompt is the system/instructions message sent with every call.
	SystemPrompt string `yaml:"system_prompt"`

	// MaxTokens caps the response length (0 = provider default).
	MaxTokens int `yaml:"max_tokens"`

	// Temperature controls randomness (nil = provider default).
	Temperature *float64 `yaml:"temperature"`

	// Tools configures built-in and plugin tools.
	Tools ToolsConfig `yaml:"tools"`
}

// ToolsConfig controls which built-in tools are registered and which plugin
// executables are loaded.
type ToolsConfig struct {
	// Preset selects the built-in tool set.
	// Values: "coding" (default) | "readonly" | "all" | "none"
	Preset string `yaml:"preset"`

	// WorkDir is the working directory for file tools.
	// Defaults to the process working directory.
	WorkDir string `yaml:"work_dir"`

	// Plugins lists external tool executables to load at startup.
	Plugins []PluginConfig `yaml:"plugins"`
}

// PluginConfig describes a single external tool plugin.
type PluginConfig struct {
	// Path is the path to the executable.
	Path string `yaml:"path"`
	// Args are extra CLI arguments passed to the plugin process.
	Args []string `yaml:"args"`
}

// ToolPreset returns the resolved builtin.Preset value from the config,
// defaulting to "coding" if unset.
func (c *FileConfig) ToolPreset() string {
	p := strings.ToLower(strings.TrimSpace(c.Tools.Preset))
	if p == "" {
		return "coding"
	}
	return p
}

// LoadFileConfig reads and parses a YAML config file, expanding ${ENV_VAR}
// references in string values.
func LoadFileConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	// Expand environment variables in the raw YAML before parsing.
	expanded := os.ExpandEnv(string(data))

	var cfg FileConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := validateFileConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateFileConfig(cfg *FileConfig) error {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Provider == "" {
		return fmt.Errorf("config: provider is required")
	}
	if cfg.Model == "" {
		return fmt.Errorf("config: model is required")
	}
	return nil
}
