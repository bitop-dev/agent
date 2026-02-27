// Package bedrock implements ai.Provider for Amazon Bedrock's ConverseStream API.
//
// Authentication is handled by the AWS SDK v2 credential chain:
//  1. AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY (+ optional AWS_SESSION_TOKEN)
//  2. AWS_PROFILE — named profile from ~/.aws/credentials
//  3. ~/.aws/credentials default profile
//  4. IAM instance roles / ECS task roles / IRSA
//
// Configure in agent.yaml:
//
//	provider: bedrock
//	model:    us.anthropic.claude-opus-4-5-20251101-v1:0
//	region:   us-east-1      # optional; falls back to AWS_DEFAULT_REGION
package bedrock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/bitop-dev/agent/pkg/ai"
)

// Provider is the Amazon Bedrock streaming provider.
type Provider struct {
	Region  string
	Profile string
}

func New(region, profile string) *Provider {
	return &Provider{Region: region, Profile: profile}
}

func (p *Provider) Name() string { return "bedrock" }

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func (p *Provider) Stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	events := make(chan ai.StreamEvent, 64)
	var finalMsg *ai.AssistantMessage
	var finalErr error
	done := make(chan struct{})

	go func() {
		defer close(events)
		defer close(done)
		finalMsg, finalErr = p.stream(ctx, model, llmCtx, opts, events)
	}()

	return events, func() (*ai.AssistantMessage, error) {
		<-done
		return finalMsg, finalErr
	}
}

func (p *Provider) stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
	events chan<- ai.StreamEvent,
) (*ai.AssistantMessage, error) {
	client, err := p.newClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("bedrock: build client: %w", err)
	}

	input, err := p.buildInput(model, llmCtx, opts)
	if err != nil {
		return nil, fmt.Errorf("bedrock: build input: %w", err)
	}

	resp, err := client.ConverseStream(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock: ConverseStream: %w", err)
	}

	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     model,
		Provider:  "bedrock",
		Timestamp: time.Now().UnixMilli(),
	}

	events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}

	// blockIndex (from Bedrock) → index in partial.Content
	blockIdx := map[int32]int{}
	// blockIndex → accumulated tool-use args string
	toolArgs := map[int32]string{}

	stream := resp.GetStream()
	defer stream.Close()

	for event := range stream.Events() {
		switch ev := event.(type) {

		// ── ContentBlockStart ──────────────────────────────────────────────
		case *types.ConverseStreamOutputMemberContentBlockStart:
			cbIdx := *ev.Value.ContentBlockIndex
			switch s := ev.Value.Start.(type) {
			case *types.ContentBlockStartMemberToolUse:
				tu := s.Value
				partial.Content = append(partial.Content, ai.ToolCall{
					Type:      "tool_call",
					ID:        aws.ToString(tu.ToolUseId),
					Name:      aws.ToString(tu.Name),
					Arguments: map[string]any{},
				})
				blockIdx[cbIdx] = len(partial.Content) - 1
				events <- ai.StreamEvent{
					Type:    ai.StreamEventToolCallStart,
					Partial: snapshotMsg(partial),
					Delta:   aws.ToString(tu.Name),
				}
			default:
				// Text block start — allocate slot
				partial.Content = append(partial.Content, ai.TextContent{Type: "text", Text: ""})
				blockIdx[cbIdx] = len(partial.Content) - 1
				events <- ai.StreamEvent{Type: ai.StreamEventTextStart, Partial: snapshotMsg(partial)}
			}

		// ── ContentBlockDelta ──────────────────────────────────────────────
		case *types.ConverseStreamOutputMemberContentBlockDelta:
			cbIdx := *ev.Value.ContentBlockIndex
			contentIdx, ok := blockIdx[cbIdx]
			if !ok {
				continue
			}
			switch d := ev.Value.Delta.(type) {
			case *types.ContentBlockDeltaMemberText:
				tb := partial.Content[contentIdx].(ai.TextContent)
				tb.Text += d.Value
				partial.Content[contentIdx] = tb
				events <- ai.StreamEvent{Type: ai.StreamEventTextDelta, Partial: snapshotMsg(partial), Delta: d.Value}

			case *types.ContentBlockDeltaMemberToolUse:
				toolArgs[cbIdx] += aws.ToString(d.Value.Input)
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, Partial: snapshotMsg(partial), Delta: aws.ToString(d.Value.Input)}
			}

		// ── ContentBlockStop ───────────────────────────────────────────────
		case *types.ConverseStreamOutputMemberContentBlockStop:
			cbIdx := *ev.Value.ContentBlockIndex
			contentIdx, ok := blockIdx[cbIdx]
			if !ok {
				continue
			}
			switch c := partial.Content[contentIdx].(type) {
			case ai.TextContent:
				events <- ai.StreamEvent{Type: ai.StreamEventTextEnd, Partial: snapshotMsg(partial)}

			case ai.ToolCall:
				// Parse accumulated args
				if argsStr, exists := toolArgs[cbIdx]; exists {
					var args map[string]any
					_ = json.Unmarshal([]byte(argsStr), &args)
					c.Arguments = args
					partial.Content[contentIdx] = c
				}
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallEnd, Partial: snapshotMsg(partial)}
			}

		// ── MessageStop ────────────────────────────────────────────────────
		case *types.ConverseStreamOutputMemberMessageStop:
			partial.StopReason = mapStopReason(ev.Value.StopReason)

		// ── Metadata (usage) ───────────────────────────────────────────────
		case *types.ConverseStreamOutputMemberMetadata:
			if ev.Value.Usage != nil {
				u := ev.Value.Usage
				partial.Usage.Input = int(aws.ToInt32(u.InputTokens))
				partial.Usage.Output = int(aws.ToInt32(u.OutputTokens))
				partial.Usage.TotalTokens = int(aws.ToInt32(u.InputTokens)) + int(aws.ToInt32(u.OutputTokens))
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("bedrock: stream error: %w", err)
	}

	if partial.StopReason == "" {
		partial.StopReason = ai.StopReasonStop
	}
	for _, c := range partial.Content {
		if _, ok := c.(ai.ToolCall); ok {
			partial.StopReason = ai.StopReasonTool
			break
		}
	}

	events <- ai.StreamEvent{Type: ai.StreamEventDone, Partial: snapshotMsg(partial)}
	return partial, nil
}

