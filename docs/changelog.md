# Changelog

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
