package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const defaultTimeout = 10 * time.Second

// Config configures a Client. Empty string values fall back to environment
// variables and then SDK defaults.
type Config struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	Retry        *RetryPolicy
	Headers      http.Header
	HTTPClient   *http.Client
	Logger       *slog.Logger
}

// RequestOptions overrides client settings for one API call.
type RequestOptions struct {
	Timeout time.Duration
	Retry   *RetryPolicy
	Headers http.Header
}

// SystemOneRequest evaluates state against named typed questions.
type SystemOneRequest struct {
	State     JSONValue
	Questions map[string]Question
	Model     string
	ExtraBody map[string]JSONValue
}

// Client is a concurrency-safe TypeSafe API client.
type Client struct {
	apiKey       string
	baseURL      string
	defaultModel string
	timeout      time.Duration
	retry        RetryPolicy
	headers      http.Header
	httpClient   *http.Client
	logger       *slog.Logger

	Models *ModelsService
}

// NewClient constructs a client. Pass no Config to resolve all settings from
// the environment and SDK defaults.
func NewClient(configs ...Config) (*Client, error) {
	if len(configs) > 1 {
		return nil, fmt.Errorf("NewClient accepts at most one Config: %w", ErrInvalidRequest)
	}
	var config Config
	if len(configs) == 1 {
		config = configs[0]
	}
	apiKey := firstNonBlank(config.APIKey, os.Getenv("TYPESAFE_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided; set Config.APIKey or TYPESAFE_API_KEY: %w", ErrInvalidRequest)
	}
	if err := validateAPIKey(apiKey); err != nil {
		return nil, err
	}
	baseURL := firstNonBlank(config.BaseURL, os.Getenv("TYPESAFE_BASE_URL"), DefaultBaseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid TypeSafe base URL %q: %w", baseURL, ErrInvalidRequest)
	}
	model := firstNonBlank(config.DefaultModel, os.Getenv("TYPESAFE_DEFAULT_MODEL"), DefaultModel)
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout must be positive: %w", ErrInvalidRequest)
	}
	retry := DefaultRetryPolicy()
	if config.Retry != nil {
		retry = cloneRetryPolicy(*config.Retry)
	}
	if err := retry.validate(); err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	client := &Client{
		apiKey:       apiKey,
		baseURL:      baseURL,
		defaultModel: model,
		timeout:      timeout,
		retry:        retry,
		headers:      config.Headers.Clone(),
		httpClient:   httpClient,
		logger:       logger,
	}
	client.Models = &ModelsService{client: client}
	return client, nil
}

// BaseURL returns the configured API root.
func (c *Client) BaseURL() string { return c.baseURL }

// DefaultModel returns the model used when a request omits Model.
func (c *Client) DefaultModel() string { return c.defaultModel }

// SystemOne answers named questions about text or structured state.
func (c *Client) SystemOne(ctx context.Context, request SystemOneRequest, options ...RequestOptions) (*SystemOneResponse, error) {
	data, meta, err := c.systemOneRaw(ctx, request, options...)
	if err != nil {
		return nil, err
	}
	response, err := decodeSystemOne(data)
	if err != nil {
		return nil, &ResponseValidationError{Cause: err, Meta: meta, Body: append([]byte(nil), data...)}
	}
	response.Meta = meta
	return response, nil
}

// systemOneRaw validates the request, builds the body, and performs the HTTP
// call shared by SystemOne and SystemOneAs. It returns the raw response body so
// callers can decode it into either the built-in SystemOneResponse or a
// caller-supplied type.
func (c *Client) systemOneRaw(ctx context.Context, request SystemOneRequest, options ...RequestOptions) ([]byte, ResponseMeta, error) {
	if err := validateQuestions(request.Questions); err != nil {
		return nil, ResponseMeta{}, err
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = c.defaultModel
	}
	body := make(map[string]JSONValue, len(request.ExtraBody)+3)
	for key, value := range request.ExtraBody {
		body[key] = value
	}
	body["state"] = request.State
	body["questions"] = request.Questions
	body["model"] = model

	return c.request(ctx, http.MethodPost, "/v1/systemone", body, oneRequestOptions(options))
}

