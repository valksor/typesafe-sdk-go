# Changelog

## v0.7.4 (2026-10-13)

- Track upstream Python SDK 0.7.4 (covers 0.7.2–0.7.4).
- Reject a missing request state locally: `SystemOneRequest.State == nil` now fails with
  `ErrInvalidRequest` before any HTTP call.
- Reject a `Choice` question, typed or raw, with empty, nil, or missing criteria; at least one
  choice is required. Raw criteria may be any map or slice type (for example
  `map[string]string`); only nil and zero-length containers are rejected.
- Validate `Noul` questions, typed and raw: raw `criteria` must be an object (any
  map type, `NoulCriteria`, or `*NoulCriteria`), and a question with nil instructions
  needs at least one non-nil `true`/`false` outcome (any entry of a non-interface map such as
  `map[string]string` counts as described).
- Behaviour change: `Score` now requires at least one criterion instead of two, typed and raw,
  matching upstream. A single-criterion score question that used to fail with `ErrInvalidRequest`
  is now sent to the API.
- Serialize non-finite numbers as the strings `"NaN"`, `"Infinity"`, and `"-Infinity"` instead of
  failing to encode them. This covers `float32`/`float64` values inside `State`, `ExtraBody`,
  question instructions and criteria, and raw questions, nested in `map[string]any`, `[]any`,
  `[]float64`, or `[]float32`. Finite values encode exactly as before. Caller-defined structs and
  other collection types holding a NaN or infinity are not rewritten and still fail to encode.
- Raise the default connection pool: when `Config.HTTPClient` is nil, the client uses a clone of
  `http.DefaultTransport` with 100 idle connections (also per host), at most 100 connections per
  host, and a 30 second idle timeout, so bursts reuse TLS connections instead of reopening them
  (the Go default keeps two idle connections per host). A caller-supplied `HTTPClient` is used
  untouched.
- Not ported: upstream's `truststore` TLS lock fix is specific to Python's AnyIO/`truststore`
  stack, and its `http2` extra is packaging only. Go's `net/http` negotiates HTTP/2 over TLS by
  default, and the cloned default transport keeps that behaviour.

## v0.7.1 (2026-09-22)

- Track upstream Python SDK 0.7.1. No wire-contract change.
- Validate the API key when the client is constructed: the resolved key is trimmed and rejected
  if it is empty or contains whitespace, control characters, or non-ASCII characters. A malformed
  credential now fails fast at `NewClient` with `ErrInvalidRequest` instead of forming a broken
  `Authorization` header that fails later in the HTTP stack.
- Document pointing the client at an OpenAI-style AI gateway by overriding `Config.BaseURL` (or
  `TYPESAFE_BASE_URL`).
- Upstream's exception-redaction fix masks credential values that Python's `httpx` can embed in a
  transport exception's message or chain. Go's `net/http` transport errors do not carry request
  header values (and already strip URL-embedded passwords), so a connection or timeout error
  surfaced by this SDK cannot contain the API key; there is no equivalent to port beyond the
  early validation above.

## v0.7.0 (2026-09-19)

- Track upstream Python and JavaScript SDK 0.7.0. No wire-contract change.
- Add `SystemOneAs[T]`, a generic response-model decode that answers named questions and
  decodes the response into a caller-supplied type (parity with upstream `response_model`).
  Declared answer fields are lifted from `answers.{name}` to top-level keys; forward-compatible
  unknown answer types are dropped; decode failures surface as `*ResponseValidationError` (with a
  field path where the JSON error provides one). `encoding/json` validates field types but not presence, so a missing field decodes
  to its zero value without error; use `SystemOne` when strict envelope validation or response
  metadata is required.
- `SystemOne` and every existing type are unchanged and fully backward compatible.
- Upstream's msgspec-to-pydantic migration and the str-subclass serialization fix are
  Python-internal implementation details with no Go equivalent.

## v0.6.0 (2026-09-17)

- Initial Go SDK release with typed Noul, Choice, and Score questions and answers.
- Add configurable retries, timeouts, request metadata, model discovery, and typed API errors.
- Add injectable HTTP transport support through `http.Client` and opt-in live integration testing.
