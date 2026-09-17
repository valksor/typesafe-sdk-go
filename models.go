package typesafe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Model describes a model or alias available to the authenticated account.
type Model struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}

// ModelsResponse contains the account's available models.
type ModelsResponse struct {
	Models []Model
	Meta   ResponseMeta
}

// ModelsService accesses the Models API resource.
type ModelsService struct{ client *Client }

// List returns the models available to the account.
func (s *ModelsService) List(ctx context.Context, options ...RequestOptions) (*ModelsResponse, error) {
	data, meta, err := s.client.request(ctx, http.MethodGet, "/v1/models", nil, oneRequestOptions(options))
	if err != nil {
		return nil, err
	}
	var wire struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, &ResponseValidationError{Cause: fmt.Errorf("decode models response: %w", err), Meta: meta, Body: append([]byte(nil), data...)}
	}
	if wire.Models == nil {
		return nil, &ResponseValidationError{Cause: fmt.Errorf("missing models: %w", ErrInvalidResponse), Meta: meta, Body: append([]byte(nil), data...)}
	}
	return &ModelsResponse{Models: wire.Models, Meta: meta}, nil
}

// ListModels is a convenience alias for Client.Models.List.
func (c *Client) ListModels(ctx context.Context, options ...RequestOptions) (*ModelsResponse, error) {
	return c.Models.List(ctx, options...)
}
