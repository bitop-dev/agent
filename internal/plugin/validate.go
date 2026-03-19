package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
)

func ValidateManifest(manifest plg.Manifest) error {
	if manifest.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if manifest.Kind != "Plugin" {
		return fmt.Errorf("kind must be Plugin")
	}
	if manifest.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if manifest.Spec.Runtime.Type == "" {
		return fmt.Errorf("spec.runtime.type is required")
	}
	return nil
}

func ValidateConfig(manifest plg.Manifest, cfg config.PluginConfig) error {
	for _, required := range manifest.Spec.ConfigSchema.Required {
		value, ok := cfg.Config[required]
		if !ok || isZeroValue(value) {
			return fmt.Errorf("plugin %s missing required config %q", manifest.Metadata.Name, required)
		}
	}
	for name, property := range manifest.Spec.ConfigSchema.Properties {
		value, ok := cfg.Config[name]
		if !ok {
			continue
		}
		if err := validateProperty(name, value, property); err != nil {
			return fmt.Errorf("plugin %s config %q invalid: %w", manifest.Metadata.Name, name, err)
		}
	}
	return nil
}

func validateProperty(name string, value any, property plg.Property) error {
	switch property.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("must be a string")
		}
	case "integer":
		switch value.(type) {
		case int, int64, float64:
		default:
			return fmt.Errorf("must be an integer")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be a boolean")
		}
	case "array":
		switch value.(type) {
		case []any, []string:
		default:
			return fmt.Errorf("must be an array")
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("must be an object")
		}
	}
	if len(property.Enum) > 0 {
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("must be a string from enum")
		}
		for _, allowed := range property.Enum {
			if str == allowed {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s", strings.Join(property.Enum, ", "))
	}
	_ = name
	return nil
}

func ParseConfigValue(property plg.Property, raw string) (any, error) {
	switch property.Type {
	case "integer":
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("must be an integer")
		}
		return value, nil
	case "boolean":
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("must be a boolean")
		}
		return value, nil
	case "array":
		var value []any
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			return value, nil
		}
		parts := strings.Split(raw, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	case "object":
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, fmt.Errorf("must be a JSON object")
		}
		return value, nil
	default:
		return raw, nil
	}
}

func isZeroValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	}
	return false
}
