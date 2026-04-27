package completion

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/tidwall/gjson"
)

// convertStreamResponseToCompletions 将一行 Codex SSE data 转成 0..N 个 Chat Completions chunk
func convertStreamResponseToCompletions(rawJSON []byte, stateRef **completionResponseState) ([][]byte, bool, error) {
	state := completionState(stateRef)
	payload := sseDataPayload(rawJSON)
	root := gjson.ParseBytes(payload)

	switch root.Get("type").String() {
	case "response.created":
		if !root.Get("response.id").Exists() || !root.Get("response.created_at").Exists() || !root.Get("response.model").Exists() {
			return nil, false, fmt.Errorf("response.created missing required response metadata")
		}
		state.ResponseID = root.Get("response.id").String()
		state.CreatedAt = root.Get("response.created_at").Int()
		state.Model = root.Get("response.model").String()
		return nil, false, nil
	case "response.reasoning_summary_text.delta":
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.ReasoningContent = root.Get("delta").String()
		return marshalStreamChunk(chunk)
	case "response.reasoning_summary_text.done":
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.ReasoningContent = "\n\n"
		return marshalStreamChunk(chunk)
	case "response.output_text.delta":
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.Content = root.Get("delta").String()
		return marshalStreamChunk(chunk)
	case "response.image_generation_call.partial_image":
		image, err := completionImageDelta(root.Get("item_id").String(), root.Get("partial_image_b64").String(), root.Get("output_format").String(), state, 0)
		if err != nil {
			return nil, false, err
		}
		if image == nil {
			return nil, false, nil
		}
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.Images = []chatCompletionImage{*image}
		return marshalStreamChunk(chunk)
	case "response.completed":
		finishReason := "stop"
		if state.FunctionCallIndex != -1 {
			finishReason = "tool_calls"
		}
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].FinishReason = ptr(finishReason)
		chunk.Choices[0].NativeFinishReason = ptr(finishReason)
		chunks, _, err := marshalStreamChunk(chunk)
		return chunks, true, err
	case "response.output_item.added":
		item := root.Get("item")
		if !item.Exists() || item.Get("type").String() != "function_call" {
			return nil, false, nil
		}
		state.FunctionCallIndex++
		state.HasReceivedArgumentsDelta = false
		state.HasToolCallAnnounced = true
		toolCall := completionToolCallChunk(state.FunctionCallIndex, item, true)
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.ToolCalls = []chatCompletionToolDelta{toolCall}
		return marshalStreamChunk(chunk)
	case "response.function_call_arguments.delta":
		state.HasReceivedArgumentsDelta = true
		toolCall := completionToolArgumentsChunk(state.FunctionCallIndex, root.Get("delta").String())
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.ToolCalls = []chatCompletionToolDelta{toolCall}
		return marshalStreamChunk(chunk)
	case "response.function_call_arguments.done":
		if state.HasReceivedArgumentsDelta {
			return nil, false, nil
		}
		toolCall := completionToolArgumentsChunk(state.FunctionCallIndex, root.Get("arguments").String())
		chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
		if err != nil {
			return nil, false, err
		}
		chunk.Choices[0].Delta.Role = "assistant"
		chunk.Choices[0].Delta.ToolCalls = []chatCompletionToolDelta{toolCall}
		return marshalStreamChunk(chunk)
	case "response.output_item.done":
		item := root.Get("item")
		if !item.Exists() {
			return nil, false, fmt.Errorf("response.output_item.done missing item")
		}
		switch item.Get("type").String() {
		case "image_generation_call":
			image, err := completionImageDelta(item.Get("id").String(), item.Get("result").String(), item.Get("output_format").String(), state, 0)
			if err != nil {
				return nil, false, err
			}
			if image == nil {
				return nil, false, nil
			}
			chunk, err := newCompletionStreamChunk(payload, root.Get("response.usage"), state)
			if err != nil {
				return nil, false, err
			}
			chunk.Choices[0].Delta.Role = "assistant"
			chunk.Choices[0].Delta.Images = []chatCompletionImage{*image}
			return marshalStreamChunk(chunk)
		case "function_call":
			if state.HasToolCallAnnounced {
				state.HasToolCallAnnounced = false
				return nil, false, nil
			}
			state.FunctionCallIndex++
			toolCall := completionToolCallChunk(state.FunctionCallIndex, item, false)
			chunk, err := newCompletionStreamChunk(payload, gjson.Result{}, state)
			if err != nil {
				return nil, false, err
			}
			chunk.Choices[0].Delta.Role = "assistant"
			chunk.Choices[0].Delta.ToolCalls = []chatCompletionToolDelta{toolCall}
			return marshalStreamChunk(chunk)
		default:
			return nil, false, nil
		}
	default:
		return nil, false, nil
	}
}

