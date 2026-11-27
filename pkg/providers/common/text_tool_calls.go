package common

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Some OpenAI-compatible models (notably free models behind a gateway) send
// tool calls as plain text instead of structured tool_calls. The shape looks
// like:
//
//	[tool_use: edit_file<arg_key:ID>path</arg_key:ID><arg_value:ID>...</arg_value:ID></tool_call:ID></tool_calls:ID>
//
// This file recognizes that shape so it never reaches the chat transcript and
// the tool is executed instead.
const textToolOpen = "[tool_use:"

var textNumberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)

var thinkBlockPattern = regexp.MustCompile(`(?s)<think:[^>]+>.*?</think:[^>]+>`)

// NormalizeThinkTags removes inline thinking blocks. Some models put a draft
// followed by </think:ID> and then repeat the final answer; only the text after
// the last closing tag is visible.
func NormalizeThinkTags(content string) string {
	content = thinkBlockPattern.ReplaceAllString(content, "")
	if index := strings.LastIndex(content, "</think:"); index >= 0 {
		if end := strings.IndexByte(content[index:], '>'); end >= 0 {
			content = content[index+end+1:]
		}
	}
	return strings.TrimSpace(content)
}

// NormalizeTextToolCalls strips textual tool calls from Content and turns them
// into structured ToolCalls when the provider did not send any.
func NormalizeTextToolCalls(response *LLMResponse) {
	if response == nil {
		return
	}
	response.Content = NormalizeThinkTags(response.Content)
	clean, calls := ParseTextToolCalls(response.Content)
	response.Content = clean
	if len(calls) == 0 {
		return
	}
	if len(response.ToolCalls) == 0 {
		response.ToolCalls = calls
		if strings.TrimSpace(response.FinishReason) == "" ||
			strings.EqualFold(response.FinishReason, "stop") {
			response.FinishReason = "tool_calls"
		}
	}
}

// VisibleStreamContent hides an in-progress textual tool call or thinking draft
// from the live stream while keeping visible prose.
func VisibleStreamContent(accumulated string) string {
	visible := accumulated
	if index := strings.LastIndex(visible, "</think:"); index >= 0 {
		if end := strings.IndexByte(visible[index:], '>'); end >= 0 {
			visible = visible[index+end+1:]
		}
	} else if index := strings.Index(visible, "<think:"); index >= 0 {
		visible = visible[:index]
	}
	if index := strings.Index(visible, textToolOpen); index >= 0 {
		visible = visible[:index]
	}
	return strings.TrimRight(visible, " \t\r\n")
}

// ParseTextToolCalls returns the visible text and the parsed tool calls. A
// partial block at the end is removed from the visible text but does not create
// a call, because executing half an edit would be unsafe.
func ParseTextToolCalls(content string) (string, []ToolCall) {
	if !strings.Contains(content, textToolOpen) {
		return content, nil
	}
	var clean strings.Builder
	var calls []ToolCall
	remaining := content
	for {
		index := strings.Index(remaining, textToolOpen)
		if index < 0 {
			clean.WriteString(remaining)
			break
		}
		clean.WriteString(remaining[:index])
		rest, call, complete := parseTextToolBlock(remaining[index:])
		if !complete {
			break
		}
		calls = append(calls, call)
		remaining = rest
	}
	return strings.TrimSpace(clean.String()), calls
}

func parseTextToolBlock(block string) (string, ToolCall, bool) {
	if !strings.HasPrefix(block, textToolOpen) {
		return block, ToolCall{}, false
	}
	after := block[len(textToolOpen):]
	nameEnd := strings.IndexByte(after, '<')
	if nameEnd <= 0 {
		return "", ToolCall{}, false
	}
	name := strings.TrimSpace(after[:nameEnd])
	body := after[nameEnd:]

	id := textToolCallID(body)
	if id == "" {
		return "", ToolCall{}, false
	}
	closing := "</tool_call:" + id + ">"
	closingIndex := strings.Index(body, closing)
	if closingIndex < 0 {
		closing = "</tool_calls:" + id + ">"
		closingIndex = strings.Index(body, closing)
	}
	if closingIndex < 0 {
		return "", ToolCall{}, false
	}
	args := parseTextToolArguments(body[:closingIndex], id)
	if name == "" {
		return "", ToolCall{}, false
	}
	toolCall := ToolCall{ID: id, Name: name, Arguments: args}
	rest := body[closingIndex+len(closing):]
	extra := "</tool_calls:" + id + ">"
	if strings.HasPrefix(rest, extra) {
		rest = rest[len(extra):]
	}
	return rest, toolCall, true
}

func textToolCallID(body string) string {
	index := strings.Index(body, "<arg_key:")
	if index < 0 {
		return ""
	}
	rest := body[index+len("<arg_key:"):]
	end := strings.IndexByte(rest, '>')
	if end <= 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func parseTextToolArguments(body, id string) map[string]any {
	args := make(map[string]any)
	keyOpen := "<arg_key:" + id + ">"
	keyClose := "</arg_key:" + id + ">"
	valueOpen := "<arg_value:" + id + ">"
	valueClose := "</arg_value:" + id + ">"
	position := 0
	for {
		keyStart := strings.Index(body[position:], keyOpen)
		if keyStart < 0 {
			break
		}
		keyStart += position
		keyBodyStart := keyStart + len(keyOpen)
		keyEnd := strings.Index(body[keyBodyStart:], keyClose)
		if keyEnd < 0 {
			break
		}
		keyEnd += keyBodyStart
		key := strings.TrimSpace(body[keyBodyStart:keyEnd])

		valueTagStart := keyEnd + len(keyClose)
		for valueTagStart < len(body) && (body[valueTagStart] == ' ' ||
			body[valueTagStart] == '\t' || body[valueTagStart] == '\r' ||
			body[valueTagStart] == '\n') {
			valueTagStart++
		}
		if !strings.HasPrefix(body[valueTagStart:], valueOpen) {
			break
		}
		valueStart := valueTagStart + len(valueOpen)
		valueEnd := strings.Index(body[valueStart:], valueClose)
		if valueEnd < 0 {
			break
		}
		valueEnd += valueStart
		if key != "" {
			args[key] = decodeTextToolValue(body[valueStart:valueEnd])
		}
		position = valueEnd + len(valueClose)
	}
	return args
}

func decodeTextToolValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	first := trimmed[0]
	if first != '{' && first != '[' && trimmed != "true" && trimmed != "false" &&
		trimmed != "null" && !textNumberPattern.MatchString(trimmed) {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		return decoded
	}
	return value
}