// ---------------------------------------------------------------------------
// Client + input building
// ---------------------------------------------------------------------------

func (p *Provider) newClient(ctx context.Context) (*bedrockruntime.Client, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error
	if p.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(p.Region))
	}
	if p.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(p.Profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}
	return bedrockruntime.NewFromConfig(cfg), nil
}

func (p *Provider) buildInput(model string, llmCtx ai.Context, opts ai.StreamOptions) (*bedrockruntime.ConverseStreamInput, error) {
	input := &bedrockruntime.ConverseStreamInput{
		ModelId: aws.String(model),
	}

	if llmCtx.SystemPrompt != "" {
		sysBlocks := []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: llmCtx.SystemPrompt},
		}
		// Add a cache breakpoint when prompt caching is enabled.
		// Bedrock supports this for Claude models via a CachePoint block.
		if opts.CacheRetention != "" && opts.CacheRetention != "none" {
			sysBlocks = append(sysBlocks,
				&types.SystemContentBlockMemberCachePoint{
					Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
				},
			)
		}
		input.System = sysBlocks
	}

	ic := &types.InferenceConfiguration{}
	if opts.MaxTokens > 0 {
		v := int32(opts.MaxTokens)
		ic.MaxTokens = &v
	}
	if opts.Temperature != nil {
		v := float32(*opts.Temperature)
		ic.Temperature = &v
	}
	input.InferenceConfig = ic

	msgs, err := convertMessages(llmCtx.Messages)
	if err != nil {
		return nil, err
	}
	input.Messages = msgs

	if len(llmCtx.Tools) > 0 {
		toolList := make([]types.Tool, 0, len(llmCtx.Tools))
		for _, t := range llmCtx.Tools {
			var schema map[string]any
			_ = json.Unmarshal(t.Parameters, &schema)
			toolList = append(toolList, &types.ToolMemberToolSpec{
				Value: types.ToolSpecification{
					Name:        aws.String(t.Name),
					Description: aws.String(t.Description),
					InputSchema: &types.ToolInputSchemaMemberJson{
						Value: lazyDoc(schema),
					},
				},
			})
		}
		input.ToolConfig = &types.ToolConfiguration{
			Tools:      toolList,
			ToolChoice: &types.ToolChoiceMemberAuto{Value: types.AutoToolChoice{}},
		}
	}

	return input, nil
}

