package typesafe

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultClientPoolsConnections(t *testing.T) {
	t.Parallel()
	client, err := NewClient(Config{APIKey: "test"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", client.httpClient.Transport)
	}
	if transport == http.DefaultTransport {
		t.Fatal("default transport must be cloned, not shared")
	}
	if transport.MaxIdleConns != 100 || transport.MaxIdleConnsPerHost != 100 || transport.MaxConnsPerHost != 100 || transport.IdleConnTimeout != 30*time.Second {
		t.Fatalf("unexpected pool: idle=%d perHost=%d conns=%d expiry=%v", transport.MaxIdleConns, transport.MaxIdleConnsPerHost, transport.MaxConnsPerHost, transport.IdleConnTimeout)
	}
}

func TestCallerHTTPClientIsUsedUntouched(t *testing.T) {
	t.Parallel()
	custom := &http.Client{}
	client, err := NewClient(Config{APIKey: "test", HTTPClient: custom})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.httpClient != custom || custom.Transport != nil {
		t.Fatalf("caller client was replaced or modified: %#v", custom)
	}
}
