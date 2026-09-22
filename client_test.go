package typesafe_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	typesafe "github.com/valksor/typesafe-sdk-go"
)

func TestSystemOneRoundTrip(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		questions := body["questions"].(map[string]any)
		if questions["tone"].(map[string]any)["type"] != "score" {
			t.Errorf("score question was not serialized")
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("x-typesafe-request-id", "req_123")
		_, _ = response.Write([]byte(`{
            "model":"jev-latest",
            "answers":{
                "urgent":{"type":"noul","noul":0.9},
                "team":{"type":"choice","choice":"billing","confidence":0.8,"probabilities":{"billing":0.9,"other":0.1}},
                "tone":{"type":"score","score":1.5,"confidence":0.7,"legend":{"0":"calm","1":"tense","2":"angry"},"probabilities":{"0":0.1,"1":0.3,"2":0.6}},
                "future":{"type":"future","value":42}
            },
            "usage":{"input_tokens":10,"output_tokens":4}
        }`))
	}))
	defer server.Close()

	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State: map[string]any{"message": "Please help"},
		Questions: map[string]typesafe.Question{
			"urgent": typesafe.Noul("Is this urgent?"),
			"team":   typesafe.Choice("Which team?", map[string]any{"billing": nil, "other": nil}),
			"tone":   typesafe.Score("How tense?", "calm", "tense", "angry"),
		},
	})
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	if answer, ok := result.Noul("urgent"); !ok || answer.Noul != 0.9 {
		t.Fatalf("unexpected noul answer: %#v, %v", answer, ok)
	}
	if answer, ok := result.Choice("team"); !ok || answer.Choice != "billing" {
		t.Fatalf("unexpected choice answer: %#v, %v", answer, ok)
	}
	if answer, ok := result.Score("tone"); !ok || answer.Legend["2"] != "angry" {
		t.Fatalf("unexpected score answer: %#v, %v", answer, ok)
	}
	if _, ok := result.Answers["future"].(typesafe.UnknownAnswer); !ok {
		t.Fatalf("future answer = %T", result.Answers["future"])
	}
	if result.Meta.RequestID != "req_123" || result.Usage.InputTokens != 10 {
		t.Fatalf("unexpected metadata: %#v %#v", result.Meta, result.Usage)
	}
}

func TestRetryAndModels(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		attempt := calls.Add(1)
		if attempt == 1 {
			response.Header().Set("retry-after-ms", "0")
			http.Error(response, `{"message":"slow down"}`, http.StatusTooManyRequests)
			return
		}
		if request.Header.Get("X-TypeSafe-Retry-Count") != "1" {
			t.Errorf("retry count = %q", request.Header.Get("X-TypeSafe-Retry-Count"))
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"models":[{"name":"jev-latest","description":"Latest","release_date":"2026-09-15"}]}`))
	}))
	defer server.Close()

	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Models.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if calls.Load() != 2 || len(result.Models) != 1 || result.Models[0].Name != "jev-latest" {
		t.Fatalf("unexpected result after %d calls: %#v", calls.Load(), result)
	}
}

func TestAuthenticationError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("x-typesafe-request-id", "req_bad")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer server.Close()
	policy := typesafe.DefaultRetryPolicy()
	policy.MaxRetries = 0
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "bad", BaseURL: server.URL, Retry: &policy})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ListModels(context.Background())
	var authentication *typesafe.AuthenticationError
	if !errors.As(err, &authentication) {
		t.Fatalf("error = %T %v", err, err)
	}
	var api *typesafe.APIError
	if !errors.As(err, &api) || api.RequestID != "req_bad" || api.Error() != "401 invalid key (request_id=req_bad)" {
		t.Fatalf("API error = %#v", api)
	}
}

func TestConflictError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusConflict)
		_, _ = response.Write([]byte(`{"message":"conflict"}`))
	}))
	defer server.Close()
	policy := typesafe.DefaultRetryPolicy()
	policy.MaxRetries = 0
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test", BaseURL: server.URL, Retry: &policy})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ListModels(context.Background())
	var conflict *typesafe.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	policy := typesafe.DefaultRetryPolicy()
	policy.MaxRetries = 0
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test", BaseURL: server.URL, Timeout: 10 * time.Millisecond, Retry: &policy})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ListModels(context.Background())
	var timeout *typesafe.APITimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestNewClientRejectsInvalidAPIKey(t *testing.T) {
	t.Parallel()
	invalid := map[string]string{
		"internal space": "abc def",
		"tab":            "abc\tdef",
		"newline":        "abc\ndef",
		"control char":   "abc\x01def",
		"non-ascii":      "abcdéf",
		"del char":       "abc\x7fdef",
	}
	for name, key := range invalid {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := typesafe.NewClient(typesafe.Config{APIKey: key})
			if !errors.Is(err, typesafe.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestNewClientAcceptsPrintableASCIIKey(t *testing.T) {
	t.Parallel()
	// Surrounding whitespace is trimmed during resolution; the punctuation-rich key is valid.
	if _, err := typesafe.NewClient(typesafe.Config{APIKey: "  sk-Test_123.ABC-xyz+/=  "}); err != nil {
		t.Fatalf("NewClient: %v", err)
	}
}

func TestQuestionValidation(t *testing.T) {
	t.Parallel()
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State:     "state",
		Questions: map[string]typesafe.Question{"score": typesafe.Score("Rate it", "only one")},
	})
	if !errors.Is(err, typesafe.ErrInvalidRequest) {
		t.Fatalf("error = %v", err)
	}
}
