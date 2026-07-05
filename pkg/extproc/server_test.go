package extproc

import (
	"context"
	"io"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc/metadata"
)

type mockProcessServer struct {
	ctx       context.Context
	requests  []*extprocv3.ProcessingRequest
	responses []*extprocv3.ProcessingResponse
	reqIndex  int
}

func (m *mockProcessServer) Send(resp *extprocv3.ProcessingResponse) error {
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockProcessServer) Recv() (*extprocv3.ProcessingRequest, error) {
	if m.reqIndex >= len(m.requests) {
		return nil, io.EOF
	}
	req := m.requests[m.reqIndex]
	m.reqIndex++
	return req, nil
}

func (m *mockProcessServer) SetHeader(metadata.MD) error  { return nil }
func (m *mockProcessServer) SendHeader(metadata.MD) error { return nil }
func (m *mockProcessServer) SetTrailer(metadata.MD)       {}
func (m *mockProcessServer) Context() context.Context      { return m.ctx }
func (m *mockProcessServer) SendMsg(msg interface{}) error  { return nil }
func (m *mockProcessServer) RecvMsg(msg interface{}) error  { return nil }

func makeRequestHeaders(path, model string, extraHeaders map[string]string) *extprocv3.ProcessingRequest {
	headersList := []*corev3.HeaderValueOption{
		{Header: &corev3.HeaderValue{Key: ":path", Value: path}},
	}
	if model != "" {
		headersList = append(headersList, &corev3.HeaderValueOption{Header: &corev3.HeaderValue{Key: "x-model", Value: model}})
	}
	for k, v := range extraHeaders {
		headersList = append(headersList, &corev3.HeaderValueOption{Header: &corev3.HeaderValue{Key: k, Value: v}})
	}

	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: headersList,
				},
			},
		},
	}
}

func makeResponseHeaders(status string) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_ResponseHeaders{
			ResponseHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: []*corev3.HeaderValueOption{
						{Header: &corev3.HeaderValue{Key: ":status", Value: status}},
					},
				},
			},
		},
	}
}

func getSetHeader(resp *extprocv3.ProcessingResponse, key string) string {
	var mutation *extprocv3.HeaderMutation
	switch r := resp.Response.(type) {
	case *extprocv3.ProcessingResponse_RequestHeaders:
		if r.RequestHeaders != nil && r.RequestHeaders.Response != nil {
			mutation = r.RequestHeaders.Response.HeaderMutation
		}
	case *extprocv3.ProcessingResponse_ResponseHeaders:
		if r.ResponseHeaders != nil && r.ResponseHeaders.Response != nil {
			mutation = r.ResponseHeaders.Response.HeaderMutation
		}
	}
	if mutation == nil {
		return ""
	}
	for _, h := range mutation.SetHeaders {
		if h.Header != nil && strings.ToLower(h.Header.Key) == strings.ToLower(key) {
			return h.Header.Value
		}
	}
	return ""
}

func TestProcess_ModelRouting(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		model            string
		expectedProvider string
	}{
		{"GPT model maps to openai", "/v1/chat/completions", "gpt-4o", "openai"},
		{"o1 model maps to openai", "/v1/chat/completions", "o1-mini", "openai"},
		{"Claude model maps to anthropic", "/v1/messages", "claude-3-5-sonnet", "anthropic"},
		{"Case insensitive GPT", "/v1/chat/completions", "GPT-4O", "openai"},
		{"Case insensitive Claude", "/v1/messages", "CLAUDE-3-OPUS", "anthropic"},
		{"Empty model on completions defaults to openai", "/v1/chat/completions", "", "openai"},
		{"Empty model on messages defaults to anthropic", "/v1/messages", "", "anthropic"},
		{"Unknown model defaults based on completions path", "/v1/chat/completions", "custom-model", "openai"},
		{"Unknown model defaults based on messages path", "/v1/messages", "custom-model", "anthropic"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer()
			stream := &mockProcessServer{
				ctx: context.Background(),
				requests: []*extprocv3.ProcessingRequest{
					makeRequestHeaders(tc.path, tc.model, nil),
				},
			}

			err := server.Process(stream)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(stream.responses) != 1 {
				t.Fatalf("expected 1 response, got %d", len(stream.responses))
			}

			provider := getSetHeader(stream.responses[0], "x-llm-provider")
			if provider != tc.expectedProvider {
				t.Errorf("expected provider %s, got %s", tc.expectedProvider, provider)
			}
		})
	}
}

func TestProcess_FallbackRedirect(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		model            string
		upstreamStatus   string
		isFallbackReq    bool
		expectedStatus   string
		expectedLocation string
	}{
		{
			name:             "OpenAI 500 error triggers fallback",
			path:             "/v1/chat/completions",
			model:            "gpt-4o",
			upstreamStatus:   "500",
			isFallbackReq:    false,
			expectedStatus:   "307",
			expectedLocation: "/v1/messages?fallback=true",
		},
		{
			name:             "Anthropic 503 error triggers fallback",
			path:             "/v1/messages",
			model:            "claude-3-5-sonnet",
			upstreamStatus:   "503",
			isFallbackReq:    false,
			expectedStatus:   "307",
			expectedLocation: "/v1/chat/completions?fallback=true",
		},
		{
			name:             "OpenAI 200 does not trigger fallback",
			path:             "/v1/chat/completions",
			model:            "gpt-4o",
			upstreamStatus:   "200",
			isFallbackReq:    false,
			expectedStatus:   "",
			expectedLocation: "",
		},
		{
			name:             "Fallback request returning 500 does not trigger loop redirect",
			path:             "/v1/messages?fallback=true",
			model:            "gpt-4o",
			upstreamStatus:   "500",
			isFallbackReq:    true,
			expectedStatus:   "",
			expectedLocation: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer()
			extra := map[string]string{}
			if tc.isFallbackReq {
				extra["fallback"] = "true"
			}
			stream := &mockProcessServer{
				ctx: context.Background(),
				requests: []*extprocv3.ProcessingRequest{
					makeRequestHeaders(tc.path, tc.model, extra),
					makeResponseHeaders(tc.upstreamStatus),
				},
			}

			err := server.Process(stream)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(stream.responses) != 2 {
				t.Fatalf("expected 2 responses, got %d", len(stream.responses))
			}

			respHeaders := stream.responses[1]
			status := getSetHeader(respHeaders, ":status")
			location := getSetHeader(respHeaders, "location")

			if status != tc.expectedStatus {
				t.Errorf("expected status %q, got %q", tc.expectedStatus, status)
			}
			if location != tc.expectedLocation {
				t.Errorf("expected location %q, got %q", tc.expectedLocation, location)
			}
		})
	}
}

func TestProcess_FallbackModelOverride(t *testing.T) {
	server := NewServer()
	// Fallback to messages (Anthropic) but client has model gpt-4o.
	stream := &mockProcessServer{
		ctx: context.Background(),
		requests: []*extprocv3.ProcessingRequest{
			makeRequestHeaders("/v1/messages?fallback=true", "gpt-4o", nil),
		},
	}

	err := server.Process(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	provider := getSetHeader(stream.responses[0], "x-llm-provider")
	model := getSetHeader(stream.responses[0], "x-model")

	if provider != "anthropic" {
		t.Errorf("expected fallback provider to be anthropic, got %s", provider)
	}
	if model != "claude-3-5-sonnet" {
		t.Errorf("expected model override to claude-3-5-sonnet, got %s", model)
	}
}
