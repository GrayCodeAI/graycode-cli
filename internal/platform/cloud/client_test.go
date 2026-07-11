package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecordUsageSendsAggregateEvent(t *testing.T) {
	var gotAuth string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/usage" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()
	c := New(Config{Endpoint: s.URL, DeviceToken: "hwc_test"})
	c.RecordUsage(context.Background(), UsageEvent{EventID: "event_0123456789", DeviceID: "device_0123456789", ProjectID: "project_0123456789", Capability: "hawk", OccurredAt: "2026-07-10T00:00:00Z"})
	if gotAuth != "Bearer hwc_test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestDisabledClientDoesNotSend(t *testing.T) {
	New(Config{}).RecordUsage(context.Background(), UsageEvent{})
}

func TestRecordDeliveryContextUsesDeviceScopedEndpoint(t *testing.T) {
	var gotAuth, gotPath string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()
	event := DeliveryContext{ProjectID: "project_0123456789", Branch: "main", CommitSHA: "abc123"}
	event.Repository.Provider, event.Repository.ExternalID, event.Repository.Name = "git", "hawk-eco", "GrayCodeAI/hawk-eco"
	New(Config{Endpoint: s.URL, DeviceToken: "hwc_test"}).RecordDeliveryContext(context.Background(), event)
	if gotPath != "/v1/delivery-context" || gotAuth != "Bearer hwc_test" {
		t.Fatalf("path/auth = %q/%q", gotPath, gotAuth)
	}
}

func TestRecordDeliveryContextIncludesCIRunAndDeployment(t *testing.T) {
	var body DeliveryContext
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()
	event := DeliveryContext{ProjectID: "project_0123456789"}
	event.Repository.Provider, event.Repository.ExternalID, event.Repository.Name = "github", "1", "GrayCodeAI/hawk"
	event.CIRun = &CIRunContext{Provider: "github", ExternalID: "run-1", Workflow: "test", Status: "succeeded"}
	event.Deployment = &DeploymentContext{Provider: "github", ExternalID: "deploy-1", Environment: "production", Status: "succeeded"}
	New(Config{Endpoint: s.URL, DeviceToken: "hwc_test"}).RecordDeliveryContext(context.Background(), event)
	if body.CIRun == nil || body.CIRun.ExternalID != "run-1" {
		t.Fatalf("CI run = %+v", body.CIRun)
	}
	if body.Deployment == nil || body.Deployment.Environment != "production" {
		t.Fatalf("deployment = %+v", body.Deployment)
	}
}
