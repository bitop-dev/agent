package ai_test

import (
	"testing"

	"github.com/nickcecere/agent/pkg/ai"
)

// TestImageContent_InUserMessage verifies that ImageContent can be embedded
// in a UserMessage alongside TextContent. This is the foundation for
// multimodal input — the provider serialization tests below exercise
// the wire format for each provider.
func TestImageContent_InUserMessage(t *testing.T) {
	msg := ai.UserMessage{
		Role: ai.RoleUser,
		Content: []ai.ContentBlock{
			ai.TextContent{Type: "text", Text: "What is in this image?"},
			ai.ImageContent{Type: "image", Data: "iVBOR...", MIMEType: "image/png"},
		},
		Timestamp: 1700000000000,
	}

	if msg.GetRole() != ai.RoleUser {
		t.Error("role should be user")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(msg.Content))
	}

	// Verify type assertions work.
	if _, ok := msg.Content[0].(ai.TextContent); !ok {
		t.Error("first block should be TextContent")
	}
	img, ok := msg.Content[1].(ai.ImageContent)
	if !ok {
		t.Fatal("second block should be ImageContent")
	}
	if img.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q", img.MIMEType)
	}
	if img.Data != "iVBOR..." {
		t.Errorf("Data = %q", img.Data)
	}
}

// TestImageContent_InToolResult verifies that ImageContent works in tool results.
// Some tools (e.g. screenshot) return images to the LLM.
func TestImageContent_InToolResult(t *testing.T) {
	msg := ai.ToolResultMessage{
		Role:       ai.RoleToolResult,
		ToolCallID: "call_1",
		ToolName:   "screenshot",
		Content: []ai.ContentBlock{
			ai.TextContent{Type: "text", Text: "Screenshot captured:"},
			ai.ImageContent{Type: "image", Data: "base64data", MIMEType: "image/jpeg"},
		},
		Timestamp: 1700000000000,
	}

	if msg.GetRole() != ai.RoleToolResult {
		t.Error("role should be tool_result")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(msg.Content))
	}

	img, ok := msg.Content[1].(ai.ImageContent)
	if !ok {
		t.Fatal("second block should be ImageContent")
	}
	if img.MIMEType != "image/jpeg" {
		t.Errorf("MIMEType = %q", img.MIMEType)
	}
}

// TestImageContent_SatisfiesContentBlock ensures ImageContent implements ContentBlock.
func TestImageContent_SatisfiesContentBlock(t *testing.T) {
	var _ ai.ContentBlock = ai.ImageContent{}
}

// TestImageContent_AllMIMETypes verifies common MIME types are storable.
func TestImageContent_AllMIMETypes(t *testing.T) {
	types := []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"image/svg+xml",
	}
	for _, mt := range types {
		img := ai.ImageContent{Type: "image", Data: "dGVzdA==", MIMEType: mt}
		if img.MIMEType != mt {
			t.Errorf("MIMEType = %q, want %q", img.MIMEType, mt)
		}
	}
}
