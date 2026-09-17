package typesafe

import (
	"encoding/json"
	"fmt"
)

// JSONValue is a value accepted by encoding/json. State, instructions, and
// criteria may be strings, JSON objects, JSON arrays, or nil where permitted.
type JSONValue = any

// Question is a typed question sent to the System One API.
type Question interface {
	json.Marshaler
	questionType() string
	validate(name string) error
}

// RawQuestion allows forward-compatible question payloads while retaining
// basic discriminator validation.
type RawQuestion map[string]JSONValue

func (q RawQuestion) questionType() string {
	typeName, _ := q["type"].(string)
	return typeName
}

func (q RawQuestion) validate(name string) error {
	if q.questionType() == "" {
		return fmt.Errorf("raw question %q requires a nonempty string type: %w", name, ErrInvalidRequest)
	}
	if (q.questionType() == "choice" || q.questionType() == "score") && q["criteria"] == nil {
		return fmt.Errorf("raw question %q requires criteria: %w", name, ErrInvalidRequest)
	}
	return nil
}

func (q RawQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]JSONValue(q))
}

// NoulCriteria optionally describes the yes and no outcomes.
type NoulCriteria struct {
	True  JSONValue `json:"true,omitempty"`
	False JSONValue `json:"false,omitempty"`
}

// NoulQuestion asks a yes/no question and returns the probability of yes.
type NoulQuestion struct {
	Instructions JSONValue
	Criteria     *NoulCriteria
}

// Noul creates a yes/no question.
func Noul(instructions JSONValue, criteria ...NoulCriteria) NoulQuestion {
	q := NoulQuestion{Instructions: instructions}
	if len(criteria) > 0 {
		q.Criteria = &criteria[0]
	}
	return q
}

func (NoulQuestion) questionType() string  { return "noul" }
func (NoulQuestion) validate(string) error { return nil }

func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string        `json:"type"`
		Instructions JSONValue     `json:"instructions,omitempty"`
		Criteria     *NoulCriteria `json:"criteria,omitempty"`
	}{Type: q.questionType(), Instructions: q.Instructions, Criteria: q.Criteria})
}

// ChoiceQuestion selects one named option. Criteria maps labels to optional
// descriptions.
type ChoiceQuestion struct {
	Instructions JSONValue
	Criteria     map[string]JSONValue
}

// Choice creates a question that selects one named option.
func Choice(instructions JSONValue, criteria map[string]JSONValue) ChoiceQuestion {
	return ChoiceQuestion{Instructions: instructions, Criteria: criteria}
}

func (ChoiceQuestion) questionType() string { return "choice" }

func (q ChoiceQuestion) validate(name string) error {
	if q.Criteria == nil {
		return fmt.Errorf("choice question %q requires criteria: %w", name, ErrInvalidRequest)
	}
	return nil
}

func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string               `json:"type"`
		Instructions JSONValue            `json:"instructions,omitempty"`
		Criteria     map[string]JSONValue `json:"criteria"`
	}{Type: q.questionType(), Instructions: q.Instructions, Criteria: q.Criteria})
}

// ScoreQuestion rates state against ordered criteria indexed from zero.
type ScoreQuestion struct {
	Instructions JSONValue
	Criteria     []JSONValue
}

// Score creates a question that rates state against an ordered rubric.
func Score(instructions JSONValue, criteria ...JSONValue) ScoreQuestion {
	return ScoreQuestion{Instructions: instructions, Criteria: criteria}
}

func (ScoreQuestion) questionType() string { return "score" }

func (q ScoreQuestion) validate(name string) error {
	if len(q.Criteria) < 2 {
		return fmt.Errorf("score question %q has %d criteria; at least two are required: %w", name, len(q.Criteria), ErrInvalidRequest)
	}
	return nil
}

func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string      `json:"type"`
		Instructions JSONValue   `json:"instructions,omitempty"`
		Criteria     []JSONValue `json:"criteria"`
	}{Type: q.questionType(), Instructions: q.Instructions, Criteria: q.Criteria})
}

func validateQuestions(questions map[string]Question) error {
	if len(questions) == 0 {
		return fmt.Errorf("at least one question is required: %w", ErrInvalidRequest)
	}
	for name, question := range questions {
		if question == nil {
			return fmt.Errorf("question %q is nil: %w", name, ErrInvalidRequest)
		}
		if err := question.validate(name); err != nil {
			return err
		}
	}
	return nil
}
