package extproc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"llm-gateway/pkg/extproc/dashboard"
)

type VirtualKey struct {
	mu             sync.RWMutex
	ID             string   `json:"id"`
	AllowedModels  []string `json:"allowed_models"`
	MonthlyBudget  float64  `json:"monthly_budget"`
	Spend          float64  `json:"spend"`
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

type Server struct {
	extprocv3.UnimplementedExternalProcessorServer
	virtualKeyVault map[string]*VirtualKey
	logger          *slog.Logger
	rdb             *redis.Client
}

var ssnRegex = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
var ccRegex = regexp.MustCompile(`\b(?:\d[ -]*?){13,16}\b`)

func NewServer() *Server {
	ringBuffer := dashboard.NewLogRingBuffer(100)
	multiWriter := io.MultiWriter(os.Stdout, ringBuffer)
	logger := slog.New(slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	var rdb *redis.Client
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr: redisAddr,
		})
	}

	s := &Server{
		virtualKeyVault: make(map[string]*VirtualKey),
		logger:          logger,
		rdb:             rdb,
	}

	keysEnv := os.Getenv("GATEWAY_API_KEYS")
	if strings.HasPrefix(strings.TrimSpace(keysEnv), "[") {
		var keys []VirtualKey
		if err := json.Unmarshal([]byte(keysEnv), &keys); err == nil {
			for i := range keys {
				s.virtualKeyVault[keys[i].ID] = &keys[i]
			}
		}
	} else if keysEnv != "" {
		for _, k := range strings.Split(keysEnv, ",") {
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
	}
	
	logger.Info("gateway auth initialized", "keys_loaded", len(s.virtualKeyVault))

	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		logger.Info("openai API key loaded into vault")
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		logger.Info("anthropic API key loaded into vault")
	}

	dashboard.StartDashboardServer(ringBuffer, len(s.virtualKeyVault))

	return s
}

