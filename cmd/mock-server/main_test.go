package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleOpenAICompletions_HappyPath(t *testing.T) {
	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hello"}]}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var openAIResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if openAIResp.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", openAIResp.Model)
	}
	if len(openAIResp.Choices) == 0 {
		t.Fatalf("expected at least one choice")
	}
	if !strings.Contains(openAIResp.Choices[0].Message.Content, "gpt-4o") {
		t.Errorf("expected response to contain model name, got %s", openAIResp.Choices[0].Message.Content)
	}
}

func TestHandleOpenAICompletions_ErrorInjectionHeader(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-error", "true")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if errResp.Error.Message == "" {
		t.Errorf("expected error message")
	}
}

func TestHandleOpenAICompletions_ErrorInjectionQueryParam(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions?inject_error=true", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
}

func TestHandleAnthropicMessages_HappyPath(t *testing.T) {
	reqBody := `{"model": "claude-3-5-sonnet", "messages": [{"role": "user", "content": "hello"}]}`
	req := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handleAnthropicMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var anthropicResp AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if anthropicResp.Model != "claude-3-5-sonnet" {
		t.Errorf("expected model claude-3-5-sonnet, got %s", anthropicResp.Model)
	}
	if len(anthropicResp.Content) == 0 {
		t.Fatalf("expected at least one content block")
	}
	if !strings.Contains(anthropicResp.Content[0].Text, "claude-3-5-sonnet") {
		t.Errorf("expected response to contain model name, got %s", anthropicResp.Content[0].Text)
	}
}

func TestHandleAnthropicMessages_ErrorInjectionHeader(t *testing.T) {
	reqBody := `{"model": "claude-3-5-sonnet"}`
	req := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-error", "true")
	w := httptest.NewRecorder()

	handleAnthropicMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	var errResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	errMap, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", errResp)
	}
	if errMap["message"] == "" {
		t.Errorf("expected error message")
	}
}

func TestHandleAnthropicMessages_ErrorInjectionQueryParam(t *testing.T) {
	reqBody := `{"model": "claude-3-5-sonnet"}`
	req := httptest.NewRequest("POST", "/anthropic/v1/messages?inject_error=true", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handleAnthropicMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
}

func TestHandleOpenAICompletions_ProviderSpecificStatus(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-openai-status", "502")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", resp.StatusCode)
	}
}

func TestHandleAnthropicMessages_ProviderSpecificStatus(t *testing.T) {
	reqBody := `{"model": "claude-3-5-sonnet"}`
	req := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-anthropic-status", "503")
	w := httptest.NewRecorder()

	handleAnthropicMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestHandleOpenAICompletions_GenericStatus(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-status", "429")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", resp.StatusCode)
	}
}

func TestHandleOpenAICompletions_Delay(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-delay", "10")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleOpenAICompletions_ConnectionDrop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			if r != http.ErrAbortHandler {
				t.Errorf("unexpected panic value: %v", r)
			}
		} else {
			t.Errorf("expected handler to panic with http.ErrAbortHandler")
		}
	}()

	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-connection-drop", "true")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)
}

func TestHandleOpenAICompletions_CorruptBody(t *testing.T) {
	reqBody := `{"model": "gpt-4o"}`
	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("x-inject-corrupt-body", "true")
	req.Header.Set("x-inject-status", "200")
	w := httptest.NewRecorder()

	handleOpenAICompletions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	bodyStr := buf.String()

	if !strings.Contains(bodyStr, "invalid_json") || strings.HasSuffix(bodyStr, "}") {
		t.Errorf("expected body to be corrupted JSON, got: %s", bodyStr)
	}
}
