package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

// OpenAIRequest represents the structure of an OpenAI chat completion request.
type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChoice represents a single choice in the OpenAI completion response.
type OpenAIChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// OpenAIUsage represents token usage statistics.
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIResponse represents the structure of an OpenAI completion response.
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// AnthropicRequest represents the structure of an Anthropic message request.
type AnthropicRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// AnthropicContent represents a content block in an Anthropic response.
type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnthropicUsage represents token usage statistics for Anthropic.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicResponse represents the structure of an Anthropic message response.
type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Model        string             `json:"model"`
	Content      []AnthropicContent `json:"content"`
	StopReason   string             `json:"stop_reason"`
	StopSequence interface{}        `json:"stop_sequence"`
	Usage        AnthropicUsage     `json:"usage"`
}

// ErrorResponse represents a generic error payload.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// handleInjection returns the status code to use, and a boolean indicating whether the injection handled (wrote) the response.
func handleInjection(w http.ResponseWriter, r *http.Request, provider string) (int, bool) {
	// 1. Delay Injection
	delayStr := ""
	if provider == "openai" {
		delayStr = r.Header.Get("x-inject-openai-delay-ms")
	} else if provider == "anthropic" {
		delayStr = r.Header.Get("x-inject-anthropic-delay-ms")
	}
	if delayStr == "" {
		delayStr = r.Header.Get("x-inject-delay")
	}
	if delayStr != "" {
		if d, err := strconv.Atoi(delayStr); err == nil && d > 0 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
	}

	// 2. Connection Drop Injection
	if r.Header.Get("x-inject-connection-drop") == "true" {
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, err := hijacker.Hijack()
			if err == nil {
				conn.Close()
				return 0, true
			}
		}
		panic(http.ErrAbortHandler)
	}

	// Determine status code
	statusStr := ""
	if provider == "openai" {
		statusStr = r.Header.Get("x-inject-openai-status")
	} else if provider == "anthropic" {
		statusStr = r.Header.Get("x-inject-anthropic-status")
	}
	if statusStr == "" {
		statusStr = r.Header.Get("x-inject-status")
	}

	status := 200
	if statusStr != "" {
		if s, err := strconv.Atoi(statusStr); err == nil {
			status = s
		}
	} else if r.Header.Get("x-inject-error") == "true" || r.URL.Query().Get("inject_error") == "true" {
		status = 500
	}

	// 3. Corrupt Body Injection
	if r.Header.Get("x-inject-corrupt-body") == "true" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(`{"invalid_json": true, "details": "truncated json`))
		return status, true
	}

	// 4. Status Injection (if status indicates an error >= 400)
	if status >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if provider == "openai" {
			w.Write([]byte(fmt.Sprintf(`{"error":{"message":"Injected OpenAI error with status %d","type":"api_error","code":"internal_error"}}`, status)))
		} else {
			w.Write([]byte(fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":"Injected Anthropic error with status %d"}}`, status)))
		}
		return status, true
	}

	return status, false
}

// handleOpenAICompletions serves mock OpenAI completions.
func handleOpenAICompletions(w http.ResponseWriter, r *http.Request) {
	log.Printf("Mock Server received request to %s", r.URL.Path)
	for k, v := range r.Header {
		log.Printf("  Header: %s = %s", k, v)
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error":{"message":"Method not allowed","type":"invalid_request_error"}}`))
		return
	}

	status, handled := handleInjection(w, r, "openai")
	if handled {
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Failed to read request body","type":"invalid_request_error"}}`))
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req OpenAIRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid JSON","type":"invalid_request_error"}}`))
		return
	}

	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}

	response := OpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Hello! I am a mock OpenAI response for model %s.", model),
				},
				FinishReason: "stop",
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 15,
			TotalTokens:      25,
		},
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Failed to marshal response","type":"api_error"}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(respBytes)
}

// handleAnthropicMessages serves mock Anthropic messages.
func handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Method not allowed"}}`))
		return
	}

	status, handled := handleInjection(w, r, "anthropic")
	if handled {
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Failed to read request body"}}`))
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid JSON"}}`))
		return
	}

	model := req.Model
	if model == "" {
		model = "claude-3-5-sonnet"
	}

	response := AnthropicResponse{
		ID:    fmt.Sprintf("msg_mock_%d", time.Now().UnixNano()),
		Type:  "message",
		Role:  "assistant",
		Model: model,
		Content: []AnthropicContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Hello! I am a mock Anthropic response for model %s.", model),
			},
		},
		StopReason:   "end_turn",
		StopSequence: nil,
		Usage: AnthropicUsage{
			InputTokens:  10,
			OutputTokens: 15,
		},
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Failed to marshal response"}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(respBytes)
}

func main() {
	port := os.Getenv("MOCK_SERVER_PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/openai/v1/chat/completions", handleOpenAICompletions)
	mux.HandleFunc("/anthropic/v1/messages", handleAnthropicMessages)

	// Basic health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	addr := ":" + port
	log.Printf("Starting mock upstream server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
