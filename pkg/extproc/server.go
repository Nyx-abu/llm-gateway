package extproc

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"llm-gateway/pkg/extproc/dashboard"
)

// Server implements the Envoy ExternalProcessorServer interface.
type Server struct {
	extprocv3.UnimplementedExternalProcessorServer

	// virtualKeyVault maps client virtual keys to metadata
	virtualKeyVault map[string]*VirtualKey

	logger *slog.Logger
}

type VirtualKey struct {
	mu             sync.RWMutex
	ID             string
	AllowedModels  []string
	MonthlyBudget  float64
	Spend          float64
	// In a real system, provider keys would be stored securely per virtual key or tenant
}

func (vk *VirtualKey) AddSpend(amount float64) {
	vk.mu.Lock()
	defer vk.mu.Unlock()
	vk.Spend += amount
}

type ModelPricing struct {
	InputCostPerMillion  float64
	OutputCostPerMillion float64
}

var pricingMap = map[string]ModelPricing{
	"gpt-4o":                      {InputCostPerMillion: 5.0, OutputCostPerMillion: 15.0},
	"gpt-4-turbo":                 {InputCostPerMillion: 10.0, OutputCostPerMillion: 30.0},
	"gpt-4":                       {InputCostPerMillion: 30.0, OutputCostPerMillion: 60.0},
	"gpt-3.5-turbo":               {InputCostPerMillion: 0.5, OutputCostPerMillion: 1.5},
	"o1-mini":                     {InputCostPerMillion: 3.0, OutputCostPerMillion: 12.0},
	"o1-preview":                  {InputCostPerMillion: 15.0, OutputCostPerMillion: 60.0},
	"claude-3-5-sonnet":           {InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0},
	"claude-3-5-sonnet-20240620":  {InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0},
	"claude-3-opus":               {InputCostPerMillion: 15.0, OutputCostPerMillion: 75.0},
	"claude-3-opus-20240229":      {InputCostPerMillion: 15.0, OutputCostPerMillion: 75.0},
	"claude-3-haiku":              {InputCostPerMillion: 0.25, OutputCostPerMillion: 1.25},
	"claude-3-haiku-20240307":     {InputCostPerMillion: 0.25, OutputCostPerMillion: 1.25},
	"claude-2.1":                  {InputCostPerMillion: 8.0, OutputCostPerMillion: 24.0},
	"claude-2.0":                  {InputCostPerMillion: 8.0, OutputCostPerMillion: 24.0},
}

type OpenAIResponseUsage struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type AnthropicResponseUsage struct {
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}



// NewServer creates a new ext_proc server.
// It reads configuration from environment variables:
//   - GATEWAY_API_KEYS: comma-separated allow-list of client bearer tokens
//   - OPENAI_API_KEY: real API key injected into OpenAI upstream requests
//   - ANTHROPIC_API_KEY: real API key injected into Anthropic upstream requests
func NewServer() *Server {
	ringBuffer := dashboard.NewLogRingBuffer(100)
	multiWriter := io.MultiWriter(os.Stdout, ringBuffer)
	logger := slog.New(slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	s := &Server{
		virtualKeyVault: make(map[string]*VirtualKey),
		logger:          logger,
	}

	// Load gateway API keys from environment and map to virtual vault
	if keys := os.Getenv("GATEWAY_API_KEYS"); keys != "" {
		for _, k := range strings.Split(keys, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				s.virtualKeyVault[k] = &VirtualKey{
					ID:            k,
					AllowedModels: []string{"*"},
					MonthlyBudget: 100.0,
					Spend:         0.0,
				}
			}
		}
		logger.Info("gateway auth enabled with virtual key vault", "keys_loaded", len(s.virtualKeyVault))
	} else {
		logger.Warn("gateway auth disabled: GATEWAY_API_KEYS is empty")
	}

	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		logger.Info("openai API key loaded into vault")
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		logger.Info("anthropic API key loaded into vault")
	}

	dashboard.StartDashboardServer(ringBuffer, len(s.virtualKeyVault))

	return s
}

