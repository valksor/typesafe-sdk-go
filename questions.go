package typesafe

import (
	"encoding/json"
	"fmt"
	"reflect"
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
	switch q.questionType() {
	case "choice":
		return validateChoiceCriteria(name, q["criteria"])
	case "score":
		return validateScoreCriteria(name, q["criteria"])
	case "noul":
		return validateNoul(name, q["instructions"], q["criteria"])
	}
	return nil
}

func (q RawQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(sanitizeNonFinite(map[string]JSONValue(q)))
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

func (NoulQuestion) questionType() string { return "noul" }

func (q NoulQuestion) validate(name string) error {
	return validateNoul(name, q.Instructions, q.Criteria)
}

func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string        `json:"type"`
		Instructions JSONValue     `json:"instructions,omitempty"`
		Criteria     *NoulCriteria `json:"criteria,omitempty"`
	}{Type: q.questionType(), Instructions: sanitizeNonFinite(q.Instructions), Criteria: q.Criteria.sanitized()})
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
	return validateChoiceCriteria(name, q.Criteria)
}

func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string    `json:"type"`
		Instructions JSONValue `json:"instructions,omitempty"`
		Criteria     JSONValue `json:"criteria"`
	}{Type: q.questionType(), Instructions: sanitizeNonFinite(q.Instructions), Criteria: sanitizeNonFinite(q.Criteria)})
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
	return validateScoreCriteria(name, q.Criteria)
}

func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string    `json:"type"`
		Instructions JSONValue `json:"instructions,omitempty"`
		Criteria     JSONValue `json:"criteria"`
	}{Type: q.questionType(), Instructions: sanitizeNonFinite(q.Instructions), Criteria: sanitizeNonFinite(q.Criteria)})
}

// emptyContainer reports whether criteria is nil or a map, slice, or array with
// no entries. Any other non-nil value is left for the API to judge.
func emptyContainer(criteria JSONValue) bool {
	if criteria == nil {
		return true
	}
	switch v := reflect.ValueOf(criteria); v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return v.Len() == 0
	}
	return false
}

// validateChoiceCriteria rejects a choice question with no criteria; at least
// one choice is required. A nil or missing value counts as empty.
func validateChoiceCriteria(name string, criteria JSONValue) error {
	if emptyContainer(criteria) {
		return fmt.Errorf("choice question %q has no criteria; at least one choice is required: %w", name, ErrInvalidRequest)
	}
	return nil
}

// validateScoreCriteria rejects a score question with no criteria; at least
// one score is required. A nil or missing value counts as empty.
func validateScoreCriteria(name string, criteria JSONValue) error {
	if emptyContainer(criteria) {
		return fmt.Errorf("score question %q has no criteria; at least one score is required: %w", name, ErrInvalidRequest)
	}
	return nil
}

// validateNoul rejects a noul question whose criteria is not an object, or that
// has neither instructions nor a described outcome. Criteria may be any map, or
// a NoulCriteria (or pointer to one). A map entry is described unless it is a
// nil interface value.
func validateNoul(name string, instructions, criteria JSONValue) error {
	described := false
	switch c := criteria.(type) {
	case nil:
	case NoulCriteria:
		described = c.True != nil || c.False != nil
	case *NoulCriteria:
		described = c != nil && (c.True != nil || c.False != nil)
	default:
		v := reflect.ValueOf(criteria)
		if v.Kind() != reflect.Map {
			return fmt.Errorf("noul question %q criteria must be an object: %w", name, ErrInvalidRequest)
		}
		for iter := v.MapRange(); iter.Next() && !described; {
			value := iter.Value()
			described = value.Kind() != reflect.Interface || !value.IsNil()
		}
	}
	if instructions == nil && !described {
		return fmt.Errorf("noul question %q has neither instructions nor criteria; at least one is required: %w", name, ErrInvalidRequest)
	}
	return nil
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
