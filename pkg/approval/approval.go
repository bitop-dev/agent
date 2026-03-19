package approval

import "context"

type Mode string

const (
	ModeNever     Mode = "never"
	ModeOnRequest Mode = "on-request"
	ModeAlways    Mode = "always"
)

type Request struct {
	Action string
	ToolID string
	Reason string
	Risk   string
}

type Decision struct {
	Approved bool
	Reason   string
}

type Resolver interface {
	Resolve(ctx context.Context, req Request) (Decision, error)
}
