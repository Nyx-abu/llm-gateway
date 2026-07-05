package extproc

import (
	"io"
	"log"
	"strconv"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// Server implements the Envoy ExternalProcessorServer interface.
type Server struct {
	extprocv3.UnimplementedExternalProcessorServer
}

// NewServer creates a new ext_proc server.
func NewServer() *Server {
	return &Server{}
}

// Process handles streaming request/response phases from Envoy.
func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	var provider string
	var isFallback bool

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		resp := &extprocv3.ProcessingResponse{}

		switch msg := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			provider = ""
			isFallback = false
			var headers []*corev3.HeaderValue
			if msg.RequestHeaders != nil && msg.RequestHeaders.Headers != nil {
				headers = msg.RequestHeaders.Headers.Headers
			}

			log.Printf("RequestHeaders received:")
			for _, h := range headers {
				if h != nil {
					log.Printf("  %s: %s", h.Key, h.Value)
				}
			}

			path := getHeader(headers, ":path")
			model := getHeader(headers, "x-model")

			// Check if this request is already a fallback (to avoid redirect loops)
			isFallback = strings.Contains(path, "fallback=true") ||
				getHeader(headers, "fallback") == "true" ||
				getHeader(headers, "x-fallback") == "true"

			// Determine LLM provider based on path and model
			if isFallback {
				if strings.Contains(path, "/v1/messages") {
					provider = "anthropic"
				} else if strings.Contains(path, "/v1/chat/completions") {
					provider = "openai"
				}
			}

			if provider == "" {
				modelLower := strings.ToLower(model)
				if strings.HasPrefix(modelLower, "gpt") || strings.HasPrefix(modelLower, "o1") {
					provider = "openai"
				} else if strings.HasPrefix(modelLower, "claude") {
					provider = "anthropic"
				} else {
					// Fallback default based on path
					if strings.Contains(path, "/v1/messages") {
						provider = "anthropic"
					} else {
						provider = "openai"
					}
				}
			}

			// Generate x-llm-provider routing header
			var setHeaders []*corev3.HeaderValueOption
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:   "x-llm-provider",
					Value: provider,
				},
			})

			// Override x-model header to fallback-friendly default if mismatched
			modelLower := strings.ToLower(model)
			if provider == "anthropic" && !strings.HasPrefix(modelLower, "claude") {
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{
						Key:   "x-model",
						Value: "claude-3-5-sonnet",
					},
				})
			} else if provider == "openai" && !strings.HasPrefix(modelLower, "gpt") && !strings.HasPrefix(modelLower, "o1") {
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{
						Key:   "x-model",
						Value: "gpt-4o",
					},
				})
			}

			resp.Response = &extprocv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status:          extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{
							SetHeaders: setHeaders,
						},
						ClearRouteCache: true,
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseHeaders:
			var headers []*corev3.HeaderValue
			if msg.ResponseHeaders != nil && msg.ResponseHeaders.Headers != nil {
				headers = msg.ResponseHeaders.Headers.Headers
			}

			statusStr := getHeader(headers, ":status")
			statusCode := 0
			if statusStr != "" {
				statusCode, _ = strconv.Atoi(statusStr)
			}

			var setHeaders []*corev3.HeaderValueOption

			// Intercept 5xx errors to redirect client to fallback provider
			if statusCode >= 500 && statusCode <= 599 && !isFallback {
				redirectURL := ""
				if provider == "openai" {
					redirectURL = "/v1/messages?fallback=true"
				} else if provider == "anthropic" {
					redirectURL = "/v1/chat/completions?fallback=true"
				}

				if redirectURL != "" {
					setHeaders = append(setHeaders, &corev3.HeaderValueOption{
						Header: &corev3.HeaderValue{
							Key:   ":status",
							Value: "307",
						},
					}, &corev3.HeaderValueOption{
						Header: &corev3.HeaderValue{
							Key:   "location",
							Value: redirectURL,
						},
					})
				}
			}

			resp.Response = &extprocv3.ProcessingResponse_ResponseHeaders{
				ResponseHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{
							SetHeaders: setHeaders,
						},
					},
				},
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			resp.Response = &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseBody:
			resp.Response = &extprocv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			}

		case *extprocv3.ProcessingRequest_RequestTrailers:
			resp.Response = &extprocv3.ProcessingResponse_RequestTrailers{
				RequestTrailers: &extprocv3.TrailersResponse{},
			}

		case *extprocv3.ProcessingRequest_ResponseTrailers:
			resp.Response = &extprocv3.ProcessingResponse_ResponseTrailers{
				ResponseTrailers: &extprocv3.TrailersResponse{},
			}
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// getHeader finds the value of a header key case-insensitively.
func getHeader(headers []*corev3.HeaderValue, name string) string {
	for _, h := range headers {
		if h != nil && strings.ToLower(h.Key) == strings.ToLower(name) {
			return h.Value
		}
	}
	return ""
}
