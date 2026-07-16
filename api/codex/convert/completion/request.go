package completion

import (
	"fmt"

	"github.com/bytedance/sonic/ast"
)

// Convert 将 Chat Completions AST 直接转换成 Responses AST。
// 转换过程不经过完整请求的中间字节表示，最终请求只由 relay 统一序列化一次。
func Convert(root *ast.Node) (*ast.Node, error) {
	if nodeType(root) != ast.V_OBJECT {
		return nil, fmt.Errorf("chat completions request must be an object")
	}

	model, _, err := optionalString(root.Get("model"), "model")
	if err != nil {
		return nil, err
	}
	input, err := convertCompletionMessages(root.Get("messages"))
	if err != nil {
		return nil, err
	}

	pairs := []ast.Pair{
		ast.NewPair("model", ast.NewString(model)),
	}
	if value, present, err := optionalString(root.Get("prompt_cache_key"), "prompt_cache_key"); err != nil {
		return nil, err
	} else if present && value != "" {
		pairs = append(pairs, ast.NewPair("prompt_cache_key", ast.NewString(value)))
	}
	if effort, present, err := optionalString(root.Get("reasoning_effort"), "reasoning_effort"); err != nil {
		return nil, err
	} else if present && effort != "" {
		pairs = append(pairs, ast.NewPair("reasoning", ast.NewObject([]ast.Pair{
			ast.NewPair("effort", ast.NewString(effort)),
		})))
	}
	pairs = append(pairs, ast.NewPair("input", ast.NewArray(input)))

	text, present, err := convertCompletionText(root)
	if err != nil {
		return nil, err
	}
	if present {
		pairs = append(pairs, ast.NewPair("text", text))
	}
	tools, present, err := convertCompletionTools(root.Get("tools"))
	if err != nil {
		return nil, err
	}
	if present {
		pairs = append(pairs, ast.NewPair("tools", ast.NewArray(tools)))
	}
	toolChoice, present, err := convertCompletionToolChoice(root.Get("tool_choice"))
	if err != nil {
		return nil, err
	}
	if present {
		pairs = append(pairs, ast.NewPair("tool_choice", toolChoice))
	}
	if value, present, err := optionalString(root.Get("service_tier"), "service_tier"); err != nil {
		return nil, err
	} else if present && value != "" {
		pairs = append(pairs, ast.NewPair("service_tier", ast.NewString(value)))
	}

	converted := ast.NewObject(pairs)
	return &converted, nil
}

func convertCompletionText(root *ast.Node) (ast.Node, bool, error) {
	responseFormat := root.Get("response_format")
	responseFormatType := nodeType(responseFormat)
	if responseFormatType != ast.V_NONE && responseFormatType != ast.V_NULL && responseFormatType != ast.V_OBJECT {
		return ast.Node{}, false, fmt.Errorf("response_format must be an object")
	}
	verbosity, verbosityPresent, err := optionalString(root.Get("verbosity"), "verbosity")
	if err != nil {
		return ast.Node{}, false, err
	}
	verbosityPresent = verbosityPresent && verbosity != ""
	responseFormatPresent := responseFormatType == ast.V_OBJECT
	if !responseFormatPresent && !verbosityPresent {
		return ast.Node{}, false, nil
	}

	pairs := make([]ast.Pair, 0, 2)
	if responseFormatPresent {
		formatType, _, err := optionalString(responseFormat.Get("type"), "response_format.type")
		if err != nil {
			return ast.Node{}, false, err
		}
		switch formatType {
		case "text", "json_object":
			pairs = append(pairs, ast.NewPair("format", ast.NewObject([]ast.Pair{
				ast.NewPair("type", ast.NewString(formatType)),
			})))
		case "json_schema":
			schemaConfig := responseFormat.Get("json_schema")
			if nodeType(schemaConfig) != ast.V_OBJECT {
				return ast.Node{}, false, fmt.Errorf("response_format.json_schema must be an object")
			}
			formatPairs := []ast.Pair{ast.NewPair("type", ast.NewString("json_schema"))}
			if name, present, err := optionalString(schemaConfig.Get("name"), "response_format.json_schema.name"); err != nil {
				return ast.Node{}, false, err
			} else if present && name != "" {
				formatPairs = append(formatPairs, ast.NewPair("name", ast.NewString(name)))
			}
			if description, present, err := optionalString(schemaConfig.Get("description"), "response_format.json_schema.description"); err != nil {
				return ast.Node{}, false, err
			} else if present && description != "" {
				formatPairs = append(formatPairs, ast.NewPair("description", ast.NewString(description)))
			}
			if strict, present, err := optionalBool(schemaConfig.Get("strict"), "response_format.json_schema.strict"); err != nil {
				return ast.Node{}, false, err
			} else if present {
				formatPairs = append(formatPairs, ast.NewPair("strict", ast.NewBool(strict)))
			}
			if schema := schemaConfig.Get("schema"); nodeType(schema) != ast.V_NONE && nodeType(schema) != ast.V_NULL {
				formatPairs = append(formatPairs, ast.NewPair("schema", *schema))
			}
			pairs = append(pairs, ast.NewPair("format", ast.NewObject(formatPairs)))
		default:
			return ast.Node{}, false, fmt.Errorf("unsupported response_format type %q", formatType)
		}
	}
	if verbosityPresent {
		pairs = append(pairs, ast.NewPair("verbosity", ast.NewString(verbosity)))
	}
	return ast.NewObject(pairs), true, nil
}

