package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// SystemOneAs answers named questions and decodes the response into a
// caller-supplied type T, mirroring the response_model argument of the official
// Python SDK. It shares all request plumbing with [Client.SystemOne]; only the
// decoding differs. It is a package-level function rather than a method because
// Go methods cannot declare type parameters.
//
// Each answer is "lifted" from answers.{name} to a top-level JSON key {name}
// before decoding, so T may declare typed answer fields at the top level:
//
//	type Triage struct {
//		Model string              `json:"model"`
//		Spam  typesafe.NoulAnswer `json:"spam"`
//		Tone  typesafe.ChoiceAnswer `json:"tone"`
//	}
//	result, err := typesafe.SystemOneAs[Triage](ctx, client, request)
//
// The pruned answers object is preserved, so T may also declare a nested
// answers field (for example map[string]json.RawMessage) alongside lifted
// fields. Reserved top-level keys (model, usage, answers) are never overwritten
// by an answer of the same name. Answer types this SDK version does not
// recognize are dropped for forward compatibility (from both the lifted keys and
// the retained answers object), but an answer that is not a JSON object with a
// string type is treated as a malformed response and reported as an error.
//
// Validation scope: decoding uses [encoding/json], which enforces field types
// but not presence. A missing answer or field decodes to its zero value without
// error — weaker than SystemOne, which rejects an absent model, answers, or
// usage. Callers needing strict guarantees should declare pointer fields and
// nil-check them, validate T themselves, or use SystemOne. A type mismatch
// (e.g. a string where a number is expected) is reported as a
// [*ResponseValidationError].
//
// Unlike SystemOne, SystemOneAs cannot attach [ResponseMeta] to the returned
// value on success, since T is caller-defined; the request ID and other
// metadata are available on the [*ResponseValidationError] when decoding fails,
// on the typed [*APIError] for non-2xx responses, or by using SystemOne. T is
// typically a struct, but any type json.Unmarshal accepts (such as a map) works.
func SystemOneAs[T any](ctx context.Context, client *Client, request SystemOneRequest, options ...RequestOptions) (*T, error) {
	data, meta, err := client.systemOneRaw(ctx, request, options...)
	if err != nil {
		return nil, err
	}
	lifted, err := liftAnswers(data)
	if err != nil {
		return nil, &ResponseValidationError{Cause: err, Meta: meta, Body: append([]byte(nil), data...)}
	}
	result := new(T)
	if err := json.Unmarshal(lifted, result); err != nil {
		return nil, &ResponseValidationError{Cause: validationCause(err), Meta: meta, Body: append([]byte(nil), data...)}
	}
	return result, nil
}

// liftAnswers copies each recognized answer from answers.{name} to a top-level
// key {name} and returns the re-encoded object. Reserved keys and keys already
// present at the top level are left untouched. An answer whose value is not a
// JSON object with a string "type" is a malformed response and reported as an
// error (matching decodeAnswer and the official SDKs); an answer whose type is a
// valid string this SDK version does not model is dropped for forward
// compatibility, from both the lifted keys and the retained answers object.
func liftAnswers(data []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode response object: %w", err)
	}
	rawAnswers, ok := root["answers"]
	if !ok {
		return data, nil
	}
	var answers map[string]json.RawMessage
	if err := json.Unmarshal(rawAnswers, &answers); err != nil {
		return nil, fmt.Errorf("decode answers object: %w", err)
	}
	kept := make(map[string]json.RawMessage, len(answers))
	for name, raw := range answers {
		var probe struct {
			Type *string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Type == nil {
			return nil, fmt.Errorf("answers.%s.type: %w", name, ErrInvalidResponse)
		}
		if _, known := knownAnswerTypes[*probe.Type]; !known {
			// Forward-compat: drop answer types this SDK version does not model.
			continue
		}
		kept[name] = raw
		switch name {
		case "model", "usage", "answers":
			continue
		}
		if _, exists := root[name]; exists {
			continue
		}
		root[name] = raw
	}
	prunedAnswers, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	root["answers"] = prunedAnswers
	return json.Marshal(root)
}

// validationCause enriches a json decode error with its field path when
// available, wrapping ErrInvalidResponse so errors.Is keeps working. The
// fallback keeps the message on a single line (no errors.Join, which would embed
// a newline) while still wrapping both the original error and ErrInvalidResponse.
func validationCause(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return fmt.Errorf("field %q: cannot decode %s into %s: %w", typeErr.Field, typeErr.Value, typeErr.Type, ErrInvalidResponse)
	}
	return fmt.Errorf("decode response into typed model: %w: %w", err, ErrInvalidResponse)
}
