package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPingRoute(t *testing.T) {
	h := newRouter("test-server")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["message"], "pong"; got != want {
		t.Errorf("message = %v, want %v", got, want)
	}
	if got, want := body["server"], "test-server"; got != want {
		t.Errorf("server = %v, want %v", got, want)
	}
	if _, ok := body["time"]; !ok {
		t.Errorf("response missing \"time\" field: %v", body)
	}
}

func TestHealthzRoute(t *testing.T) {
	h := newRouter("test-server")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	h := newRouter("test-server")

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (404)", rec.Code, http.StatusNotFound)
	}
}
