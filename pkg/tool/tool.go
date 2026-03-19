package tool

import "context"

type Definition struct {
	ID          string
	Description string
	Schema      map[string]any
}

type Call struct {
	ID        string
	ToolID    string
	Arguments map[string]any
}

type Result struct {
	ToolID string
	Output string
	Data   map[string]any
}

type Tool interface {
	Definition() Definition
	Run(ctx context.Context, call Call) (Result, error)
}

type Registry interface {
	Register(tool Tool) error
	Get(id string) (Tool, bool)
	List() []Definition
}
