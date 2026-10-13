package typesafe_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	typesafe "github.com/valksor/typesafe-sdk-go"
)

const emptyResponse = `{"model":"jev-latest","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`

// newCapturingClient returns a client whose server records the request body and
// the number of requests it served.
func newCapturingClient(t *testing.T) (*typesafe.Client, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var calls atomic.Int32
	var body atomic.Value
	client := observedJSONServer(t, http.StatusOK, emptyResponse, func(data []byte) {
		calls.Add(1)
		body.Store(string(data))
	})
	return client, &calls, &body
}

func TestRequestValidationBeforeNetwork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		state     typesafe.JSONValue
		questions map[string]typesafe.Question
		want      string
	}{
		{"missing state", nil, map[string]typesafe.Question{"q": typesafe.Noul("?")}, "state is required"},
		{"typed empty choice", "x", map[string]typesafe.Question{"q": typesafe.Choice("?", map[string]any{})}, `choice question "q" has no criteria; at least one choice is required`},
		{"typed nil choice", "x", map[string]typesafe.Question{"q": typesafe.Choice("?", nil)}, `choice question "q" has no criteria`},
		{"raw empty choice", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "choice", "criteria": map[string]any{}}}, `choice question "q" has no criteria`},
		{"raw nil choice", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "choice", "criteria": nil}}, `choice question "q" has no criteria`},
		{"raw missing choice", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "choice"}}, `choice question "q" has no criteria`},
		{"typed empty score", "x", map[string]typesafe.Question{"q": typesafe.Score("?")}, `score question "q" has no criteria; at least one score is required`},
		{"raw empty score", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "score", "criteria": []any{}}}, `score question "q" has no criteria`},
		{"raw missing score", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "score"}}, `score question "q" has no criteria`},
		{"raw empty typed-map choice", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "choice", "criteria": map[string]string{}}}, `choice question "q" has no criteria`},
		{"raw empty typed-slice score", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "score", "criteria": []string{}}}, `score question "q" has no criteria`},
		{"raw noul empty typed-map criteria", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "criteria": map[string]string{}}}, `noul question "q" has neither instructions nor criteria`},
		{"typed noul without either", "x", map[string]typesafe.Question{"q": typesafe.Noul(nil)}, `noul question "q" has neither instructions nor criteria; at least one is required`},
		{"typed noul null outcomes", "x", map[string]typesafe.Question{"q": typesafe.Noul(nil, typesafe.NoulCriteria{})}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul without either", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul"}}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul null criteria", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "criteria": nil}}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul empty criteria", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "criteria": map[string]any{}}}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul null outcomes", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "criteria": map[string]any{"true": nil, "false": nil}}}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul criteria struct without either", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "criteria": &typesafe.NoulCriteria{}}}, `noul question "q" has neither instructions nor criteria`},
		{"raw noul string criteria", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "instructions": "?", "criteria": "spam"}}, `noul question "q" criteria must be an object`},
		{"raw noul array criteria", "x", map[string]typesafe.Question{"q": typesafe.RawQuestion{"type": "noul", "instructions": "?", "criteria": []any{"a"}}}, `noul question "q" criteria must be an object`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, calls, _ := newCapturingClient(t)
			_, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{State: test.state, Questions: test.questions})
			if !errors.Is(err, typesafe.ErrInvalidRequest) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want ErrInvalidRequest containing %q", err, test.want)
			}
			if calls.Load() != 0 {
				t.Fatalf("invalid request reached the network")
			}
		})
	}
}