func convertCompletionTools(node *ast.Node) ([]ast.Node, bool, error) {
	typ := nodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil, false, nil
	}
	if typ != ast.V_ARRAY {
		return nil, false, fmt.Errorf("tools must be an array")
	}
	length, err := node.Len()
	if err != nil {
		return nil, false, err
	}
	if length == 0 {
		return nil, false, nil
	}
	tools := make([]ast.Node, 0, length)
	for index := 0; index < length; index++ {
		tool := node.Index(index)
		if nodeType(tool) != ast.V_OBJECT {
			return nil, false, fmt.Errorf("tool %d must be an object", index)
		}
		toolType, _, err := optionalString(tool.Get("type"), fmt.Sprintf("tools[%d].type", index))
		if err != nil {
			return nil, false, err
		}
		if toolType != "function" {
			return nil, false, fmt.Errorf("unsupported tool type %q", toolType)
		}
		function := tool.Get("function")
		if nodeType(function) != ast.V_OBJECT {
			return nil, false, fmt.Errorf("tools[%d].function must be an object", index)
		}
		name, _, err := optionalString(function.Get("name"), fmt.Sprintf("tools[%d].function.name", index))
		if err != nil {
			return nil, false, err
		}
		definition := []ast.Pair{ast.NewPair("type", ast.NewString("function"))}
		if name != "" {
			definition = append(definition, ast.NewPair("name", ast.NewString(name)))
		}
		if description, present, err := optionalString(function.Get("description"), fmt.Sprintf("tools[%d].function.description", index)); err != nil {
			return nil, false, err
		} else if present && description != "" {
			definition = append(definition, ast.NewPair("description", ast.NewString(description)))
		}
		if parameters := function.Get("parameters"); nodeType(parameters) != ast.V_NONE && nodeType(parameters) != ast.V_NULL {
			definition = append(definition, ast.NewPair("parameters", *parameters))
		}
		if strict, present, err := optionalBool(function.Get("strict"), fmt.Sprintf("tools[%d].function.strict", index)); err != nil {
			return nil, false, err
		} else if present {
			definition = append(definition, ast.NewPair("strict", ast.NewBool(strict)))
		}
		tools = append(tools, ast.NewObject(definition))
	}
	return tools, true, nil
}

func convertCompletionToolChoice(node *ast.Node) (ast.Node, bool, error) {
	switch nodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return ast.Node{}, false, nil
	case ast.V_STRING:
		value, err := node.StrictString()
		if err != nil {
			return ast.Node{}, false, err
		}
		return ast.NewString(value), true, nil
	case ast.V_OBJECT:
		choiceType, _, err := optionalString(node.Get("type"), "tool_choice.type")
		if err != nil {
			return ast.Node{}, false, err
		}
		if choiceType != "function" {
			return ast.Node{}, false, fmt.Errorf("unsupported tool_choice type %q", choiceType)
		}
		function := node.Get("function")
		if nodeType(function) != ast.V_OBJECT {
			return ast.Node{}, false, fmt.Errorf("tool_choice.function must be an object")
		}
		name, _, err := optionalString(function.Get("name"), "tool_choice.function.name")
		if err != nil {
			return ast.Node{}, false, err
		}
		pairs := []ast.Pair{ast.NewPair("type", ast.NewString("function"))}
		if name != "" {
			pairs = append(pairs, ast.NewPair("name", ast.NewString(name)))
		}
		return ast.NewObject(pairs), true, nil
	default:
		return ast.Node{}, false, fmt.Errorf("tool_choice must be a string or object")
	}
}

