package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Paths struct {
	CWD              string
	HomeDir          string
	ConfigDir        string
	ConfigFile       string
	UserProfilesDir  string
	UserPluginsDir   string
	SessionsDir      string
	LocalProfilesDir string
	LocalPluginsDir  string
}

type Config struct {
	DefaultProfile string                    `yaml:"defaultProfile"`
	EnabledPlugins []string                  `yaml:"enabledPlugins"`
	ApprovalMode   string                    `yaml:"approvalMode"`
	Providers      map[string]ProviderConfig `yaml:"providers"`
	Plugins        map[string]PluginConfig   `yaml:"plugins"`
	PluginSources  []PluginSource            `yaml:"pluginSources,omitempty"`
}

type PluginSource struct {
	Name         string `yaml:"name"`
	Type         string `yaml:"type,omitempty"`
	Path         string `yaml:"path,omitempty"`
	URL          string `yaml:"url,omitempty"`
	Enabled      bool   `yaml:"enabled,omitempty"`
	PublishToken string `yaml:"publishToken,omitempty"`
}

type ProviderConfig struct {
	BaseURL string            `yaml:"baseURL"`
	APIKey  string            `yaml:"apiKey"`
	APIMode string            `yaml:"apiMode"`
	Model   string            `yaml:"model"`            // global default model
	Models  map[string]string `yaml:"models,omitempty"` // per-profile model overrides
}

type PluginConfig struct {
	Enabled          bool           `yaml:"enabled"`
	InstalledVersion string         `yaml:"installedVersion,omitempty"`
	InstalledSource  string         `yaml:"installedSource,omitempty"`
	Config           map[string]any `yaml:"config"`
}

