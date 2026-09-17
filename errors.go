package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalidRequest  = errors.New("invalid TypeSafe request")
	ErrInvalidResponse = errors.New("invalid TypeSafe response")
)

// APIError is an unsuccessful HTTP response from the TypeSafe API.
type APIError struct {
	StatusCode int
	Body       JSONValue
	Headers    http.Header
	RequestID  string
	Message    string
}

func (e *APIError) Error() string {
	message := fmt.Sprintf("%d %s", e.StatusCode, e.Message)
	if e.RequestID != "" {
		message += fmt.Sprintf(" (request_id=%s)", e.RequestID)
	}
	return message
}

type BadRequestError struct{ *APIError }
type AuthenticationError struct{ *APIError }
type PermissionDeniedError struct{ *APIError }
type NotFoundError struct{ *APIError }
type ConflictError struct{ *APIError }
type UnprocessableEntityError struct{ *APIError }
type InternalServerError struct{ *APIError }

func (e *BadRequestError) Unwrap() error          { return e.APIError }
func (e *AuthenticationError) Unwrap() error      { return e.APIError }
func (e *PermissionDeniedError) Unwrap() error    { return e.APIError }
func (e *NotFoundError) Unwrap() error            { return e.APIError }
func (e *ConflictError) Unwrap() error            { return e.APIError }
func (e *UnprocessableEntityError) Unwrap() error { return e.APIError }
func (e *InternalServerError) Unwrap() error      { return e.APIError }

type RateLimitError struct {
	*APIError
	RetryAfter time.Duration
}

func (e *RateLimitError) Unwrap() error { return e.APIError }

// APIConnectionError reports a failure before a complete HTTP response arrived.
type APIConnectionError struct{ Cause error }

func (e *APIConnectionError) Error() string {
	if e.Cause == nil {
		return "connection error"
	}
	return "connection error: " + e.Cause.Error()
}

func (e *APIConnectionError) Unwrap() error { return e.Cause }

// APITimeoutError reports that one HTTP attempt exceeded its timeout.
type APITimeoutError struct {
	Timeout time.Duration
	Cause   error
}

func (e *APITimeoutError) Error() string {
	return fmt.Sprintf("request timed out after %s", e.Timeout)
}

func (e *APITimeoutError) Unwrap() error { return e.Cause }

// APIUserAbortError reports cancellation through the caller's context.
type APIUserAbortError struct{ Cause error }

func (e *APIUserAbortError) Error() string { return "request was canceled by caller" }
func (e *APIUserAbortError) Unwrap() error { return e.Cause }

func apiError(status int, body []byte, headers http.Header) error {
	parsed := parseBody(body)
	base := &APIError{
		StatusCode: status,
		Body:       parsed,
		Headers:    headers.Clone(),
		RequestID:  headers.Get("x-typesafe-request-id"),
		Message:    errorMessage(parsed),
	}
	switch status {
	case http.StatusBadRequest:
		return &BadRequestError{base}
	case http.StatusUnauthorized:
		return &AuthenticationError{base}
	case http.StatusForbidden:
		return &PermissionDeniedError{base}
	case http.StatusNotFound:
		return &NotFoundError{base}
	case http.StatusConflict:
		return &ConflictError{base}
	case http.StatusUnprocessableEntity:
		return &UnprocessableEntityError{base}
	case http.StatusTooManyRequests:
		delay, _ := parseRetryAfter(headers, time.Now())
		return &RateLimitError{APIError: base, RetryAfter: delay}
	default:
		if status >= 500 {
			return &InternalServerError{base}
		}
		return base
	}
}

func parseBody(body []byte) JSONValue {
	if len(body) == 0 {
		return nil
	}
	var value JSONValue
	if json.Unmarshal(body, &value) == nil {
		return value
	}
	return string(body)
}

func errorMessage(body JSONValue) string {
	if body == nil {
		return "status code (no body)"
	}
	if text, ok := body.(string); ok && text != "" {
		return truncate(text, 200)
	}
	record, ok := body.(map[string]any)
	if ok {
		for _, key := range []string{"error", "message", "detail"} {
			if text := nestedMessage(record[key]); text != "" {
				return text
			}
		}
		if detail, ok := record["detail"].([]any); ok {
			if text := validationMessage(detail); text != "" {
				return text
			}
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "unknown error"
	}
	return truncate(string(raw), 200)
}

func nestedMessage(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if record, ok := value.(map[string]any); ok {
		if text, ok := record["message"].(string); ok {
			return text
		}
	}
	return ""
}

func validationMessage(entries []any) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		message, ok := record["msg"].(string)
		if !ok {
			continue
		}
		var path []string
		if location, ok := record["loc"].([]any); ok {
			for _, item := range location {
				part := fmt.Sprint(item)
				if part != "body" {
					path = append(path, part)
				}
			}
		}
		if len(path) > 0 {
			message = strings.Join(path, ".") + ": " + message
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}