// convertCodexCompletionResponseToOpenAINonStream 将完整 response.completed 事件转成 Chat Completions JSON
func convertCodexCompletionResponseToOpenAINonStream(rawJSON []byte) ([]byte, error) {
	root := gjson.ParseBytes(rawJSON)
	if root.Get("type").String() != "response.completed" {
		return nil, fmt.Errorf("expected response.completed event, got %q", root.Get("type").String())
	}
	response := root.Get("response")
	msg := chatCompletionMessage{Role: "assistant"}
	var toolCalls []chatCompletionToolCall
	var images []chatCompletionImage

	for _, outputItem := range response.Get("output").Array() {
		switch outputItem.Get("type").String() {
		case "reasoning":
			reasoning := ""
			for _, summary := range outputItem.Get("summary").Array() {
				if summary.Get("type").String() == "summary_text" {
					reasoning = summary.Get("text").String()
					break
				}
			}
			msg.ReasoningContent = ptr(reasoning)
		case "message":
			contentText := ""
			for _, content := range outputItem.Get("content").Array() {
				if content.Get("type").String() == "output_text" {
					contentText = content.Get("text").String()
					break
				}
			}
			msg.Content = ptr(contentText)
		case "function_call":
			toolCalls = append(toolCalls, completionToolCallMessage(outputItem))
		case "image_generation_call":
			image, err := completionImage(outputItem.Get("result").String(), outputItem.Get("output_format").String(), len(images))
			if err != nil {
				return nil, err
			}
			if image != nil {
				images = append(images, *image)
			}
		}
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	if len(images) > 0 {
		msg.Images = images
	}

	finishReason := ""
	if response.Get("status").String() == "completed" {
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	}
	createdAt := time.Now().Unix()
	if v := response.Get("created_at"); v.Exists() {
		createdAt = v.Int()
	}
	out := chatCompletionResponse{
		ID:      response.Get("id").String(),
		Object:  "chat.completion",
		Created: createdAt,
		Model:   response.Get("model").String(),
		Choices: []chatCompletionChoice{{
			Index:              0,
			Message:            msg,
			FinishReason:       ptr(finishReason),
			NativeFinishReason: ptr(finishReason),
		}},
		Usage: completionUsageFromGJSON(response.Get("usage")),
	}
	body, err := sonic.Marshal(out)
	if err != nil {
		return []byte{}, nil
	}
	return body, nil
}

type completionResponseState struct {
	ResponseID                string
	CreatedAt                 int64
	Model                     string
	FunctionCallIndex         int
	HasReceivedArgumentsDelta bool
	HasToolCallAnnounced      bool
	LastImageHashByItemID     map[string][32]byte
}

func completionState(stateRef **completionResponseState) *completionResponseState {
	if stateRef == nil {
		return &completionResponseState{FunctionCallIndex: -1, LastImageHashByItemID: map[string][32]byte{}}
	}
	if *stateRef == nil {
		*stateRef = &completionResponseState{FunctionCallIndex: -1, LastImageHashByItemID: map[string][32]byte{}}
	}
	state := *stateRef
	if state.LastImageHashByItemID == nil {
		state.LastImageHashByItemID = map[string][32]byte{}
	}
	return state
}

type chatCompletionStreamChunk struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []chatCompletionStreamChoice `json:"choices"`
	Usage   *completionUsage             `json:"usage,omitempty"`
}

type chatCompletionStreamChoice struct {
	Index              int                 `json:"index"`
	Delta              chatCompletionDelta `json:"delta"`
	FinishReason       *string             `json:"finish_reason"`
	NativeFinishReason *string             `json:"native_finish_reason"`
}

type chatCompletionDelta struct {
	Role             string                    `json:"role,omitempty"`
	Content          string                    `json:"content,omitempty"`
	ReasoningContent string                    `json:"reasoning_content,omitempty"`
	ToolCalls        []chatCompletionToolDelta `json:"tool_calls,omitempty"`
	Images           []chatCompletionImage     `json:"images,omitempty"`
}

type chatCompletionToolDelta struct {
	Index    int                         `json:"index"`
	ID       string                      `json:"id,omitempty"`
	Type     string                      `json:"type,omitempty"`
	Function chatCompletionFunctionDelta `json:"function"`
}

