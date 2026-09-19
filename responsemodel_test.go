package typesafe_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	typesafe "github.com/valksor/typesafe-sdk-go"
)

// jsonServer returns a client pointed at a server that replies with status and
// body for every request.
func jsonServer(t *testing.T, status int, body string) *typesafe.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("x-typesafe-request-id", "req_model")
		response.WriteHeader(status)
		_, _ = response.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	policy := typesafe.DefaultRetryPolicy()
	policy.MaxRetries = 0
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test", BaseURL: server.URL, Retry: &policy})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func modelRequest() typesafe.SystemOneRequest {
	return typesafe.SystemOneRequest{
		State: map[string]any{"message": "help"},
		Questions: map[string]typesafe.Question{
			"spam": typesafe.Noul("Is this spam?"),
		},
	}
}

const modelBody = `{
    "model":"jev-latest",
    "answers":{
        "spam":{"type":"noul","noul":0.98},
        "tone":{"type":"choice","choice":"friendly","confidence":0.9,"probabilities":{"friendly":0.9,"hostile":0.1}},
        "future":{"type":"future","value":42}
    },
    "usage":{"input_tokens":10,"output_tokens":4}
}`

func TestSystemOneAsNestedAnswers(t *testing.T) {
	t.Parallel()
	type known struct {
		Model   string `json:"model"`
		Answers struct {
			Spam typesafe.NoulAnswer `json:"spam"`
		} `json:"answers"`
	}
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[known](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if result.Model != "jev-latest" || result.Answers.Spam.Noul != 0.98 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSystemOneAsLiftedFields(t *testing.T) {
	t.Parallel()
	type typed struct {
		Spam    typesafe.NoulAnswer   `json:"spam"`
		Tone    typesafe.ChoiceAnswer `json:"tone"`
		Missing *typesafe.NoulAnswer  `json:"missing"`
		Usage   typesafe.Usage        `json:"usage"`
	}
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if result.Spam.Noul != 0.98 {
		t.Fatalf("spam not lifted: %#v", result.Spam)
	}
	if result.Tone.Choice != "friendly" || result.Tone.Probabilities["friendly"] != 0.9 {
		t.Fatalf("tone not lifted: %#v", result.Tone)
	}
	if result.Missing != nil {
		t.Fatalf("missing should be nil: %#v", result.Missing)
	}
	if result.Usage.InputTokens != 10 {
		t.Fatalf("usage not decoded: %#v", result.Usage)
	}
}

func TestSystemOneAsMissingRequiredField(t *testing.T) {
	t.Parallel()
	// An answer T declares as a non-pointer struct but the server omits decodes
	// to a zero value with no error: encoding/json does not enforce presence.
	type typed struct {
		Spam typesafe.NoulAnswer   `json:"spam"`
		Team typesafe.ChoiceAnswer `json:"team"`
	}
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: unexpected error %v", err)
	}
	if result.Spam.Noul != 0.98 {
		t.Fatalf("spam not lifted: %#v", result.Spam)
	}
	if result.Team.Choice != "" || result.Team.Probabilities != nil {
		t.Fatalf("absent team should be zero-valued: %#v", result.Team)
	}
}

func TestSystemOneAsDropsUnknownAnswer(t *testing.T) {
	t.Parallel()
	type typed struct {
		Spam    typesafe.NoulAnswer        `json:"spam"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if result.Spam.Noul != 0.98 {
		t.Fatalf("spam not lifted: %#v", result.Spam)
	}
	if _, ok := result.Answers["future"]; ok {
		t.Fatalf("unknown answer should be dropped from retained answers: %v", result.Answers)
	}
	if _, ok := result.Answers["spam"]; !ok {
		t.Fatalf("known answer should remain in retained answers: %v", result.Answers)
	}
}

func TestSystemOneAsReservedKeyNotClobbered(t *testing.T) {
	t.Parallel()
	// Answers literally named "model", "usage", or "answers" must not overwrite
	// those reserved top-level fields.
	body := `{
        "model":"jev-latest",
        "answers":{
            "model":{"type":"noul","noul":0.5},
            "usage":{"type":"noul","noul":0.4},
            "answers":{"type":"noul","noul":0.3},
            "spam":{"type":"noul","noul":0.98}
        },
        "usage":{"input_tokens":7,"output_tokens":2}
    }`
	type typed struct {
		Model string              `json:"model"`
		Usage typesafe.Usage      `json:"usage"`
		Spam  typesafe.NoulAnswer `json:"spam"`
	}
	client := jsonServer(t, http.StatusOK, body)
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if result.Model != "jev-latest" {
		t.Fatalf("wire model was clobbered: %q", result.Model)
	}
	if result.Usage.InputTokens != 7 {
		t.Fatalf("wire usage was clobbered: %#v", result.Usage)
	}
	if result.Spam.Noul != 0.98 {
		t.Fatalf("spam not lifted: %#v", result.Spam)
	}
}

func TestSystemOneAsMalformedAnswerErrors(t *testing.T) {
	t.Parallel()
	type typed struct {
		Spam typesafe.NoulAnswer `json:"spam"`
	}
	// A null answer, a non-object answer, and an answer without a string type are
	// all malformed responses and must surface as *ResponseValidationError, not be
	// silently dropped like an unknown (but well-formed) future answer type.
	cases := map[string]string{
		"null answer":     `{"model":"m","answers":{"spam":{"type":"noul","noul":0.98},"broken":null},"usage":{}}`,
		"non-object":      `{"model":"m","answers":{"spam":"nope"},"usage":{}}`,
		"type not string": `{"model":"m","answers":{"spam":{"type":123}},"usage":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := jsonServer(t, http.StatusOK, body)
			_, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
			var validation *typesafe.ResponseValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %T %v", err, err)
			}
			if !errors.Is(err, typesafe.ErrInvalidResponse) {
				t.Fatalf("error should wrap ErrInvalidResponse: %v", err)
			}
		})
	}
}

