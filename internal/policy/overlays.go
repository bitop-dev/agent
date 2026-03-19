package policy

import (
	"fmt"
	"path/filepath"

	loaderutil "github.com/ncecere/agent/internal/loader"
	"github.com/ncecere/agent/pkg/policy"
)

type overlayFile struct {
	Version int           `yaml:"version"`
	Rules   []overlayRule `yaml:"rules"`
}

type overlayRule struct {
	ID       string `yaml:"id"`
	Action   string `yaml:"action"`
	Tool     string `yaml:"tool,omitempty"`
	Decision string `yaml:"decision"`
}

func LoadToolOverrides(profilePath string, overlays []string) (map[string]policy.DecisionKind, error) {
	if len(overlays) == 0 {
		return map[string]policy.DecisionKind{}, nil
	}
	baseDir := filepath.Dir(profilePath)
	result := make(map[string]policy.DecisionKind)
	for _, overlay := range overlays {
		path := overlay
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, overlay)
		}
		parsed, err := loaderutil.LoadYAML[overlayFile](path)
		if err != nil {
			return nil, err
		}
		for _, rule := range parsed.Rules {
			if rule.Action != string(policy.ActionTool) || rule.Tool == "" {
				continue
			}
			decision, err := parseDecision(rule.Decision)
			if err != nil {
				return nil, fmt.Errorf("overlay rule %s: %w", rule.ID, err)
			}
			result[rule.Tool] = decision
		}
	}
	return result, nil
}

func parseDecision(input string) (policy.DecisionKind, error) {
	switch policy.DecisionKind(input) {
	case policy.DecisionAllow, policy.DecisionDeny, policy.DecisionRequireApproval:
		return policy.DecisionKind(input), nil
	default:
		return "", fmt.Errorf("unsupported decision %q", input)
	}
}
