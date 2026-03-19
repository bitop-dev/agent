package workspace

import (
	"path/filepath"
	"strings"
)

type Workspace struct {
	Root string
}

func Resolve(path string) (Workspace, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Root: abs}, nil
}

func (w Workspace) Contains(path string) bool {
	if w.Root == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(w.Root, abs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
