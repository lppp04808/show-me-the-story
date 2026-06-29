package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withTestHTTPClient(t *testing.T, rt http.RoundTripper, fn func()) {
	t.Helper()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = rt
	defer func() {
		http.DefaultTransport = oldTransport
	}()
	fn()
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCallAPIMessagesReturnsErrorAfterInterruptedStream(t *testing.T) {
	apiCfg := &APIConfig{
		BaseURL:            "http://example.com",
		Model:              "test-model",
		HTTPTimeoutSeconds: 5,
		UseStream:          true,
	}
	messages := []Message{{Role: "user", Content: "hi"}}

	calls := 0
	withTestHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"title\\\":\"}}]}\n"), nil
	}), func() {
		got, err := CallAPIMessages(context.Background(), apiCfg, messages)
		if err == nil {
			t.Fatal("CallAPIMessages() error = nil, want non-nil")
		}
		if got != `{"title":` {
			t.Fatalf("CallAPIMessages() = %q, want partial content", got)
		}
		if !strings.Contains(err.Error(), "缺少 [DONE]") {
			t.Fatalf("CallAPIMessages() error = %v, want missing [DONE]", err)
		}
	})

	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", calls)
	}
}

func TestCallAPIMessagesUsesSyncWhenStreamDisabled(t *testing.T) {
	apiCfg := &APIConfig{
		BaseURL:            "http://example.com",
		Model:              "test-model",
		HTTPTimeoutSeconds: 5,
		UseStream:          false,
	}
	messages := []Message{{Role: "user", Content: "hi"}}

	calls := 0
	withTestHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost {
			return nil, fmt.Errorf("unexpected method %s", req.Method)
		}
		return jsonResponse(`{"choices":[{"message":{"role":"assistant","content":"sync-ok"}}]}`), nil
	}), func() {
		got, err := CallAPIMessages(context.Background(), apiCfg, messages)
		if err != nil {
			t.Fatalf("CallAPIMessages() error = %v", err)
		}
		if got != "sync-ok" {
			t.Fatalf("CallAPIMessages() = %q, want %q", got, "sync-ok")
		}
	})

	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", calls)
	}
}

func TestCallAPIStreamFallsBackToSyncWhenStreamDisabled(t *testing.T) {
	apiCfg := &APIConfig{
		BaseURL:            "http://example.com",
		Model:              "test-model",
		HTTPTimeoutSeconds: 5,
		UseStream:          false,
	}

	withTestHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"choices":[{"message":{"role":"assistant","content":"non-stream"}}]}`), nil
	}), func() {
		got, err := CallAPIStream(context.Background(), apiCfg, "sys", "user", func(string) {
			t.Fatal("onChunk should not be called when stream is disabled")
		})
		if err != nil {
			t.Fatalf("CallAPIStream() error = %v", err)
		}
		if got != "non-stream" {
			t.Fatalf("CallAPIStream() = %q, want %q", got, "non-stream")
		}
	})
}

func TestCallAPIStreamMessagesReturnsErrorOnInterruptedStream(t *testing.T) {
	apiCfg := &APIConfig{
		BaseURL:            "http://example.com",
		Model:              "test-model",
		HTTPTimeoutSeconds: 5,
	}
	messages := []Message{{Role: "user", Content: "hi"}}

	withTestHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n"), nil
	}), func() {
		got, err := CallAPIStreamMessages(context.Background(), apiCfg, messages, nil)
		if err == nil {
			t.Fatal("CallAPIStreamMessages() error = nil, want non-nil")
		}
		if got != "partial" {
			t.Fatalf("CallAPIStreamMessages() = %q, want partial content", got)
		}
		if !strings.Contains(err.Error(), "缺少 [DONE]") {
			t.Fatalf("CallAPIStreamMessages() error = %v, want missing [DONE]", err)
		}
	})
}

func TestCallAPIMessagesKeepsFatalStreamError(t *testing.T) {
	apiCfg := &APIConfig{
		BaseURL:            "http://example.com",
		Model:              "test-model",
		HTTPTimeoutSeconds: 5,
	}
	messages := []Message{{Role: "user", Content: "hi"}}

	calls := 0
	withTestHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unauthorized")),
		}, nil
	}), func() {
		_, err := CallAPIMessages(context.Background(), apiCfg, messages)
		if err == nil {
			t.Fatal("CallAPIMessages() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "状态码: 401") {
			t.Fatalf("CallAPIMessages() error = %v, want 401", err)
		}
	})

	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", calls)
	}
}

func TestParseOutlineResponseRejectsTruncatedJSON(t *testing.T) {
	_, err := parseOutlineResponse(`{"title":"broken"`)
	if err == nil {
		t.Fatal("parseOutlineResponse() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("parseOutlineResponse() error = %v, want truncated JSON error", err)
	}
}