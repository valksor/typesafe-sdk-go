# Changelog

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