func convertCompletionMessages(messages *ast.Node) ([]ast.Node, error) {
	if nodeType(messages) != ast.V_ARRAY {
		return nil, fmt.Errorf("messages must be an array")
	}
	length, err := messages.Len()
	if err != nil {
		return nil, err
	}
	input := make([]ast.Node, 0, length)
	for index := 0; index < length; index++ {
		message := messages.Index(index)
		if nodeType(message) != ast.V_OBJECT {
			return nil, fmt.Errorf("message %d must be an object", index)
		}
		role, _, err := optionalString(message.Get("role"), fmt.Sprintf("messages[%d].role", index))
		if err != nil {
			return nil, err
		}
		if role == "tool" {
			callID, _, err := optionalString(message.Get("tool_call_id"), fmt.Sprintf("messages[%d].tool_call_id", index))
			if err != nil {
				return nil, err
			}
			output, err := stringOrJSON(message.Get("content"))
			if err != nil {
				return nil, fmt.Errorf("messages[%d].content: %w", index, err)
			}
			input = append(input, ast.NewObject([]ast.Pair{
				ast.NewPair("type", ast.NewString("function_call_output")),
				ast.NewPair("call_id", ast.NewString(callID)),
				ast.NewPair("output", ast.NewString(output)),
			}))
			continue
		}
		if role != "system" && role != "developer" && role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported message role %q", role)
		}

		codexRole := role
		if codexRole == "system" {
			codexRole = "developer"
		}
		parts, err := convertCompletionContent(message.Get("content"), role, index)
		if err != nil {
			return nil, err
		}
		if role != "assistant" || len(parts) > 0 {
			input = append(input, ast.NewObject([]ast.Pair{
				ast.NewPair("type", ast.NewString("message")),
				ast.NewPair("role", ast.NewString(codexRole)),
				ast.NewPair("content", ast.NewArray(parts)),
			}))
		}
		if role == "assistant" {
			calls, err := convertCompletionToolCalls(message.Get("tool_calls"), index)
			if err != nil {
				return nil, err
			}
			input = append(input, calls...)
		}
	}
	return input, nil
}

func convertCompletionContent(content *ast.Node, role string, messageIndex int) ([]ast.Node, error) {
	textPartType := "input_text"
	if role == "assistant" {
		textPartType = "output_text"
	}
	switch nodeType(content) {
	case ast.V_NONE, ast.V_NULL:
		return nil, nil
	case ast.V_STRING:
		text, err := content.StrictString()
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return []ast.Node{ast.NewObject([]ast.Pair{
			ast.NewPair("type", ast.NewString(textPartType)),
			ast.NewPair("text", ast.NewString(text)),
		})}, nil
	case ast.V_ARRAY:
		length, err := content.Len()
		if err != nil {
			return nil, err
		}
		parts := make([]ast.Node, 0, length)
		for index := 0; index < length; index++ {
			inputPart := content.Index(index)
			if nodeType(inputPart) != ast.V_OBJECT {
				return nil, fmt.Errorf("messages[%d].content[%d] must be an object", messageIndex, index)
			}
			partType, _, err := optionalString(inputPart.Get("type"), fmt.Sprintf("messages[%d].content[%d].type", messageIndex, index))
			if err != nil {
				return nil, err
			}
			switch partType {
			case "text":
				text, _, err := optionalString(inputPart.Get("text"), fmt.Sprintf("messages[%d].content[%d].text", messageIndex, index))
				if err != nil {
					return nil, err
				}
				pairs := []ast.Pair{ast.NewPair("type", ast.NewString(textPartType))}
				if text != "" {
					pairs = append(pairs, ast.NewPair("text", ast.NewString(text)))
				}
				parts = append(parts, ast.NewObject(pairs))
			case "image_url":
				if role != "user" {
					return nil, fmt.Errorf("content part %q is unsupported for role %q", partType, role)
				}
				imageURL, _, err := optionalString(inputPart.Get("image_url").Get("url"), fmt.Sprintf("messages[%d].content[%d].image_url.url", messageIndex, index))
				if err != nil {
					return nil, err
				}
				pairs := []ast.Pair{ast.NewPair("type", ast.NewString("input_image"))}
				if imageURL != "" {
					pairs = append(pairs, ast.NewPair("image_url", ast.NewString(imageURL)))
				}
				parts = append(parts, ast.NewObject(pairs))
			case "file":
				if role != "user" {
					return nil, fmt.Errorf("content part %q is unsupported for role %q", partType, role)
				}
				file := inputPart.Get("file")
				if nodeType(file) != ast.V_OBJECT {
					return nil, fmt.Errorf("messages[%d].content[%d].file must be an object", messageIndex, index)
				}
				fileData, _, err := optionalString(file.Get("file_data"), fmt.Sprintf("messages[%d].content[%d].file.file_data", messageIndex, index))
				if err != nil {
					return nil, err
				}
				if fileData == "" {
					continue
				}
				pairs := []ast.Pair{
					ast.NewPair("type", ast.NewString("input_file")),
					ast.NewPair("file_data", ast.NewString(fileData)),
				}
				if filename, present, err := optionalString(file.Get("filename"), fmt.Sprintf("messages[%d].content[%d].file.filename", messageIndex, index)); err != nil {
					return nil, err
				} else if present && filename != "" {
					pairs = append(pairs, ast.NewPair("filename", ast.NewString(filename)))
				}
				parts = append(parts, ast.NewObject(pairs))
			default:
				return nil, fmt.Errorf("unsupported content part type %q", partType)
			}
		}
		return parts, nil
	default:
		return nil, fmt.Errorf("message content for role %q must be a string, array, or null", role)
	}
}

