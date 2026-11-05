package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHello(t *testing.T) {
	response := httptest.NewRecorder()
	hello(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != "Hello, world!\n" {
		t.Fatalf("body = %q, want Hello, world! followed by newline", body)
	}
}
