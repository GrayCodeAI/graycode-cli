package mcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestParseSSEResponseAllowsLargeEvent(t *testing.T) {
	payload := strings.Repeat("x", 128<<10)
	body := strings.NewReader(`data: {"jsonrpc":"2.0","id":7,"result":"` + payload + `"}` + "\n\n")
	server := &HTTPServer{}

	result, err := server.parseSSEResponse(body, 7)
	if err != nil {
		t.Fatalf("parseSSEResponse returned error: %v", err)
	}
	if !strings.Contains(string(result), payload[:100]) {
		t.Fatal("parseSSEResponse did not return the large result")
	}
}

func TestParseSSEResponseReportsScannerError(t *testing.T) {
	body := strings.NewReader("data: " + strings.Repeat("x", maxMCPResponseSize+1))
	server := &HTTPServer{}

	if _, err := server.parseSSEResponse(body, 1); err == nil {
		t.Fatal("parseSSEResponse accepted an oversized event")
	}
}

func TestHTTPCallRejectsOversizedResponse(t *testing.T) {
	server := &HTTPServer{
		URL:  "http://mcp.invalid",
		Type: "http",
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxMCPResponseSize+1))),
				Header:     make(http.Header),
			}, nil
		})},
	}

	if _, err := server.Call(context.Background(), "test", nil); err == nil {
		t.Fatal("Call accepted an oversized response")
	}
}