type chatCompletionFunctionDelta struct {
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

func newCompletionStreamChunk(payload []byte, usage gjson.Result, state *completionResponseState) (chatCompletionStreamChunk, error) {
	model := state.Model
	if v := gjson.GetBytes(payload, "model"); v.Exists() {
		model = v.String()
	}
	if model == "" {
		return chatCompletionStreamChunk{}, fmt.Errorf("response stream missing model metadata")
	}
	return chatCompletionStreamChunk{
		ID:      state.ResponseID,
		Object:  "chat.completion.chunk",
		Created: state.CreatedAt,
		Model:   model,
		Choices: []chatCompletionStreamChoice{{
			Index:        0,
			Delta:        chatCompletionDelta{},
			FinishReason: nil,
		}},
		Usage: completionUsageFromGJSON(usage),
	}, nil
}

func marshalStreamChunk(chunk chatCompletionStreamChunk) ([][]byte, bool, error) {
	body, err := sonic.Marshal(chunk)
	if err != nil {
		return nil, false, err
	}
	return [][]byte{body}, false, nil
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *completionUsage       `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index              int                   `json:"index"`
	Message            chatCompletionMessage `json:"message"`
	FinishReason       *string               `json:"finish_reason"`
	NativeFinishReason *string               `json:"native_finish_reason"`
}

type chatCompletionMessage struct {
	Role             string                   `json:"role"`
	Content          *string                  `json:"content"`
	ReasoningContent *string                  `json:"reasoning_content"`
	ToolCalls        []chatCompletionToolCall `json:"tool_calls,omitempty"`
	Images           []chatCompletionImage    `json:"images,omitempty"`
}

type chatCompletionToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function chatCompletionFunction `json:"function"`
}

type chatCompletionFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionImage struct {
	Index    int                    `json:"index"`
	Type     string                 `json:"type"`
	ImageURL chatCompletionImageURL `json:"image_url"`
}

type chatCompletionImageURL struct {
	URL string `json:"url"`
}

type completionUsage struct {
	PromptTokens            *int64                        `json:"prompt_tokens,omitempty"`
	CompletionTokens        *int64                        `json:"completion_tokens,omitempty"`
	TotalTokens             *int64                        `json:"total_tokens,omitempty"`
	PromptTokensDetails     *completionPromptTokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionOutputTokenDetails `json:"completion_tokens_details,omitempty"`
}

type completionPromptTokenDetails struct {
	CachedTokens *int64 `json:"cached_tokens,omitempty"`
}

type completionOutputTokenDetails struct {
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}

func completionUsageFromGJSON(usage gjson.Result) *completionUsage {
	if !usage.Exists() {
		return nil
	}
	out := &completionUsage{}
	if v := usage.Get("input_tokens"); v.Exists() {
		out.PromptTokens = ptr(v.Int())
	}
	if v := usage.Get("output_tokens"); v.Exists() {
		out.CompletionTokens = ptr(v.Int())
	}
	if v := usage.Get("total_tokens"); v.Exists() {
		out.TotalTokens = ptr(v.Int())
	}
	if v := usage.Get("input_tokens_details.cached_tokens"); v.Exists() {
		out.PromptTokensDetails = &completionPromptTokenDetails{CachedTokens: ptr(v.Int())}
	}
	if v := usage.Get("output_tokens_details.reasoning_tokens"); v.Exists() {
		out.CompletionTokensDetails = &completionOutputTokenDetails{ReasoningTokens: ptr(v.Int())}
	}
	return out
}

func completionToolCallChunk(index int, item gjson.Result, announce bool) chatCompletionToolDelta {
	name := item.Get("name").String()
	arguments := ""
	toolCall := chatCompletionToolDelta{
		Index: index,
		ID:    item.Get("call_id").String(),
		Type:  "function",
		Function: chatCompletionFunctionDelta{
			Name:      &name,
			Arguments: &arguments,
		},
	}
	if !announce {
		toolCall.Function.Arguments = ptr(item.Get("arguments").String())
	}
	return toolCall
}

func completionToolArgumentsChunk(index int, arguments string) chatCompletionToolDelta {
	return chatCompletionToolDelta{
		Index: index,
		Function: chatCompletionFunctionDelta{
			Arguments: &arguments,
		},
	}
}

func completionToolCallMessage(item gjson.Result) chatCompletionToolCall {
	return chatCompletionToolCall{
		ID:   item.Get("call_id").String(),
		Type: "function",
		Function: chatCompletionFunction{
			Name:      item.Get("name").String(),
			Arguments: item.Get("arguments").String(),
		},
	}
}

func completionImageDelta(itemID string, b64 string, outputFormat string, state *completionResponseState, index int) (*chatCompletionImage, error) {
	if b64 == "" {
		return nil, nil
	}
	if itemID != "" {
		hash := sha256.Sum256([]byte(b64))
		if last, ok := state.LastImageHashByItemID[itemID]; ok && last == hash {
			return nil, nil
		}
		state.LastImageHashByItemID[itemID] = hash
	}
	return completionImage(b64, outputFormat, index)
}

func completionImage(b64 string, outputFormat string, index int) (*chatCompletionImage, error) {
	if b64 == "" {
		return nil, nil
	}
	mimeType, err := mimeTypeFromCodexOutputFormat(outputFormat)
	if err != nil {
		return nil, err
	}
	return &chatCompletionImage{
		Index: index,
		Type:  "image_url",
		ImageURL: chatCompletionImageURL{
			URL: "data:" + mimeType + ";base64," + b64,
		},
	}, nil
}
