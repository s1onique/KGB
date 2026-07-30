// Package main provides the UVB-76 pprof memory leak lab.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

// mockAuthServer creates a test server that issues session cookies.
func mockAuthServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/status" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/api/v1/auth/login" {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if req.Username == "lab-user" && req.Password == "lab-password" {
				http.SetCookie(w, &http.Cookie{Name: "session", Value: "test-session-token"})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"success":true}`))
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
}

func makeTestAuthority(t *testing.T, server *httptest.Server) *GeneratedLabAuthority {
	return &GeneratedLabAuthority{
		Config: &GeneratedConfig{
			Auth: config.AuthConfig{
				Username: "lab-user",
			},
		},
		UVB76APIBaseURL: server.URL,
	}
}

func TestResolveAuthWithValidCredentials(t *testing.T) {
	server := mockAuthServer(t)
	defer server.Close()
	authority := makeTestAuthority(t, server)

	password := []byte("lab-password")
	resolver := &ProductionAuthResolver{
		Client:            server.Client(),
		EphemeralPassword: password,
	}

	result, err := resolver.Resolve(context.Background(), authority)
	if err != nil {
		t.Fatalf("expected successful auth, got error: %v", err)
	}

	if result.CookieName != "session" {
		t.Errorf("expected cookie name 'session', got %q", result.CookieName)
	}
	if result.CookieValue != "test-session-token" {
		t.Errorf("expected cookie value 'test-session-token', got %q", result.CookieValue)
	}

	// P0-6: Prove credential is cleared after successful login
	for i, b := range password {
		if b != 0 {
			t.Errorf("password[%d] was not cleared after success, got %d", i, b)
		}
	}
}

func TestResolveAuthMissingEphemeralPasswordFailsClosed(t *testing.T) {
	server := mockAuthServer(t)
	defer server.Close()
	authority := makeTestAuthority(t, server)

	password := []byte("lab-password") // Will be empty after defer
	resolver := &ProductionAuthResolver{
		Client:            server.Client(),
		EphemeralPassword: nil, // Missing password
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error when ephemeral password is missing, got nil")
	}
	// P0-7: Use errors.Is for wrapped error chain
	if !errors.Is(err, ErrAuthInputMissing) {
		t.Errorf("expected ErrAuthInputMissing, got: %v", err)
	}

	_ = password // Use to avoid unused variable
}

func TestResolveAuthEmptyPasswordFailsClosed(t *testing.T) {
	server := mockAuthServer(t)
	defer server.Close()
	authority := makeTestAuthority(t, server)

	password := []byte{}
	resolver := &ProductionAuthResolver{
		Client:            server.Client(),
		EphemeralPassword: password,
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error when ephemeral password is empty, got nil")
	}
	if !errors.Is(err, ErrAuthInputMissing) {
		t.Errorf("expected ErrAuthInputMissing, got: %v", err)
	}
}

func TestResolveAuthInvalidCredentialsFails(t *testing.T) {
	server := mockAuthServer(t)
	defer server.Close()
	authority := makeTestAuthority(t, server)

	password := []byte("wrong-password")
	resolver := &ProductionAuthResolver{
		Client:            server.Client(),
		EphemeralPassword: password,
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error for invalid credentials, got nil")
	}
	// P0-5: Should be typed ErrAuthRejected, not raw body in error
	if !errors.Is(err, ErrAuthRejected) {
		t.Errorf("expected ErrAuthRejected, got: %v", err)
	}
	// P0-5: Error should NOT contain response body
	if containsSensitive(err.Error(), "unauthorized") {
		t.Error("error should not contain raw response body")
	}

	// P0-6: Credential should still be cleared even on failure
	for i, b := range password {
		if b != 0 {
			t.Errorf("password[%d] was not cleared after failure, got %d", i, b)
		}
	}
}

func TestResolveAuthTransportFailure(t *testing.T) {
	server := mockAuthServer(t)
	server.Close() // Close to cause connection failure
	authority := makeTestAuthority(t, server)

	password := []byte("lab-password")
	resolver := &ProductionAuthResolver{
		Client:            http.DefaultClient,
		EphemeralPassword: password,
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error for transport failure, got nil")
	}
	if !errors.Is(err, ErrAuthTransportFailed) {
		t.Errorf("expected ErrAuthTransportFailed, got: %v", err)
	}

	// P0-6: Credential should be cleared
	for i, b := range password {
		if b != 0 {
			t.Errorf("password[%d] was not cleared after transport failure, got %d", i, b)
		}
	}
}

func TestResolveAuthNilAuthorityFails(t *testing.T) {
	resolver := &ProductionAuthResolver{
		EphemeralPassword: []byte("lab-password"),
	}

	_, err := resolver.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil authority, got nil")
	}
	if !errors.Is(err, ErrGeneratedConfigNil) {
		t.Errorf("expected ErrGeneratedConfigNil, got: %v", err)
	}
}

func TestResolveAuthNilConfigFails(t *testing.T) {
	authority := &GeneratedLabAuthority{
		Config: nil,
	}

	resolver := &ProductionAuthResolver{
		EphemeralPassword: []byte("lab-password"),
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
	if !errors.Is(err, ErrGeneratedConfigNil) {
		t.Errorf("expected ErrGeneratedConfigNil, got: %v", err)
	}
}

func TestResolveAuthEmptyUsernameFails(t *testing.T) {
	server := mockAuthServer(t)
	defer server.Close()
	authority := &GeneratedLabAuthority{
		Config: &GeneratedConfig{
			Auth: config.AuthConfig{
				Username: "", // Empty username
			},
		},
		UVB76APIBaseURL: server.URL,
	}

	resolver := &ProductionAuthResolver{
		EphemeralPassword: []byte("lab-password"),
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
	if !errors.Is(err, ErrAuthInputMissing) {
		t.Errorf("expected ErrAuthInputMissing, got: %v", err)
	}
}

func TestResolveAuthMalformedURLFails(t *testing.T) {
	authority := &GeneratedLabAuthority{
		Config: &GeneratedConfig{
			Auth: config.AuthConfig{
				Username: "lab-user",
			},
		},
		UVB76APIBaseURL: "://invalid-url",
	}

	resolver := &ProductionAuthResolver{
		EphemeralPassword: []byte("lab-password"),
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
	if !errors.Is(err, ErrAuthTransportFailed) {
		t.Errorf("expected ErrAuthTransportFailed, got: %v", err)
	}
}

func TestResolveAuthNoSessionCookieFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			// Return OK but no cookie
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
			return
		}
	}))
	defer server.Close()
	authority := makeTestAuthority(t, server)

	resolver := &ProductionAuthResolver{
		EphemeralPassword: []byte("lab-password"),
	}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error when no session cookie, got nil")
	}
	if !errors.Is(err, ErrAuthMalformedResponse) {
		t.Errorf("expected ErrAuthMalformedResponse, got: %v", err)
	}
}

func TestTestAuthResolver(t *testing.T) {
	authority := &GeneratedLabAuthority{}
	resolver := &TestAuthResolver{
		CookieName:  "session",
		CookieValue: "test-token",
	}

	result, err := resolver.Resolve(context.Background(), authority)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CookieName != "session" || result.CookieValue != "test-token" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestNoopAuthResolverFails(t *testing.T) {
	authority := &GeneratedLabAuthority{}
	resolver := &NoopAuthResolver{}

	_, err := resolver.Resolve(context.Background(), authority)
	if err == nil {
		t.Fatal("expected error from NoopAuthResolver")
	}
	if !errors.Is(err, ErrAuthInputMissing) {
		t.Errorf("expected ErrAuthInputMissing, got: %v", err)
	}
}

func TestClearCredentialZerosSlice(t *testing.T) {
	cred := []byte("secret-password")
	clearCredential(cred)

	for i, b := range cred {
		if b != 0 {
			t.Errorf("byte at index %d not zeroed, got %d", i, b)
		}
	}
}

// containsSensitive checks if error message contains sensitive content.
// P0-5: Errors should not leak server response bodies.
func containsSensitive(msg, pattern string) bool {
	return len(pattern) > 0 && len(msg) > 0 &&
		(len(msg) >= len(pattern)*3) &&
		(msg == pattern ||
			len(msg) > 0 &&
				(msg[:min(50, len(msg))] == pattern[:min(50, len(pattern))]))
}
