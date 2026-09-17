# typesafe-sdk-go

[![CI](https://github.com/valksor/typesafe-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/valksor/typesafe-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/valksor/typesafe-sdk-go.svg)](https://pkg.go.dev/github.com/valksor/typesafe-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/valksor/typesafe-sdk-go)](https://goreportcard.com/report/github.com/valksor/typesafe-sdk-go)

An idiomatic Go client for the [TypeSafe AI](https://typesafe.ai) System One API.

> [!IMPORTANT]
> **Unofficial and unaffiliated.** This project is **not** created, maintained,
> endorsed by, or associated with TypeSafe AI in any way. It is an independent,
> community-maintained SDK that aims for **1:1 feature parity** with the official
> [JavaScript](https://github.com/typesafe-ai/typesafe-sdk-js) and
> [Python](https://github.com/typesafe-ai/typesafe-sdk-python) SDKs — the same
> wire contract, environment configuration, retry semantics, request metadata,
> and typed errors — adapted to idiomatic Go. All product names, logos, and
> brands are property of their respective owners.

## Features

- Typed `Noul`, `Choice`, and `Score` questions and their answers
- Single `SystemOne` call with typed answer lookup helpers
- Model discovery via `client.Models.List`
- Environment-based configuration with explicit overrides
- Configurable per-attempt timeouts and exponential-backoff retries
- Request IDs and raw response metadata on every call
- Injectable `*http.Client` transport for framework integration and tests
- Status-specific typed errors that work with `errors.As`
- Context-aware cancellation throughout

## Install

```sh
go get github.com/valksor/typesafe-sdk-go
```

Requires Go 1.25 or newer.

## Quick start

Set `TYPESAFE_API_KEY`, then ask typed questions:

```go
package main

import (
	"context"
	"fmt"
	"log"

	typesafe "github.com/valksor/typesafe-sdk-go"
)

func main() {
	client, err := typesafe.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	response, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State: map[string]any{"document": "I was charged twice. Please fix this ASAP."},
		Questions: map[string]typesafe.Question{
			"category": typesafe.Choice("What is this ticket about?", map[string]any{
				"billing": nil, "technical": nil, "other": nil,
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	category, ok := response.Choice("category")
	if !ok {
		log.Fatal("category was not a choice answer")
	}
	fmt.Println(category.Choice, category.Confidence)
}
```

`Noul`, `Choice`, and `Score` create the three question types. Responses keep all
answers in `SystemOneResponse.Answers` and provide typed lookup helpers
(`Noul`, `Choice`, `Score`). List available models with
`client.Models.List(ctx)`.

## Configuration

`NewClient` accepts an optional `Config`. Explicit values take precedence over
environment variables:

| Setting               | Environment variable      | Default                    |
| --------------------- | ------------------------- | -------------------------- |
| API key (required)    | `TYPESAFE_API_KEY`        | —                          |
| Base URL              | `TYPESAFE_BASE_URL`       | `https://api.typesafe.ai`  |
| Default model         | `TYPESAFE_DEFAULT_MODEL`  | `jev-latest`               |

The default timeout is 10 seconds per attempt. The client retries HTTP 408, 429,
and 5xx responses, connection errors, and timeouts twice with exponential
backoff. Pass `Config.Retry` for client-wide behavior or per-call
`RequestOptions` to override it for a single request.

## Error handling

HTTP failures support `errors.As` with `*typesafe.APIError` and status-specific
types:

```go
resp, err := client.SystemOne(ctx, req)
if err != nil {
	var rateLimit *typesafe.RateLimitError
	var apiErr *typesafe.APIError
	switch {
	case errors.As(err, &rateLimit):
		time.Sleep(rateLimit.RetryAfter)
	case errors.As(err, &apiErr):
		log.Printf("api error %d (request %s): %s", apiErr.StatusCode, apiErr.RequestID, apiErr.Message)
	default:
		log.Fatal(err)
	}
}
```

Status codes map to `*BadRequestError` (400), `*AuthenticationError` (401),
`*PermissionDeniedError` (403), `*NotFoundError` (404), `*ConflictError` (409),
`*UnprocessableEntityError` (422), `*RateLimitError` (429), and
`*InternalServerError` (5xx). Transport-level failures surface as
`*APIConnectionError`, `*APITimeoutError`, and `*APIUserAbortError`.

## Documentation

- Package reference: [pkg.go.dev](https://pkg.go.dev/github.com/valksor/typesafe-sdk-go)
- API wire contract: [TypeSafe API docs](https://docs.typesafe.ai/api)

## Testing

```sh
go test -race -cover ./...
```

Live integration tests are opt-in and read-only (they exercise `GET /v1/models`):

```sh
TYPESAFE_RUN_LIVE_TESTS=1 TYPESAFE_API_KEY=... go test -run Integration ./...
```

## Releases

This SDK tracks the same release version line as the official JavaScript and
Python SDKs. `Version`, the latest changelog entry, and the Git tag must all
match exactly — for example, release `0.6.0` with tag `v0.6.0`. See
[docs/changelog.md](docs/changelog.md).

CI tests Go 1.25 and 1.26. The publish workflow can be run manually as a dry
run; pushing the matching `vX.Y.Z` tag from the default branch creates the GitHub
Release, and the Git tag itself publishes the Go module version.

## License

[MIT](LICENSE)
