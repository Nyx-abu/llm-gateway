package extproc

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

// TranslateOpenAIToAnthropic takes an OpenAI JSON request body and translates it to Anthropic format.
func TranslateOpenAIToAnthropic(body []byte) ([]byte, error) {
	var oReq OpenAIRequest
	if err := json.Unmarshal(body, &oReq); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %v", err)
	}

	// Dynamic model mapping
	anthropicModel := "claude-3-5-sonnet-20240620" // Default fallback model
	modelLower := strings.ToLower(oReq.Model)
	if strings.Contains(modelLower, "claude") {
		anthropicModel = oReq.Model // Keep client's requested Anthropic model name
	} else if strings.Contains(modelLower, "gpt-4o-mini") || strings.Contains(modelLower, "gpt-3.5") {
		anthropicModel = "claude-3-haiku-20240307"
	} else if strings.Contains(modelLower, "gpt-4") || strings.Contains(modelLower, "gpt-4o") {
		anthropicModel = "claude-3-5-sonnet-20240620"
	} else if strings.Contains(modelLower, "o1") {
		anthropicModel = "claude-3-opus-20240229"
	}

	aReq := AnthropicRequest{
		Model:     anthropicModel,
		MaxTokens: 1024, // Default required by Anthropic
	}

	// Map max_tokens
	if oReq.MaxTokens != nil {
		aReq.MaxTokens = *oReq.MaxTokens
	}

	// Map temperature
	if oReq.Temperature != nil {
		aReq.Temperature = oReq.Temperature
	}

	// Map stream
	aReq.Stream = oReq.Stream

	// Extract system messages and map them to Anthropic's top-level "system" parameter
	var systemPrompts []string
	for _, m := range oReq.Messages {
		if strings.ToLower(m.Role) == "system" {
			systemPrompts = append(systemPrompts, m.Content)
		} else {
			role := m.Role
			aReq.Messages = append(aReq.Messages, AnthropicMessage{
				Role:    role,
				Content: m.Content,
			})
		}
	}

	if len(systemPrompts) > 0 {
		aReq.System = strings.Join(systemPrompts, "\n")
	}

	// In case there were no non-system messages, Anthropic requires at least one user message
	if len(aReq.Messages) == 0 {
		aReq.Messages = append(aReq.Messages, AnthropicMessage{
			Role:    "user",
			Content: "Hello",
		})
	}

	return json.Marshal(aReq)
}

// TranslateAnthropicToOpenAI takes an Anthropic JSON request body and translates it to OpenAI format.
func TranslateAnthropicToOpenAI(body []byte) ([]byte, error) {
	var aReq AnthropicRequest
	if err := json.Unmarshal(body, &aReq); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %v", err)
	}

	// Dynamic model mapping
	openAIModel := "gpt-4o" // Default fallback model
	modelLower := strings.ToLower(aReq.Model)
	if strings.Contains(modelLower, "gpt") || strings.Contains(modelLower, "o1") {
		openAIModel = aReq.Model // Keep client's requested OpenAI model name
	} else if strings.Contains(modelLower, "haiku") {
		openAIModel = "gpt-4o-mini"
	} else if strings.Contains(modelLower, "sonnet") {
		openAIModel = "gpt-4o"
	} else if strings.Contains(modelLower, "opus") {
		openAIModel = "o1-preview"
	}

	oReq := OpenAIRequest{
		Model: openAIModel,
	}

	// Map max_tokens (Anthropic's max_tokens maps to OpenAI's max_tokens)
	if aReq.MaxTokens > 0 {
		maxTokensVal := aReq.MaxTokens
		oReq.MaxTokens = &maxTokensVal
	}

	// Map temperature
	if aReq.Temperature != nil {
		oReq.Temperature = aReq.Temperature
	}

	// Map stream
	oReq.Stream = aReq.Stream

	// Handle system prompt
	if aReq.System != "" {
		oReq.Messages = append(oReq.Messages, OpenAIMessage{
			Role:    "system",
			Content: aReq.System,
		})
	}

	// Map messages
	for _, m := range aReq.Messages {
		oReq.Messages = append(oReq.Messages, OpenAIMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return json.Marshal(oReq)
}