func convertCompletionToolCalls(node *ast.Node, messageIndex int) ([]ast.Node, error) {
	typ := nodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil, nil
	}
	if typ != ast.V_ARRAY {
		return nil, fmt.Errorf("messages[%d].tool_calls must be an array", messageIndex)
	}
	length, err := node.Len()
	if err != nil {
		return nil, err
	}
	calls := make([]ast.Node, 0, length)
	for index := 0; index < length; index++ {
		toolCall := node.Index(index)
		if nodeType(toolCall) != ast.V_OBJECT {
			return nil, fmt.Errorf("messages[%d].tool_calls[%d] must be an object", messageIndex, index)
		}
		callType, _, err := optionalString(toolCall.Get("type"), fmt.Sprintf("messages[%d].tool_calls[%d].type", messageIndex, index))
		if err != nil {
			return nil, err
		}
		if callType != "function" {
			return nil, fmt.Errorf("unsupported tool_call type %q", callType)
		}
		function := toolCall.Get("function")
		if nodeType(function) != ast.V_OBJECT {
			return nil, fmt.Errorf("messages[%d].tool_calls[%d].function must be an object", messageIndex, index)
		}
		callID, _, err := optionalString(toolCall.Get("id"), fmt.Sprintf("messages[%d].tool_calls[%d].id", messageIndex, index))
		if err != nil {
			return nil, err
		}
		name, _, err := optionalString(function.Get("name"), fmt.Sprintf("messages[%d].tool_calls[%d].function.name", messageIndex, index))
		if err != nil {
			return nil, err
		}
		arguments, _, err := optionalString(function.Get("arguments"), fmt.Sprintf("messages[%d].tool_calls[%d].function.arguments", messageIndex, index))
		if err != nil {
			return nil, err
		}
		calls = append(calls, ast.NewObject([]ast.Pair{
			ast.NewPair("type", ast.NewString("function_call")),
			ast.NewPair("call_id", ast.NewString(callID)),
			ast.NewPair("name", ast.NewString(name)),
			ast.NewPair("arguments", ast.NewString(arguments)),
		}))
	}
	return calls, nil
}

func optionalString(node *ast.Node, field string) (string, bool, error) {
	switch nodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return "", false, nil
	case ast.V_STRING:
		value, err := node.StrictString()
		return value, true, err
	default:
		return "", false, fmt.Errorf("%s must be a string", field)
	}
}

func optionalBool(node *ast.Node, field string) (bool, bool, error) {
	switch nodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return false, false, nil
	case ast.V_TRUE, ast.V_FALSE:
		value, err := node.StrictBool()
		return value, true, err
	default:
		return false, false, fmt.Errorf("%s must be a boolean", field)
	}
}

func stringOrJSON(node *ast.Node) (string, error) {
	switch nodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return "", nil
	case ast.V_STRING:
		return node.StrictString()
	default:
		raw, err := node.MarshalJSON()
		return string(raw), err
	}
}

func nodeType(node *ast.Node) int {
	if node == nil || !node.Exists() {
		return ast.V_NONE
	}
	if err := node.Load(); err != nil {
		return ast.V_ERROR
	}
	return node.TypeSafe()
}