func TestValidQuestionsAreAccepted(t *testing.T) {
	t.Parallel()
	questions := map[string]typesafe.Question{
		"score-one":      typesafe.Score("Rate it", "only one"),
		"raw-score-one":  typesafe.RawQuestion{"type": "score", "criteria": []any{"only one"}},
		"choice-one":     typesafe.Choice("Which?", map[string]any{"a": nil}),
		"noul-instr":     typesafe.Noul("Spam?"),
		"noul-criteria":  typesafe.Noul(nil, typesafe.NoulCriteria{True: "spam"}),
		"noul-false":     typesafe.Noul(nil, typesafe.NoulCriteria{False: "ham"}),
		"raw-noul-map":   typesafe.RawQuestion{"type": "noul", "criteria": map[string]any{"true": "spam", "false": nil}},
		"raw-noul-ptr":   typesafe.RawQuestion{"type": "noul", "criteria": &typesafe.NoulCriteria{False: "ham"}},
		"raw-noul-value": typesafe.RawQuestion{"type": "noul", "criteria": typesafe.NoulCriteria{True: "spam"}},
		"raw-choice-str": typesafe.RawQuestion{"type": "choice", "criteria": map[string]string{"a": "x"}},
		"raw-score-str":  typesafe.RawQuestion{"type": "score", "criteria": []string{"low", "high"}},
		"raw-noul-str":   typesafe.RawQuestion{"type": "noul", "criteria": map[string]string{"true": "yes"}},
		"raw-future":     typesafe.RawQuestion{"type": "future"},
	}
	client, calls, _ := newCapturingClient(t)
	if _, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{State: "x", Questions: questions}); err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestNonFiniteNumbersAreStringified(t *testing.T) {
	t.Parallel()
	client, _, body := newCapturingClient(t)
	_, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State: map[string]any{
			"value": math.NaN(),
			"arr":   []any{1.0, math.Inf(1), math.Inf(-1)},
			"nest":  map[string]any{"deep": []any{map[string]any{"f32": float32(math.Inf(1))}}},
			"vec":   []float64{0.5, math.NaN()},
			"ok":    []float64{0.5, 2},
		},
		Questions: map[string]typesafe.Question{
			"noul":   typesafe.Noul(map[string]any{"x": math.NaN()}, typesafe.NoulCriteria{True: math.Inf(1), False: "no"}),
			"choice": typesafe.Choice([]any{math.Inf(-1)}, map[string]any{"a": math.NaN(), "b": nil}),
			"score":  typesafe.Score(math.NaN(), math.Inf(1), "fine", 2.5),
			"raw":    typesafe.RawQuestion{"type": "future", "extra": []any{math.NaN()}},
		},
		ExtraBody: map[string]any{"temperature": math.NaN(), "top": map[string]any{"p": math.Inf(1)}},
	})
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	got := body.Load().(string)
	for _, want := range []string{
		`"state":{"arr":[1,"Infinity","-Infinity"],"nest":{"deep":[{"f32":"Infinity"}]},"ok":[0.5,2],"value":"NaN","vec":[0.5,"NaN"]}`,
		`"temperature":"NaN"`,
		`"top":{"p":"Infinity"}`,
		`"noul":{"type":"noul","instructions":{"x":"NaN"},"criteria":{"true":"Infinity","false":"no"}}`,
		`"choice":{"type":"choice","instructions":["-Infinity"],"criteria":{"a":"NaN","b":null}}`,
		`"score":{"type":"score","instructions":"NaN","criteria":["Infinity","fine",2.5]}`,
		`"raw":{"extra":["NaN"],"type":"future"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %s\nbody: %s", want, got)
		}
	}
}

func TestFiniteNumbersEncodeUnchanged(t *testing.T) {
	t.Parallel()
	client, _, body := newCapturingClient(t)
	_, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State:     map[string]any{"n": 1e21, "f": float32(0.1), "i": 3, "s": []float64{0.1, 1e-7}},
		Questions: map[string]typesafe.Question{"q": typesafe.Noul(1.5)},
	})
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}
	got := body.Load().(string)
	for _, want := range []string{`"state":{"f":0.1,"i":3,"n":1e+21,"s":[0.1,1e-7]}`, `"q":{"type":"noul","instructions":1.5}`} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %s\nbody: %s", want, got)
		}
	}
}