func (c *Client) request(ctx context.Context, method, path string, body JSONValue, options RequestOptions) ([]byte, ResponseMeta, error) {
	if ctx == nil {
		return nil, ResponseMeta{}, fmt.Errorf("context is nil: %w", ErrInvalidRequest)
	}
	timeout := c.timeout
	if options.Timeout != 0 {
		timeout = options.Timeout
	}
	if timeout < 0 {
		return nil, ResponseMeta{}, fmt.Errorf("timeout must be positive: %w", ErrInvalidRequest)
	}
	retry := cloneRetryPolicy(c.retry)
	if options.Retry != nil {
		retry = cloneRetryPolicy(*options.Retry)
	}
	if err := retry.validate(); err != nil {
		return nil, ResponseMeta{}, err
	}

	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, ResponseMeta{}, fmt.Errorf("encode request body: %w", err)
		}
	}
	headers := mergeHeaders(c.headers, options.Headers)
	headers.Set("Authorization", "Bearer "+c.apiKey)
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "typesafe-sdk/"+Version)
	headers.Set("X-TypeSafe-SDK", "typesafe-sdk/"+Version)
	headers.Set("X-TypeSafe-Runtime", fmt.Sprintf("go/%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))
	headers.Del("X-TypeSafe-Retry-Count")
	if body != nil {
		headers.Set("Content-Type", "application/json")
	}

	for attempt := 0; ; attempt++ {
		attemptHeaders := headers.Clone()
		if attempt > 0 {
			attemptHeaders.Set("X-TypeSafe-Retry-Count", strconv.Itoa(attempt))
		}
		data, meta, requestErr := c.attempt(ctx, timeout, method, c.baseURL+path, encoded, attemptHeaders)
		if requestErr == nil {
			return data, meta, nil
		}
		if attempt >= retry.MaxRetries || !retryable(requestErr, retry) {
			return nil, meta, requestErr
		}
		delay := retry.delay(attempt, meta.Headers)
		c.logger.Info("retrying TypeSafe request", "method", method, "path", path, "retry", attempt+1, "delay", delay, "error", requestErr)
		if err := sleepContext(ctx, delay); err != nil {
			return nil, meta, &APIUserAbortError{Cause: err}
		}
	}
}

func (c *Client) attempt(parent context.Context, timeout time.Duration, method, target string, body []byte, headers http.Header) ([]byte, ResponseMeta, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("build HTTP request: %w", err)
	}
	request.Header = headers
	started := time.Now()
	response, err := c.httpClient.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return nil, ResponseMeta{}, &APIUserAbortError{Cause: parent.Err()}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ResponseMeta{}, &APITimeoutError{Timeout: timeout, Cause: err}
		}
		return nil, ResponseMeta{}, &APIConnectionError{Cause: err}
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	meta := ResponseMeta{
		StatusCode: response.StatusCode,
		Headers:    response.Header.Clone(),
		RequestID:  response.Header.Get("x-typesafe-request-id"),
	}
	c.logger.Info("TypeSafe request completed", "method", method, "url", target, "status", response.StatusCode, "duration", time.Since(started), "request_id", meta.RequestID)
	if readErr != nil {
		return nil, meta, &APIConnectionError{Cause: fmt.Errorf("read response body: %w", readErr)}
	}
	if closeErr != nil {
		return nil, meta, &APIConnectionError{Cause: fmt.Errorf("close response body: %w", closeErr)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, meta, apiError(response.StatusCode, data, response.Header)
	}
	return data, meta, nil
}

// ResponseValidationError reports a successful response with an invalid body.
type ResponseValidationError struct {
	Cause error
	Meta  ResponseMeta
	Body  []byte
}

func (e *ResponseValidationError) Error() string {
	return "invalid TypeSafe response: " + e.Cause.Error()
}

func (e *ResponseValidationError) Unwrap() error { return e.Cause }

func retryable(err error, policy RetryPolicy) bool {
	var timeout *APITimeoutError
	if errors.As(err, &timeout) {
		return policy.APITimeoutError
	}
	var connection *APIConnectionError
	if errors.As(err, &connection) {
		return policy.APIConnectionError
	}
	var api *APIError
	if errors.As(err, &api) {
		_, ok := policy.HTTPStatuses[api.StatusCode]
		return ok
	}
	return false
}

// validateAPIKey rejects keys that are not printable ASCII or that contain
// whitespace. Such a value cannot form a valid Authorization header and would
// otherwise fail deep in the HTTP stack with a less actionable error; rejecting
// it here keeps a malformed credential out of the request entirely.
func validateAPIKey(key string) error {
	for _, r := range key {
		if r > unicode.MaxASCII || r == ' ' || !unicode.IsPrint(r) {
			return fmt.Errorf("API key must contain only printable ASCII characters without whitespace: %w", ErrInvalidRequest)
		}
	}
	return nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeHeaders(sources ...http.Header) http.Header {
	merged := make(http.Header)
	for _, source := range sources {
		for name, values := range source {
			canonical := http.CanonicalHeaderKey(name)
			merged.Del(canonical)
			for _, value := range values {
				merged.Add(canonical, value)
			}
		}
	}
	return merged
}

func cloneRetryPolicy(policy RetryPolicy) RetryPolicy {
	statuses := make(map[int]struct{}, len(policy.HTTPStatuses))
	for status := range policy.HTTPStatuses {
		statuses[status] = struct{}{}
	}
	policy.HTTPStatuses = statuses
	return policy
}

func oneRequestOptions(options []RequestOptions) RequestOptions {
	var merged RequestOptions
	for _, option := range options {
		if option.Timeout != 0 {
			merged.Timeout = option.Timeout
		}
		if option.Retry != nil {
			merged.Retry = option.Retry
		}
		merged.Headers = mergeHeaders(merged.Headers, option.Headers)
	}
	return merged
}
