package completion

import (
	"encoding/json"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/tidwall/gjson"
)

// Convert Chat Completions -> Responses 的语义转换
func Convert(modelName string, rawJSON []byte, stream bool) ([]byte, error) {
	input, err := convertCompletionMessages(gjson.GetBytes(rawJSON, "messages"))
	if err != nil {
		return nil, err
	}
	req := codexCompletionRequest{
		Model:  modelName,
		Stream: stream,
		Input:  input,
	}
	if v := gjson.GetBytes(rawJSON, "reasoning_effort"); v.Exists() {
		req.Reasoning = &codexCompletionReasoning{Effort: v.String()}
	}

	responseFormat := gjson.GetBytes(rawJSON, "response_format")
	text := gjson.GetBytes(rawJSON, "text")
	if responseFormat.Exists() || text.Get("verbosity").Exists() {
		req.Text = &codexCompletionText{}
		switch responseFormat.Get("type").String() {
		case "text":
			req.Text.Format = &codexCompletionTextFormat{Type: "text"}
		case "json_schema":
			if schema := responseFormat.Get("json_schema"); schema.Exists() {
				req.Text.Format = &codexCompletionTextFormat{
					Type:   "json_schema",
					Schema: rawJSONMessage(schema.Get("schema")),
				}
				if v := schema.Get("name"); v.Exists() {
					req.Text.Format.Name = v.String()
				}
				if v := schema.Get("strict"); v.Exists() {
					req.Text.Format.Strict = ptr(v.Bool())
				}
			}
		}
		if verbosity := text.Get("verbosity"); verbosity.Exists() {
			req.Text.Verbosity = rawJSONMessage(verbosity)
		}
	}

	if tools := gjson.GetBytes(rawJSON, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		req.Tools = make([]json.RawMessage, 0, len(tools.Array()))
		for _, tool := range tools.Array() {
			toolType := tool.Get("type").String()
			if toolType != "function" {
				return nil, fmt.Errorf("unsupported tool type %q", toolType)
			}
			definition := codexToolDefinition{
				Type:       "function",
				Name:       tool.Get("function.name").String(),
				Parameters: rawJSONMessage(tool.Get("function.parameters")),
			}
			if v := tool.Get("function.description"); v.Exists() {
				definition.Description = v.String()
			}
			if v := tool.Get("function.strict"); v.Exists() {
				definition.Strict = ptr(v.Bool())
			}
			if raw := marshalRaw(definition); len(raw) > 0 {
				req.Tools = append(req.Tools, raw)
			} else {
				return nil, fmt.Errorf("marshal tool definition %q", definition.Name)
			}
		}
	}

	if toolChoice := gjson.GetBytes(rawJSON, "tool_choice"); toolChoice.Exists() {
		switch {
		case toolChoice.Type == gjson.String:
			req.ToolChoice = json.RawMessage(toolChoice.Raw)
		case toolChoice.IsObject():
			choiceType := toolChoice.Get("type").String()
			if choiceType != "function" {
				return nil, fmt.Errorf("unsupported tool_choice type %q", choiceType)
			} else {
				req.ToolChoice = marshalRaw(struct {
					Type string `json:"type"`
					Name string `json:"name,omitempty"`
				}{
					Type: "function",
					Name: toolChoice.Get("function.name").String(),
				})
				if len(req.ToolChoice) == 0 {
					return nil, fmt.Errorf("marshal tool_choice")
				}
			}
		default:
			return nil, fmt.Errorf("unsupported tool_choice JSON type %v", toolChoice.Type)
		}
	}

	out, err := sonic.Marshal(req)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type codexCompletionRequest struct {
	Model      string                    `json:"model"`
	Stream     bool                      `json:"stream"`
	Reasoning  *codexCompletionReasoning `json:"reasoning,omitempty"`
	Input      []json.RawMessage         `json:"input"`
	Text       *codexCompletionText      `json:"text,omitempty"`
	Tools      []json.RawMessage         `json:"tools,omitempty"`
	ToolChoice json.RawMessage           `json:"tool_choice,omitempty"`
}

type codexCompletionReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type codexCompletionText struct {
	Format    *codexCompletionTextFormat `json:"format,omitempty"`
	Verbosity json.RawMessage            `json:"verbosity,omitempty"`
}

type codexCompletionTextFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name,omitempty"`
	Strict *bool           `json:"strict,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

type codexInputMessage struct {
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []codexContentPart `json:"content"`
}

type codexContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type codexFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type codexFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func convertCompletionMessages(messages gjson.Result) ([]json.RawMessage, error) {
	if !messages.IsArray() {
		return nil, fmt.Errorf("messages must be an array")
	}
	input := make([]json.RawMessage, 0, len(messages.Array()))
	for _, message := range messages.Array() {
		role := message.Get("role").String()
		if role == "tool" {
			input = appendRaw(input, codexFunctionCallOutput{
				Type:   "function_call_output",
				CallID: message.Get("tool_call_id").String(),
				Output: message.Get("content").String(),
			})
			continue
		}
		if role != "system" && role != "developer" && role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported message role %q", role)
		}

		codexRole := role
		if codexRole == "system" {
			codexRole = "developer"
		}
		content := message.Get("content")
		parts := make([]codexContentPart, 0)
		textPartType := "input_text"
		if role == "assistant" {
			textPartType = "output_text"
		}
		if content.Exists() && content.Type == gjson.String {
			if content.String() != "" {
				parts = append(parts, codexContentPart{Type: textPartType, Text: content.String()})
			}
		} else if content.IsArray() {
			for _, inputPart := range content.Array() {
				switch inputPart.Get("type").String() {
				case "text":
					parts = append(parts, codexContentPart{Type: textPartType, Text: inputPart.Get("text").String()})
				case "image_url":
					if role != "user" {
						return nil, fmt.Errorf("content part %q is unsupported for role %q", "image_url", role)
					} else {
						parts = append(parts, codexContentPart{Type: "input_image", ImageURL: inputPart.Get("image_url.url").String()})
					}
				case "file":
					if role != "user" {
						return nil, fmt.Errorf("content part %q is unsupported for role %q", "file", role)
					} else {
						if fileData := inputPart.Get("file.file_data").String(); fileData != "" {
							parts = append(parts, codexContentPart{
								Type:     "input_file",
								FileData: fileData,
								Filename: inputPart.Get("file.filename").String(),
							})
						}
					}
				default:
					return nil, fmt.Errorf("unsupported content part type %q", inputPart.Get("type").String())
				}
			}
		} else if content.Exists() && content.Type != gjson.Null {
			return nil, fmt.Errorf("message content for role %q must be a string, array, or null", role)
		}

		msg := codexInputMessage{
			Type:    "message",
			Role:    codexRole,
			Content: parts,
		}
		// 只有 tool_calls 的 assistant 消息不能发空 message，否则 Codex 会匹配不到 call_id
		if role != "assistant" || len(msg.Content) > 0 {
			input = appendRaw(input, msg)
		}
		if role == "assistant" {
			for _, toolCall := range message.Get("tool_calls").Array() {
				if toolCall.Get("type").String() != "function" {
					return nil, fmt.Errorf("unsupported tool_call type %q", toolCall.Get("type").String())
				}
				input = appendRaw(input, codexFunctionCall{
					Type:      "function_call",
					CallID:    toolCall.Get("id").String(),
					Name:      toolCall.Get("function.name").String(),
					Arguments: toolCall.Get("function.arguments").String(),
				})
			}
		}
	}
	return input, nil
}

type codexToolDefinition struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}
