package typesafe

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Usage reports token use for an API request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Answer is a response to one question.
type Answer interface {
	answerType() string
}

type NoulAnswer struct {
	Type string  `json:"type"`
	Noul float64 `json:"noul"`
}

func (NoulAnswer) answerType() string { return "noul" }

type ChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

func (ChoiceAnswer) answerType() string { return "choice" }

type ScoreAnswer struct {
	Type          string               `json:"type"`
	Score         float64              `json:"score"`
	Confidence    float64              `json:"confidence"`
	Legend        map[string]JSONValue `json:"legend"`
	Probabilities map[string]float64   `json:"probabilities"`
}

func (ScoreAnswer) answerType() string { return "score" }

// UnknownAnswer preserves an answer type added by a future API version.
type UnknownAnswer struct {
	Type string
	Raw  json.RawMessage
}

func (a UnknownAnswer) answerType() string { return a.Type }

// ResponseMeta contains HTTP metadata shared by successful responses.
type ResponseMeta struct {
	StatusCode int
	Headers    http.Header
	RequestID  string
}

// SystemOneResponse contains answers keyed by the corresponding question IDs.
type SystemOneResponse struct {
	Model   string
	Answers map[string]Answer
	Usage   Usage
	Meta    ResponseMeta
}

func decodeSystemOne(body []byte) (*SystemOneResponse, error) {
	var wire struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
		Usage   *Usage                     `json:"usage"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode System One response: %w", err)
	}
	if wire.Model == "" {
		return nil, fmt.Errorf("decode System One response: missing model: %w", ErrInvalidResponse)
	}
	if wire.Answers == nil {
		return nil, fmt.Errorf("decode System One response: missing answers: %w", ErrInvalidResponse)
	}
	if wire.Usage == nil {
		return nil, fmt.Errorf("decode System One response: missing usage: %w", ErrInvalidResponse)
	}
	answers := make(map[string]Answer, len(wire.Answers))
	for name, raw := range wire.Answers {
		answer, err := decodeAnswer(raw)
		if err != nil {
			return nil, fmt.Errorf("decode answer %q: %w", name, err)
		}
		answers[name] = answer
	}
	return &SystemOneResponse{Model: wire.Model, Answers: answers, Usage: *wire.Usage}, nil
}

// knownAnswerTypes is the single source of truth for the answer discriminators
// this SDK version models. The switch in decodeAnswer and the forward-compat
// check in liftAnswers (response_model.go) must both stay consistent with it.
var knownAnswerTypes = map[string]struct{}{
	"noul":   {},
	"choice": {},
	"score":  {},
}

func decodeAnswer(raw json.RawMessage) (Answer, error) {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &tag); err != nil {
		return nil, fmt.Errorf("read answer type: %w", err)
	}
	if tag.Type == "" {
		return nil, fmt.Errorf("missing type: %w", ErrInvalidResponse)
	}
	switch tag.Type {
	case "noul":
		var answer NoulAnswer
		if err := json.Unmarshal(raw, &answer); err != nil {
			return nil, err
		}
		return answer, nil
	case "choice":
		var answer ChoiceAnswer
		if err := json.Unmarshal(raw, &answer); err != nil {
			return nil, err
		}
		if answer.Choice == "" || answer.Probabilities == nil {
			return nil, fmt.Errorf("missing choice fields: %w", ErrInvalidResponse)
		}
		return answer, nil
	case "score":
		var answer ScoreAnswer
		if err := json.Unmarshal(raw, &answer); err != nil {
			return nil, err
		}
		if answer.Legend == nil || answer.Probabilities == nil {
			return nil, fmt.Errorf("missing score fields: %w", ErrInvalidResponse)
		}
		return answer, nil
	default:
		return UnknownAnswer{Type: tag.Type, Raw: append(json.RawMessage(nil), raw...)}, nil
	}
}

func (r *SystemOneResponse) Noul(name string) (NoulAnswer, bool) {
	answer, ok := r.Answers[name].(NoulAnswer)
	return answer, ok
}

func (r *SystemOneResponse) Choice(name string) (ChoiceAnswer, bool) {
	answer, ok := r.Answers[name].(ChoiceAnswer)
	return answer, ok
}

func (r *SystemOneResponse) Score(name string) (ScoreAnswer, bool) {
	answer, ok := r.Answers[name].(ScoreAnswer)
	return answer, ok
}