func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	var provider string
	var isFallback bool
	var requestID string
	var vk *VirtualKey
	var model string
	var originalPath string
	var cacheKey string
	ctx := context.Background()

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
			atomic.AddUint64(&dashboard.TotalRequests, 1)
			headers := msg.RequestHeaders.Headers.Headers

			requestID = getHeader(headers, "x-request-id")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			path := getHeader(headers, ":path")
			originalPath = path
			model = getHeader(headers, "x-model")
			clientAuth := getHeader(headers, "authorization")

			// Secure logging: Do not dump raw headers
			s.logger.Info("request started", "request_id", requestID, "path", path, "model", model)

			// ── Auth & RBAC check ─────────────────────────
			if len(s.virtualKeyVault) > 0 {
				token := strings.TrimPrefix(clientAuth, "Bearer ")
				token = strings.TrimPrefix(token, "bearer ")
				vk = s.virtualKeyVault[token]
				
				if vk == nil {
					resp.Response = buildImmediateError(typev3.StatusCode_Unauthorized, "Invalid API key")
					if stream.Send(resp) != nil { return nil }
					continue
				}
				
				// Model RBAC Check
				allowed := false
				for _, m := range vk.AllowedModels {
					if m == "*" || m == model {
						allowed = true
						break
					}
				}
				if !allowed {
					resp.Response = buildImmediateError(typev3.StatusCode_Forbidden, "Model not allowed for this key")
					if stream.Send(resp) != nil { return nil }
					continue
				}

				// Global Rate Limiting via Redis
				if s.rdb != nil {
					rateKey := "rate:" + vk.ID
					script := `
						local c = redis.call('INCR', KEYS[1])
						if c == 1 then
							redis.call('EXPIRE', KEYS[1], 60)
						end
						return c
					`
					count, err := s.rdb.Eval(ctx, script, []string{rateKey}).Int()
					if err == nil {
						if count > 100 { // 100 req/min
							resp.Response = buildImmediateError(typev3.StatusCode_TooManyRequests, "Rate limit exceeded")
							if stream.Send(resp) != nil { return nil }
							continue
						}
					}
				}
				
				vk.mu.RLock()
				budgetExceeded := vk.Spend >= vk.MonthlyBudget
				vk.mu.RUnlock()
				if budgetExceeded {
					resp.Response = buildImmediateError(typev3.StatusCode_TooManyRequests, "Budget exceeded")
					if stream.Send(resp) != nil { return nil }
					continue
				}
			}

			isFallback = strings.Contains(path, "fallback=true") || getHeader(headers, "fallback") == "true"
			if isFallback {
				if strings.Contains(path, "/v1/messages") { provider = "anthropic" } else { provider = "openai" }
			}

			if provider == "" {
				modelLower := strings.ToLower(model)
				if strings.HasPrefix(modelLower, "gpt") || strings.HasPrefix(modelLower, "o1") {
					provider = "openai"
				} else if strings.HasPrefix(modelLower, "claude") {
					provider = "anthropic"
				} else {
					provider = "openai"
				}
			}

			var setHeaders []*corev3.HeaderValueOption
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{Key: "x-llm-provider", RawValue: []byte(provider)},
			})
			setHeaders = append(setHeaders, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{Key: "x-request-id", RawValue: []byte(requestID)},
			})

			providerKey := ""
			if provider == "openai" { providerKey = os.Getenv("OPENAI_API_KEY") } else if provider == "anthropic" { providerKey = os.Getenv("ANTHROPIC_API_KEY") }
			if providerKey != "" {
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{Key: "authorization", RawValue: []byte("Bearer " + providerKey)},
				})
			}

			modelLower := strings.ToLower(model)
			if provider == "anthropic" && !strings.HasPrefix(modelLower, "claude") {
				model = "claude-3-5-sonnet"
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{Key: "x-model", RawValue: []byte("claude-3-5-sonnet")},
				})
			} else if provider == "openai" && !strings.HasPrefix(modelLower, "gpt") && !strings.HasPrefix(modelLower, "o1") {
				model = "gpt-4o"
				setHeaders = append(setHeaders, &corev3.HeaderValueOption{
					Header: &corev3.HeaderValue{Key: "x-model", RawValue: []byte("gpt-4o")},
				})
			}

			resp.Response = &extprocv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status:         extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{SetHeaders: setHeaders},
						ClearRouteCache: true,
					},
				},
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			body := msg.RequestBody.Body
			
			// Semantic Caching Check
			if s.rdb != nil {
				hash := fmt.Sprintf("%x", sha256.Sum256(body))
				cacheKey = "cache:" + provider + ":" + model + ":" + hash
				cachedResp, err := s.rdb.Get(ctx, cacheKey).Result()
				if err == nil && cachedResp != "" {
					s.logger.Info("cache hit", "request_id", requestID, "hash", hash)
					resp.Response = &extprocv3.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &extprocv3.ImmediateResponse{
							Status: &typev3.HttpStatus{Code: typev3.StatusCode_OK},
							Headers: &extprocv3.HeaderMutation{
								SetHeaders: []*corev3.HeaderValueOption{
									{Header: &corev3.HeaderValue{Key: "content-type", RawValue: []byte("application/json")}},
									{Header: &corev3.HeaderValue{Key: "x-cache", RawValue: []byte("HIT")}},
								},
							},
							Body: cachedResp,
						},
					}
					if err := stream.Send(resp); err != nil { return err }
					continue
				}
			}

			// DLP Scrubbing
			bodyStr := string(body)
			if ssnRegex.MatchString(bodyStr) || ccRegex.MatchString(bodyStr) {
				bodyStr = ssnRegex.ReplaceAllString(bodyStr, "[REDACTED SSN]")
				bodyStr = ccRegex.ReplaceAllString(bodyStr, "[REDACTED CC]")
				s.logger.Info("DLP redaction applied", "request_id", requestID)
				body = []byte(bodyStr)
			}

			// Translate
			if provider == "anthropic" && strings.Contains(originalPath, "/v1/chat/completions") {
				translated, err := TranslateOpenAIToAnthropic(body)
				if err == nil && translated != nil { body = translated }
			}
			if provider == "openai" && strings.Contains(originalPath, "/v1/messages") {
				translated, err := TranslateAnthropicToOpenAI(body)
				if err == nil && translated != nil { body = translated }
			}

			resp.Response = &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE_AND_REPLACE,
						BodyMutation: &extprocv3.BodyMutation{
							Mutation: &extprocv3.BodyMutation_Body{Body: body},
						},
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseHeaders:
			headers := msg.ResponseHeaders.Headers.Headers
			statusCode, _ := strconv.Atoi(getHeader(headers, ":status"))
			
			if statusCode >= 500 && statusCode <= 599 && !isFallback {
				redirectURL := ""
				if provider == "openai" { redirectURL = "/v1/messages?fallback=true" }
				if provider == "anthropic" { redirectURL = "/v1/chat/completions?fallback=true" }
				if redirectURL != "" {
					resp.Response = &extprocv3.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &extprocv3.ImmediateResponse{
							Status: &typev3.HttpStatus{Code: typev3.StatusCode_TemporaryRedirect},
							Headers: &extprocv3.HeaderMutation{
								SetHeaders: []*corev3.HeaderValueOption{
									{Header: &corev3.HeaderValue{Key: "location", RawValue: []byte(redirectURL)}},
								},
							},
							Body: "",
						},
					}
					if err := stream.Send(resp); err != nil { return err }
					continue
				}
			}

			resp.Response = &extprocv3.ProcessingResponse_ResponseHeaders{
				ResponseHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseBody:
			resp.Response = &extprocv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{Status: extprocv3.CommonResponse_CONTINUE},
				},
			}

			if vk != nil && len(msg.ResponseBody.Body) > 0 {
				if cacheKey != "" && s.rdb != nil {
					s.rdb.Set(ctx, cacheKey, msg.ResponseBody.Body, time.Hour)
				}
				
				var promptTokens, completionTokens int
				var oi OpenAIResponseUsage
				if err := json.Unmarshal(msg.ResponseBody.Body, &oi); err == nil && oi.Usage.TotalTokens > 0 {
					promptTokens = oi.Usage.PromptTokens
					completionTokens = oi.Usage.CompletionTokens
				} else {
					var anth AnthropicResponseUsage
					if err := json.Unmarshal(msg.ResponseBody.Body, &anth); err == nil && (anth.Usage.InputTokens > 0 || anth.Usage.OutputTokens > 0) {
						promptTokens = anth.Usage.InputTokens
						completionTokens = anth.Usage.OutputTokens
					} else {
						// Fallback: Check for SSE stream data
						lines := strings.Split(string(msg.ResponseBody.Body), "\n")
						for _, line := range lines {
							line = strings.TrimSpace(line)
							if strings.HasPrefix(line, "data: ") {
								dataJSON := strings.TrimPrefix(line, "data: ")
								if dataJSON == "[DONE]" { continue }
								var sseMap map[string]interface{}
								if json.Unmarshal([]byte(dataJSON), &sseMap) == nil {
									if usage, ok := sseMap["usage"].(map[string]interface{}); ok {
										if pt, ok := usage["prompt_tokens"].(float64); ok { promptTokens = int(pt) }
										if ct, ok := usage["completion_tokens"].(float64); ok { completionTokens = int(ct) }
									}
								}
							}
						}
					}
				}

				if promptTokens > 0 || completionTokens > 0 {
					pricing, exists := pricingMap[strings.ToLower(model)]
					if !exists { pricing = ModelPricing{InputCostPerMillion: 10.0, OutputCostPerMillion: 30.0} }
					cost := (float64(promptTokens) * pricing.InputCostPerMillion / 1000000.0) +
						(float64(completionTokens) * pricing.OutputCostPerMillion / 1000000.0)
					vk.AddSpend(cost)
				}
			}

		case *extprocv3.ProcessingRequest_RequestTrailers:
			resp.Response = &extprocv3.ProcessingResponse_RequestTrailers{RequestTrailers: &extprocv3.TrailersResponse{}}
		case *extprocv3.ProcessingRequest_ResponseTrailers:
			resp.Response = &extprocv3.ProcessingResponse_ResponseTrailers{ResponseTrailers: &extprocv3.TrailersResponse{}}
		}

		if err := stream.Send(resp); err != nil { return err }
	}
}

func getHeader(headers []*corev3.HeaderValue, name string) string {
	for _, h := range headers {
		if h != nil && strings.ToLower(h.Key) == strings.ToLower(name) {
			if h.Value != "" { return h.Value }
			if len(h.RawValue) > 0 { return string(h.RawValue) }
		}
	}
	return ""
}

func buildImmediateError(code typev3.StatusCode, msg string) *extprocv3.ProcessingResponse_ImmediateResponse {
	return &extprocv3.ProcessingResponse_ImmediateResponse{
		ImmediateResponse: &extprocv3.ImmediateResponse{
			Status: &typev3.HttpStatus{Code: code},
			Headers: &extprocv3.HeaderMutation{
				SetHeaders: []*corev3.HeaderValueOption{{Header: &corev3.HeaderValue{Key: "content-type", RawValue: []byte("application/json")}}},
			},
			Body: `{"error":{"message":"` + msg + `"}}`,
		},
	}
}
