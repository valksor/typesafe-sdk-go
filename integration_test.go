package typesafe_test

import (
	"context"
	"os"
	"testing"
	"time"

	typesafe "github.com/valksor/typesafe-sdk-go"
)

func TestIntegrationListModels(t *testing.T) {
	if os.Getenv("TYPESAFE_RUN_LIVE_TESTS") != "1" || os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("set TYPESAFE_RUN_LIVE_TESTS=1 and TYPESAFE_API_KEY to run live tests")
	}
	client, err := typesafe.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	response, err := client.Models.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(response.Models) == 0 {
		t.Fatal("List returned no models")
	}
}
