package loader

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadYAML[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