func TestSystemOneAsMalformedBodyErrors(t *testing.T) {
	t.Parallel()
	type typed struct {
		Spam typesafe.NoulAnswer `json:"spam"`
	}
	// A top-level body that is not a JSON object cannot be lifted.
	client := jsonServer(t, http.StatusOK, `["not","an","object"]`)
	_, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	var validation *typesafe.ResponseValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestSystemOneAsNonStructType(t *testing.T) {
	t.Parallel()
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[map[string]any](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if (*result)["model"] != "jev-latest" {
		t.Fatalf("map decode missing model: %#v", *result)
	}
	if _, ok := (*result)["spam"]; !ok {
		t.Fatalf("map decode missing lifted spam: %#v", *result)
	}
}

func TestSystemOneAsTypeMismatch(t *testing.T) {
	t.Parallel()
	body := `{
        "model":"jev-latest",
        "answers":{"spam":{"type":"noul","noul":"not-a-number"}},
        "usage":{"input_tokens":1,"output_tokens":1}
    }`
	type typed struct {
		Spam typesafe.NoulAnswer `json:"spam"`
	}
	client := jsonServer(t, http.StatusOK, body)
	_, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	var validation *typesafe.ResponseValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T %v", err, err)
	}
	if !errors.Is(err, typesafe.ErrInvalidResponse) {
		t.Fatalf("error should wrap ErrInvalidResponse: %v", err)
	}
	if validation.Meta.RequestID != "req_model" {
		t.Fatalf("meta not carried on validation error: %#v", validation.Meta)
	}
	if !strings.Contains(err.Error(), "spam.noul") {
		t.Fatalf("error should name the field path: %v", err)
	}
}

func TestSystemOneAsFallbackDecodeError(t *testing.T) {
	t.Parallel()
	// Decoding an object body into a non-struct scalar T yields an UnmarshalTypeError
	// with an empty Field, exercising validationCause's fallback branch. The message
	// must wrap ErrInvalidResponse, stay on a single line, and not duplicate text.
	client := jsonServer(t, http.StatusOK, modelBody)
	_, err := typesafe.SystemOneAs[int](context.Background(), client, modelRequest())
	var validation *typesafe.ResponseValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T %v", err, err)
	}
	if !errors.Is(err, typesafe.ErrInvalidResponse) {
		t.Fatalf("error should wrap ErrInvalidResponse: %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error message should be single-line: %q", err.Error())
	}
}

func TestSystemOneAsNoMetaOnSuccess(t *testing.T) {
	t.Parallel()
	// A field named "meta" is never injected by the SDK on the success path.
	type typed struct {
		Spam typesafe.NoulAnswer   `json:"spam"`
		Meta typesafe.ResponseMeta `json:"meta"`
	}
	client := jsonServer(t, http.StatusOK, modelBody)
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	if err != nil {
		t.Fatalf("SystemOneAs: %v", err)
	}
	if result.Meta.RequestID != "" {
		t.Fatalf("meta should not be attached on success: %#v", result.Meta)
	}
}

func TestSystemOneAsAPIError(t *testing.T) {
	t.Parallel()
	client := jsonServer(t, http.StatusUnauthorized, `{"error":"invalid key"}`)
	type typed struct {
		Spam typesafe.NoulAnswer `json:"spam"`
	}
	result, err := typesafe.SystemOneAs[typed](context.Background(), client, modelRequest())
	var authentication *typesafe.AuthenticationError
	if !errors.As(err, &authentication) {
		t.Fatalf("error = %T %v", err, err)
	}
	if result != nil {
		t.Fatalf("result should be nil on API error: %#v", result)
	}
}

func TestSystemOneAsQuestionValidation(t *testing.T) {
	t.Parallel()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		called = true
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client, err := typesafe.NewClient(typesafe.Config{APIKey: "test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	type typed struct {
		Spam typesafe.NoulAnswer `json:"spam"`
	}
	_, err = typesafe.SystemOneAs[typed](context.Background(), client, typesafe.SystemOneRequest{
		State:     "x",
		Questions: nil,
	})
	if !errors.Is(err, typesafe.ErrInvalidRequest) {
		t.Fatalf("error should be ErrInvalidRequest: %T %v", err, err)
	}
	if called {
		t.Fatalf("request should short-circuit before any HTTP call")
	}
}