// Process handles streaming request/response phases from Envoy.
func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	var provider string
	var isFallback bool
	var requestID string
	var startTime time.Time
	var vk *VirtualKey
	var model string
	var originalPath string

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
			startTime = time.Now()
			provider = ""
			isFallback = false
			atomic.AddUint64(&dashboard.TotalRequests, 1)
			headers := msg.RequestHeaders.Headers.Headers

			// Extract or generate request ID for tracing
			requestID = getHeader(headers, "x-request-id")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			path := getHeader(headers, ":path")
			originalPath = path
			model = getHeader(headers, "x-model")
			clientAuth := getHeader(headers, "authorization")

			// Log all headers for debugging
			var headerKeys []string
			for _, h := range headers {
				if h != nil {
					headerKeys = append(headerKeys, h.Key+"="+h.Value+"|"+string(h.RawValue))
				}
			}
			s.logger.Info("received headers", "request_id", requestID, "headers", strings.Join(headerKeys, ", "))



			// ── Auth & Virtual Key check ─────────────────────────
			if len(s.virtualKeyVault) > 0 {
				token := strings.TrimPrefix(clientAuth, "Bearer ")
				token = strings.TrimPrefix(token, "bearer ")
				vk = s.virtualKeyVault[token]
				if token == "" || vk == nil {
					s.logger.Warn("auth rejected",
						"request_id", requestID,
						"reason", "invalid_virtual_key",
					)
					resp.Response = &extprocv3.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &extprocv3.ImmediateResponse{
							Status: &typev3.HttpStatus{Code: typev3.StatusCode_Unauthorized},
							Headers: &extprocv3.HeaderMutation{
								SetHeaders: []*corev3.HeaderValueOption{
									{Header: &corev3.HeaderValue{
										Key:      "content-type",
										RawValue: []byte("application/json"),
									}},
								},
							},
							Body: `{"error":{"message":"Invalid or missing Gateway API key","type":"authentication_error","code":"invalid_api_key"}}`,
						},
					}
					if err := stream.Send(resp); err != nil {
						return err
					}
					continue
				}
				
				// Budget check
				vk.mu.RLock()
				budgetExceeded := vk.Spend >= vk.MonthlyBudget
				vk.mu.RUnlock()
				if budgetExceeded {
					s.logger.Warn("budget exceeded", "request_id", requestID, "virtual_key", vk.ID)
					resp.Response = &extprocv3.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &extprocv3.ImmediateResponse{
							Status: &typev3.HttpStatus{Code: typev3.StatusCode_TooManyRequests},
							Headers: &extprocv3.HeaderMutation{
								SetHeaders: []*corev3.HeaderValueOption{
									{Header: &corev3.HeaderValue{Key: "content-type", RawValue: []byte("application/json")}},
								},
							},
							Body: `{"error":{"message":"Virtual key budget exceeded","type":"billing_error"}}`,
						},
					}
					if err := stream.Send(resp); err != nil {
						return err
					}
					continue
				}
			}

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

			s.logger.Info("request routed",
				"request_id", requestID,
				"model", model,
				"provider", provider,
				"fallback", isFallback,
				"path", path,
			)

			// Generate routing and tracing headers
			var setHeaders []*corev3.HeaderValueOption
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:      "x-llm-provider",
					RawValue: []byte(provider),
				},
			})

			// Add request ID header for end-to-end tracing
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:      "x-request-id",
					RawValue: []byte(requestID),
				},
			})

			// Inject real provider API key from central vault
			var providerKey string
			if provider == "openai" {
				providerKey = os.Getenv("OPENAI_API_KEY")
			} else if provider == "anthropic" {
				providerKey = os.Getenv("ANTHROPIC_API_KEY")
			}
			
			if providerKey != "" {
				s.logger.Info("injecting provider key", "provider", provider, "key_prefix", providerKey[:min(10, len(providerKey))])
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{
						Key:      "authorization",
						RawValue: []byte("Bearer " + providerKey),
					},
				})
			} else {
				s.logger.Warn("no provider key found to inject", "provider", provider)
			}

			// Override x-model header to fallback-friendly default if mismatched
			modelLower := strings.ToLower(model)
			if provider == "anthropic" && !strings.HasPrefix(modelLower, "claude") {
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{
						Key:      "x-model",
						RawValue: []byte("claude-3-5-sonnet"),
					},
				})
			} else if provider == "openai" && !strings.HasPrefix(modelLower, "gpt") && !strings.HasPrefix(modelLower, "o1") {
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{
						Key:      "x-model",
						RawValue: []byte("gpt-4o"),
					},
				})
			}

			resp.Response = &extprocv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status:         extprocv3.CommonResponse_CONTINUE,
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

			latencyMs := time.Since(startTime).Milliseconds()

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
					s.logger.Warn("upstream failure, triggering fallback",
						"request_id", requestID,
						"provider", provider,
						"status_code", statusCode,
						"latency_ms", latencyMs,
					)

					setHeaders = append(setHeaders, &corev3.HeaderValueOption{
						Header: &corev3.HeaderValue{
							Key:      ":status",
							RawValue: []byte("307"),
						},
					}, &corev3.HeaderValueOption{
						Header: &corev3.HeaderValue{
							Key:      "location",
							RawValue: []byte(redirectURL),
						},
					})
				}
			} else {
				s.logger.Info("response completed",
					"request_id", requestID,
					"provider", provider,
					"status_code", statusCode,
					"fallback", isFallback,
					"latency_ms", latencyMs,
				)
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
			// Default to no mutation
			resp.Response = &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			}

			// Translate OpenAI to Anthropic: client requested OpenAI but routed to Anthropic
			if provider == "anthropic" && strings.Contains(originalPath, "/v1/chat/completions") {
				translated, err := TranslateOpenAIToAnthropic(msg.RequestBody.Body)
				if err == nil && translated != nil {
					s.logger.Info("translated request body from OpenAI to Anthropic", "request_id", requestID)
					resp.Response = &extprocv3.ProcessingResponse_RequestBody{
						RequestBody: &extprocv3.BodyResponse{
							Response: &extprocv3.CommonResponse{
								Status: extprocv3.CommonResponse_CONTINUE_AND_REPLACE,
								BodyMutation: &extprocv3.BodyMutation{
									Mutation: &extprocv3.BodyMutation_Body{
										Body: translated,
									},
								},
							},
						},
					}
				}
			}

			// Translate Anthropic to OpenAI: client requested Anthropic but routed to OpenAI
			if provider == "openai" && strings.Contains(originalPath, "/v1/messages") {
				translated, err := TranslateAnthropicToOpenAI(msg.RequestBody.Body)
				if err == nil && translated != nil {
					s.logger.Info("translated request body from Anthropic to OpenAI", "request_id", requestID)
					resp.Response = &extprocv3.ProcessingResponse_RequestBody{
						RequestBody: &extprocv3.BodyResponse{
							Response: &extprocv3.CommonResponse{
								Status: extprocv3.CommonResponse_CONTINUE_AND_REPLACE,
								BodyMutation: &extprocv3.BodyMutation{
									Mutation: &extprocv3.BodyMutation_Body{
										Body: translated,
									},
								},
							},
						},
					}
				}
			}

		case *extprocv3.ProcessingRequest_ResponseBody:
			resp.Response = &extprocv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			}

			if vk != nil && len(msg.ResponseBody.Body) > 0 {
				var promptTokens, completionTokens int
				
				// Try parsing as OpenAI response first
				var oi OpenAIResponseUsage
				if err := json.Unmarshal(msg.ResponseBody.Body, &oi); err == nil && oi.Usage.TotalTokens > 0 {
					promptTokens = oi.Usage.PromptTokens
					completionTokens = oi.Usage.CompletionTokens
				} else {
					// Try parsing as Anthropic response
					var anth AnthropicResponseUsage
					if err := json.Unmarshal(msg.ResponseBody.Body, &anth); err == nil && (anth.Usage.InputTokens > 0 || anth.Usage.OutputTokens > 0) {
						promptTokens = anth.Usage.InputTokens
						completionTokens = anth.Usage.OutputTokens
					}
				}

				if promptTokens > 0 || completionTokens > 0 {
					// Calculate cost
					pricing, exists := pricingMap[strings.ToLower(model)]
					if !exists {
						// Safe default fallback pricing ($10/1M prompt, $30/1M completion)
						pricing = ModelPricing{InputCostPerMillion: 10.0, OutputCostPerMillion: 30.0}
					}
					
					cost := (float64(promptTokens) * pricing.InputCostPerMillion / 1000000.0) +
						(float64(completionTokens) * pricing.OutputCostPerMillion / 1000000.0)
					
					vk.AddSpend(cost)
					vk.mu.RLock()
					currentSpend := vk.Spend
					vk.mu.RUnlock()
					
					s.logger.Info("updated virtual key spend",
						"request_id", requestID,
						"virtual_key", vk.ID,
						"prompt_tokens", promptTokens,
						"completion_tokens", completionTokens,
						"added_cost", cost,
						"total_spend", currentSpend,
					)
				}
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
			if h.Value != "" {
				return h.Value
			}
			if len(h.RawValue) > 0 {
				return string(h.RawValue)
			}
		}
	}
	return ""
}
