package builtin

import (
	"github.com/bitop-dev/agent/pkg/tools"
)

// Preset selects which built-in tools are registered.
type Preset string

const (
	// PresetCoding registers read, bash, edit, write — the default for an
	// agent that needs to read and modify files.
	PresetCoding Preset = "coding"

	// PresetReadOnly registers read, grep, find, ls — safe for exploration
	// without modification.
	PresetReadOnly Preset = "readonly"

	// PresetAll registers all built-in tools including web search and fetch.
	PresetAll Preset = "all"

	// PresetWeb registers web_search and web_fetch only.
	PresetWeb Preset = "web"

	// PresetNone registers nothing; useful when you only want plugin tools.
	PresetNone Preset = "none"
)

// Register adds the tools for the given preset to the registry.
// cwd is the working directory all file tools operate from.
// Pass an empty string to use the process working directory.
func Register(reg *tools.Registry, preset Preset, cwd string) {
	if cwd == "" {
		cwd = "."
	}

	switch preset {
	case PresetCoding:
		reg.Register(NewReadTool(cwd))
		reg.Register(NewBashTool(cwd))
		reg.Register(NewEditTool(cwd))
		reg.Register(NewWriteTool(cwd))

	case PresetReadOnly:
		reg.Register(NewReadTool(cwd))
		reg.Register(NewGrepTool(cwd))
		reg.Register(NewFindTool(cwd))
		reg.Register(NewLsTool(cwd))

	case PresetAll:
		reg.Register(NewReadTool(cwd))
		reg.Register(NewBashTool(cwd))
		reg.Register(NewEditTool(cwd))
		reg.Register(NewWriteTool(cwd))
		reg.Register(NewGrepTool(cwd))
		reg.Register(NewFindTool(cwd))
		reg.Register(NewLsTool(cwd))
		reg.Register(NewWebSearchTool())
		reg.Register(NewWebFetchTool())

	case PresetWeb:
		reg.Register(NewWebSearchTool())
		reg.Register(NewWebFetchTool())

	case PresetNone:
		// nothing
	}
}

// Individual constructors are exported so callers can mix and match.
// e.g.:  reg.Register(builtin.NewReadTool(cwd))
