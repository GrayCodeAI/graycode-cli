package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/session"
)

func TestGetSessionGraphProjectsBoundedRequest(t *testing.T) {
	srv := New(Config{APIKey: "secret"}, nil)
	var captured GraphRequest
	srv.SetGraphFactory(func(_ context.Context, req GraphRequest) (executiongraph.Export, error) {
		captured = req
		return executiongraph.Export{
			SchemaVersion: executiongraph.SchemaVersion,
			GeneratedAt:   req.GeneratedAt,
		}, nil
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/sessions/session-1/graph?repository=hawk&swift_checkpoint=012345abcdef&swift_checkpoint=fedcba987654",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	srv.mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if captured.SessionID != "session-1" || captured.RepositoryID != "hawk" {
		t.Fatalf("captured request = %#v", captured)
	}
	wantCheckpoints := []string{"012345abcdef", "fedcba987654"}
	if !reflect.DeepEqual(captured.SwiftCheckpointIDs, wantCheckpoints) {
		t.Fatalf("checkpoints = %#v, want %#v", captured.SwiftCheckpointIDs, wantCheckpoints)
	}
	if captured.GeneratedAt.IsZero() || captured.GeneratedAt.Location() != time.UTC {
		t.Fatalf("generated_at = %v, want non-zero UTC", captured.GeneratedAt)
	}

	var export executiongraph.Export
	if err := json.NewDecoder(response.Body).Decode(&export); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if export.SchemaVersion != executiongraph.SchemaVersion {
		t.Fatalf("schema = %q, want %q", export.SchemaVersion, executiongraph.SchemaVersion)
	}
}

func TestGetSessionGraphRequiresAuthentication(t *testing.T) {
	srv := New(Config{APIKey: "secret"}, nil)
	srv.SetGraphFactory(func(context.Context, GraphRequest) (executiongraph.Export, error) {
		t.Fatal("factory must not run for an unauthorized request")
		return executiongraph.Export{}, nil
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/graph", nil)
	srv.mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestGetSessionGraphRejectsInvalidInputBeforeFactory(t *testing.T) {
	srv := New(Config{}, nil)
	srv.SetGraphFactory(func(context.Context, GraphRequest) (executiongraph.Export, error) {
		t.Fatal("factory must not run for invalid input")
		return executiongraph.Export{}, nil
	})

	tests := []struct {
		name string
		url  string
		code string
	}{
		{
			name: "session",
			url:  "/v1/sessions/bad%24id/graph",
			code: "invalid_id",
		},
		{
			name: "repository control character",
			url:  "/v1/sessions/session-1/graph?repository=hawk%0Aother",
			code: "invalid_repository",
		},
		{
			name: "checkpoint",
			url:  "/v1/sessions/session-1/graph?swift_checkpoint=ABC",
			code: "invalid_swift_checkpoint",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.url, nil)
			srv.mux.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
			var body ErrorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Code, test.code)
			}
		})
	}
}

func TestGetSessionGraphUnavailableAndNotFound(t *testing.T) {
	srv := New(Config{}, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/graph", nil)
	srv.mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured status = %d, want 503", response.Code)
	}

	srv.SetGraphFactory(func(context.Context, GraphRequest) (executiongraph.Export, error) {
		return executiongraph.Export{}, errors.Join(errors.New("wrapped"), session.ErrNotFound)
	})
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/graph", nil)
	srv.mux.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d, want 404", response.Code)
	}
}
