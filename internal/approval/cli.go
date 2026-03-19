package approval

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ncecere/agent/pkg/approval"
)

type CLIResolver struct {
	Mode   approval.Mode
	Reader io.Reader
	Writer io.Writer
}

func (r CLIResolver) Resolve(_ context.Context, req approval.Request) (approval.Decision, error) {
	switch r.Mode {
	case approval.ModeAlways:
		return approval.Decision{Approved: true, Reason: "auto-approved by mode=always"}, nil
	case approval.ModeNever:
		return approval.Decision{Approved: false, Reason: "denied by mode=never"}, nil
	default:
		reader := bufio.NewReader(r.Reader)
		if _, err := fmt.Fprintf(r.Writer, "Approve %s for tool %s? [y/N]: ", req.Action, req.ToolID); err != nil {
			return approval.Decision{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return approval.Decision{}, err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		approved := line == "y" || line == "yes"
		return approval.Decision{Approved: approved}, nil
	}
}