func DefaultPaths(cwd string) (Paths, error) {
	if cwd == "" {
		return Paths{}, errors.New("cwd is required")
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return Paths{}, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	configDir := filepath.Join(homeDir, ".agent")
	return Paths{
		CWD:              absCWD,
		HomeDir:          homeDir,
		ConfigDir:        configDir,
		ConfigFile:       filepath.Join(configDir, "config.yaml"),
		UserProfilesDir:  filepath.Join(configDir, "profiles"),
		UserPluginsDir:   filepath.Join(configDir, "plugins"),
		SessionsDir:      filepath.Join(configDir, "sessions"),
		LocalProfilesDir: filepath.Join(absCWD, ".agent", "profiles"),
		LocalPluginsDir:  filepath.Join(absCWD, ".agent", "plugins"),
	}, nil
}

func Load(paths Paths) (Config, error) {
	if _, err := os.Stat(paths.ConfigFile); err != nil {
		if os.IsNotExist(err) {
			cfg := Config{}
			applyEnvOverrides(&cfg)
			return cfg, nil
		}
		return Config{}, err
	}
	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyEnvOverrides(&cfg)
	return cfg, nil
}

func Save(paths Paths, cfg Config) error {
	if err := os.MkdirAll(paths.ConfigDir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile, data, 0o644)
}

func applyEnvOverrides(cfg *Config) {
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	if cfg.Plugins == nil {
		cfg.Plugins = make(map[string]PluginConfig)
	}
	openAI := cfg.Providers["openai"]
	if value := os.Getenv("OPENAI_BASE_URL"); value != "" {
		openAI.BaseURL = value
	}
	if value := os.Getenv("OPENAI_API_KEY"); value != "" {
		openAI.APIKey = value
	}
	if value := os.Getenv("OPENAI_API_MODE"); value != "" {
		openAI.APIMode = value
	}
	if value := os.Getenv("OPENAI_MODEL"); value != "" {
		openAI.Model = value
	}
	if openAI.BaseURL != "" || openAI.APIKey != "" || openAI.APIMode != "" || openAI.Model != "" {
		cfg.Providers["openai"] = openAI
	}
}

// ResolveModel determines the model to use with the following priority:
//  1. cliModel — explicit --model flag (highest priority)
//  2. AGENT_MODEL env var
//  3. Per-profile override in config: providers.<name>.models.<profile>
//  4. Global default in config: providers.<name>.model
//  5. Profile's own model: spec.provider.model
//  6. Hardcoded fallback: gpt-4o
func ResolveModel(cfg Config, providerName, profileName, profileModel, cliModel string) string {
	// 1. CLI flag
	if cliModel != "" {
		return cliModel
	}

	// 2. AGENT_MODEL env var
	if envModel := os.Getenv("AGENT_MODEL"); envModel != "" {
		return envModel
	}

	provCfg := cfg.Providers[providerName]

	// 3. Per-profile override
	if provCfg.Models != nil {
		if m, ok := provCfg.Models[profileName]; ok && m != "" {
			return m
		}
	}

	// 4. Global default from config
	if provCfg.Model != "" {
		return provCfg.Model
	}

	// 5. Profile's recommendation
	if profileModel != "" {
		return profileModel
	}

	// 6. Hardcoded fallback
	return "gpt-4o"
}

func (c Config) IsPluginEnabled(name string) bool {
	if name == "core-tools" {
		return true
	}
	if pluginCfg, ok := c.Plugins[name]; ok {
		return pluginCfg.Enabled
	}
	for _, enabled := range c.EnabledPlugins {
		if enabled == name {
			return true
		}
	}
	return false
}

func (c *Config) SetPluginInstallRecord(name, version, source string) {
	if c.Plugins == nil {
		c.Plugins = make(map[string]PluginConfig)
	}
	pluginCfg := c.Plugins[name]
	pluginCfg.InstalledVersion = version
	pluginCfg.InstalledSource = source
	if pluginCfg.Config == nil {
		pluginCfg.Config = map[string]any{}
	}
	c.Plugins[name] = pluginCfg
}

func (c *Config) SetPluginEnabled(name string, enabled bool) {
	if c.Plugins == nil {
		c.Plugins = make(map[string]PluginConfig)
	}
	pluginCfg := c.Plugins[name]
	pluginCfg.Enabled = enabled
	if pluginCfg.Config == nil {
		pluginCfg.Config = map[string]any{}
	}
	c.Plugins[name] = pluginCfg
}

func (c *Config) SetPluginConfigValue(name, key string, value any) {
	if c.Plugins == nil {
		c.Plugins = make(map[string]PluginConfig)
	}
	pluginCfg := c.Plugins[name]
	if pluginCfg.Config == nil {
		pluginCfg.Config = make(map[string]any)
	}
	pluginCfg.Config[key] = value
	c.Plugins[name] = pluginCfg
}

func (c *Config) UnsetPluginConfigValue(name, key string) {
	if c.Plugins == nil {
		return
	}
	pluginCfg, ok := c.Plugins[name]
	if !ok || pluginCfg.Config == nil {
		return
	}
	delete(pluginCfg.Config, key)
	c.Plugins[name] = pluginCfg
}

func (c *Config) RemovePlugin(name string) {
	if c.Plugins != nil {
		delete(c.Plugins, name)
	}
	filtered := c.EnabledPlugins[:0]
	for _, enabled := range c.EnabledPlugins {
		if enabled != name {
			filtered = append(filtered, enabled)
		}
	}
	c.EnabledPlugins = filtered
}

func (c *Config) SetPluginSource(source PluginSource) {
	for i, existing := range c.PluginSources {
		if existing.Name == source.Name {
			c.PluginSources[i] = source
			return
		}
	}
	c.PluginSources = append(c.PluginSources, source)
}

func (c *Config) RemovePluginSource(name string) bool {
	for i, existing := range c.PluginSources {
		if existing.Name != name {
			continue
		}
		c.PluginSources = append(c.PluginSources[:i], c.PluginSources[i+1:]...)
		return true
	}
	return false
}