// ---------------------------------------------------------------------------
// Message conversion
// ---------------------------------------------------------------------------

func convertMessages(msgs []ai.Message) ([]types.Message, error) {
	var out []types.Message
	for _, m := range msgs {
		switch msg := m.(type) {
		case ai.UserMessage:
			var blocks []types.ContentBlock
			for _, c := range msg.Content {
				switch blk := c.(type) {
				case ai.TextContent:
					blocks = append(blocks, &types.ContentBlockMemberText{Value: blk.Text})
				case ai.ImageContent:
					imgBytes, _ := base64.StdEncoding.DecodeString(blk.Data)
					blocks = append(blocks, &types.ContentBlockMemberImage{
						Value: types.ImageBlock{
							Format: imageFormat(blk.MIMEType),
							Source: &types.ImageSourceMemberBytes{Value: imgBytes},
						},
					})
				}
			}
			out = append(out, types.Message{Role: types.ConversationRoleUser, Content: blocks})

		case ai.AssistantMessage:
			var blocks []types.ContentBlock
			for _, c := range msg.Content {
				switch blk := c.(type) {
				case ai.TextContent:
					if strings.TrimSpace(blk.Text) != "" {
						blocks = append(blocks, &types.ContentBlockMemberText{Value: blk.Text})
					}
				case ai.ToolCall:
					var inputMap map[string]any
					argsJSON, _ := json.Marshal(blk.Arguments)
					_ = json.Unmarshal(argsJSON, &inputMap)
					blocks = append(blocks, &types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							ToolUseId: aws.String(blk.ID),
							Name:      aws.String(blk.Name),
							Input:     lazyDoc(inputMap),
						},
					})
				}
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, types.Message{Role: types.ConversationRoleAssistant, Content: blocks})

		case ai.ToolResultMessage:
			var content []types.ToolResultContentBlock
			for _, c := range msg.Content {
				switch blk := c.(type) {
				case ai.TextContent:
					content = append(content, &types.ToolResultContentBlockMemberText{Value: blk.Text})
				case ai.ImageContent:
					imgBytes, _ := base64.StdEncoding.DecodeString(blk.Data)
					content = append(content, &types.ToolResultContentBlockMemberImage{
						Value: types.ImageBlock{
							Format: imageFormat(blk.MIMEType),
							Source: &types.ImageSourceMemberBytes{Value: imgBytes},
						},
					})
				}
			}
			status := types.ToolResultStatusSuccess
			if msg.IsError {
				status = types.ToolResultStatusError
			}
			toolResultBlock := &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(msg.ToolCallID),
					Status:    status,
					Content:   content,
				},
			}
			// Bedrock requires all tool results in the same user message
			if len(out) > 0 && out[len(out)-1].Role == types.ConversationRoleUser {
				out[len(out)-1].Content = append(out[len(out)-1].Content, toolResultBlock)
			} else {
				out = append(out, types.Message{
					Role:    types.ConversationRoleUser,
					Content: []types.ContentBlock{toolResultBlock},
				})
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func snapshotMsg(msg *ai.AssistantMessage) *ai.AssistantMessage {
	cp := *msg
	cp.Content = make([]ai.ContentBlock, len(msg.Content))
	copy(cp.Content, msg.Content)
	return &cp
}

func mapStopReason(r types.StopReason) ai.StopReason {
	switch r {
	case types.StopReasonEndTurn:
		return ai.StopReasonStop
	case types.StopReasonMaxTokens:
		return ai.StopReasonLength
	case types.StopReasonToolUse:
		return ai.StopReasonTool
	default:
		return ai.StopReasonStop
	}
}

func imageFormat(mimeType string) types.ImageFormat {
	switch mimeType {
	case "image/jpeg":
		return types.ImageFormatJpeg
	case "image/png":
		return types.ImageFormatPng
	case "image/gif":
		return types.ImageFormatGif
	case "image/webp":
		return types.ImageFormatWebp
	default:
		return types.ImageFormatPng
	}
}

// lazyDoc wraps a map[string]any as a Bedrock document.Interface.
func lazyDoc(m map[string]any) brdoc.Interface {
	return brdoc.NewLazyDocument(m)
}
