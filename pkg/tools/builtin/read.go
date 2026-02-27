package builtin

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools"
)

// imageExtensions maps lowercase file extensions to MIME types.
var imageExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// ReadTool reads files — text with pagination/truncation, or images as base64.
type ReadTool struct {
	cwd string
}

func NewReadTool(cwd string) *ReadTool { return &ReadTool{cwd: cwd} }

func (t *ReadTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "read",
		Description: fmt.Sprintf(
			"Read the contents of a file. Supports text files and images (jpg, png, gif, webp). "+
				"Images are sent as attachments. For text files, output is truncated to %d lines or %s "+
				"(whichever is hit first). Use offset/limit for large files. "+
				"When you need the full file, continue with offset until complete.",
			DefaultMaxLines, FormatSize(DefaultMaxBytes),
		),
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"path":   {Type: "string", Description: "Path to the file to read (relative or absolute)"},
				"offset": {Type: "integer", Description: "Line number to start reading from (1-indexed)"},
				"limit":  {Type: "integer", Description: "Maximum number of lines to read"},
			},
			Required: []string{"path"},
		}),
	}
}

func (t *ReadTool) Execute(ctx context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	pathParam, _ := params["path"].(string)
	if pathParam == "" {
		return tools.ErrorResult(fmt.Errorf("path is required")), nil
	}

	absPath := resolvePath(pathParam, t.cwd)

	// Image?
	if mimeType, ok := imageExtensions[strings.ToLower(filepath.Ext(absPath))]; ok {
		return t.readImage(absPath, mimeType, pathParam)
	}
	return t.readText(ctx, absPath, pathParam, params)
}

func (t *ReadTool) readImage(absPath, mimeType, displayPath string) (tools.Result, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot read %s: %w", displayPath, err)), nil
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return tools.Result{
		Content: []ai.ContentBlock{
			ai.TextContent{Type: "text", Text: fmt.Sprintf("Read image file [%s]", mimeType)},
			ai.ImageContent{Type: "image", Data: encoded, MIMEType: mimeType},
		},
	}, nil
}

func (t *ReadTool) readText(_ context.Context, absPath, displayPath string, params map[string]any) (tools.Result, error) {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot read %s: %w", displayPath, err)), nil
	}

	text := string(raw)
	allLines := strings.Split(normalizeToLF(text), "\n")
	totalFileLines := len(allLines)

	// Parse offset (1-indexed) and limit
	offset := 0
	if v, ok := params["offset"]; ok {
		switch n := v.(type) {
		case float64:
			offset = int(n)
		case int:
			offset = n
		}
	}
	hasLimit := false
	limit := 0
	if v, ok := params["limit"]; ok {
		hasLimit = true
		switch n := v.(type) {
		case float64:
			limit = int(n)
		case int:
			limit = n
		}
	}

	startLine := 0 // 0-indexed
	if offset > 0 {
		startLine = offset - 1
	}
	if startLine >= totalFileLines {
		return tools.ErrorResult(fmt.Errorf("offset %d is beyond end of file (%d lines total)", offset, totalFileLines)), nil
	}

	var selected string
	var userLimitedLines int
	if hasLimit && limit > 0 {
		endLine := min(startLine+limit, totalFileLines)
		selected = joinLines(allLines[startLine:endLine])
		userLimitedLines = endLine - startLine
	} else {
		selected = joinLines(allLines[startLine:])
	}

	tr := TruncateHead(selected, DefaultMaxLines, DefaultMaxBytes)
	startDisplay := startLine + 1 // 1-indexed for display

	var outputText string

	switch {
	case tr.FirstLineExceedsLimit:
		firstLineSize := FormatSize(len([]byte(allLines[startLine])))
		outputText = fmt.Sprintf(
			"[Line %d is %s, exceeds %s limit. Use bash: sed -n '%dp' %s | head -c %d]",
			startDisplay, firstLineSize, FormatSize(DefaultMaxBytes), startDisplay, displayPath, DefaultMaxBytes,
		)

	case tr.Truncated:
		endLineDisplay := startDisplay + tr.OutputLines - 1
		nextOffset := endLineDisplay + 1
		outputText = tr.Content
		if tr.TruncatedBy == "lines" {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				startDisplay, endLineDisplay, totalFileLines, nextOffset,
			)
		} else {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startDisplay, endLineDisplay, totalFileLines, FormatSize(DefaultMaxBytes), nextOffset,
			)
		}

	case hasLimit && userLimitedLines > 0 && startLine+userLimitedLines < totalFileLines:
		remaining := totalFileLines - (startLine + userLimitedLines)
		nextOffset := startLine + userLimitedLines + 1
		outputText = tr.Content
		outputText += fmt.Sprintf(
			"\n\n[%d more lines in file. Use offset=%d to continue.]",
			remaining, nextOffset,
		)

	default:
		outputText = tr.Content
	}

	return tools.TextResult(outputText), nil
}
