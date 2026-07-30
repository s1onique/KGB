// Package main provides the UVB-76 pprof memory leak lab.
//
// # Explicit Target-State Authentication
//
// This file implements P0-7: Make target-state authentication explicit.
// Authentication is resolved from the generated config, not the environment.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Sentinel errors for auth failures (P0-2: hierarchical)
// Parent: ErrAuthResolutionFailed
// Children: ErrAuthInputMissing, ErrAuthRejected, ErrAuthTransportFailed, ErrAuthMalformedResponse
var (
	ErrAuthResolutionFailed  = errors.New("auth resolution failed")
	ErrAuthInputMissing      = errors.New("auth input missing")
	ErrAuthRejected          = errors.New("authentication rejected")
	ErrAuthTransportFailed   = errors.New("auth transport failed")
	ErrAuthMalformedResponse = errors.New("auth malformed response")
)

// TargetStateAuthResolver resolves target-state authentication from the generated authority.
// P0-7: Authentication must be supplied explicitly, not read from environment.
type TargetStateAuthResolver interface {
	Resolve(ctx context.Context, authority *GeneratedLabAuthority) (TargetStateAuthInput, error)
}

// ProductionAuthResolver resolves authentication from the generated lab config.
// P0-7: Uses the auth config from the generated configuration and performs real login.
type ProductionAuthResolver struct {
	// Client allows injection for testing. P0-2: Will be copied before mutation.
	Client *http.Client
	// EphemeralPassword carries the plaintext credential (never persisted)
	// Set by labconfig.Generate() and consumed here, then cleared.
	// Best-effort bounded lifetime: cleared via defer immediately at function start.
	EphemeralPassword []byte
}

// authFailure creates a hierarchical auth error with both broad and precise identities.
// P0-2: All auth errors must satisfy errors.Is(err, ErrAuthResolutionFailed).
func authFailure(category, detail error) error {
	return errors.Join(
		ErrAuthResolutionFailed,
		category,
		detail,
	)
}

// Resolve resolves authentication by performing a real login to UVB-76.
// P0-2: All errors satisfy errors.Is(err, ErrAuthResolutionFailed).
// P0-2: Uses authFailure helper for hierarchical error construction.
func (r *ProductionAuthResolver) Resolve(ctx context.Context, authority *GeneratedLabAuthority) (TargetStateAuthInput, error) {
	// Install credential cleanup at function entry - covers ALL return paths
	if len(r.EphemeralPassword) > 0 {
		defer clearCredential(r.EphemeralPassword)
	}

	// P0-2: Validate context is non-nil
	if ctx == nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthInputMissing, errors.New("nil context"))
	}

	// Validate inputs
	if authority == nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthInputMissing, ErrGeneratedConfigNil)
	}

	if authority.Config == nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthInputMissing, ErrGeneratedConfigNil)
	}

	cfg := authority.Config

	// P0-2: Validate API base URL before joining path
	if authority.UVB76APIBaseURL == "" {
		return TargetStateAuthInput{}, authFailure(ErrAuthTransportFailed, errors.New("empty API base URL"))
	}

	username := cfg.Auth.Username
	if username == "" {
		return TargetStateAuthInput{}, authFailure(ErrAuthInputMissing, errors.New("empty username"))
	}

	// P0-7-fix: Ephemeral password is REQUIRED - no fallback to hash
	if len(r.EphemeralPassword) == 0 {
		return TargetStateAuthInput{}, authFailure(ErrAuthInputMissing, errors.New("ephemeral password required"))
	}

	// Build login URL
	loginURL, err := url.JoinPath(authority.UVB76APIBaseURL, "api", "v1", "auth", "login")
	if err != nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthTransportFailed, fmt.Errorf("build login URL: %v", err))
	}

	// Prepare login request body
	loginReq := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: username,
		Password: string(r.EphemeralPassword),
	}

	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthTransportFailed, fmt.Errorf("marshal login request: %v", err))
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(reqBody))
	if err != nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthTransportFailed, fmt.Errorf("create request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")

	// P0-2: Create private client - do not mutate injected client
	// If client is injected, copy it to avoid mutating caller's client
	var client *http.Client
	if r.Client != nil {
		clientCopy := *r.Client
		client = &clientCopy
	} else {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	// P0-2: Prevent redirect on auth endpoint - production auth is terminal
	// Redirects could leak credentials or cause session confusion
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Perform login
	resp, err := client.Do(req)
	if err != nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthTransportFailed, fmt.Errorf("request failed: %v", err))
	}
	defer resp.Body.Close()

	// P0-2: Read with explicit limit + 1 byte to detect oversize
	// Read up to MaxAuthBodyBytes + 1, reject if more
	limitReader := io.LimitReader(resp.Body, MaxAuthBodyBytes+1)
	body, err := io.ReadAll(limitReader)
	if err != nil {
		return TargetStateAuthInput{}, authFailure(ErrAuthMalformedResponse, fmt.Errorf("read response: %v", err))
	}

	// P0-2: Detect oversized body - read one extra byte beyond limit
	if len(body) > MaxAuthBodyBytes {
		return TargetStateAuthInput{}, authFailure(ErrAuthMalformedResponse, errors.New("response body exceeds maximum size"))
	}

	// Check response status - don't include body in error
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return TargetStateAuthInput{}, authFailure(ErrAuthRejected, fmt.Errorf("status=%d", resp.StatusCode))
	}

	// Extract session cookie
	cookieName, cookieValue := extractSessionCookie(resp.Cookies())
	if cookieName == "" {
		return TargetStateAuthInput{}, authFailure(ErrAuthMalformedResponse, errors.New("no session cookie in response"))
	}

	return TargetStateAuthInput{
		CookieName:  cookieName,
		CookieValue: cookieValue,
	}, nil
}

// MaxAuthBodyBytes is the maximum auth response body size to read.
// P0-2: Used to detect oversized responses.
const MaxAuthBodyBytes = 1024

// canonicalSessionCookieNames lists the only valid session cookie names.
// P0-2: Production session-cookie contract accepts ONLY these names.
var canonicalSessionCookieNames = []string{"session", "session_id"}

// extractSessionCookie extracts the canonical session cookie from HTTP cookies.
// P0-2: Binds cookie selection to production session-cookie contract.
// Forbidden: "token" or any other arbitrary name.
func extractSessionCookie(cookies []*http.Cookie) (name string, value string) {
	for _, c := range cookies {
		for _, validName := range canonicalSessionCookieNames {
			if c.Name == validName {
				// P0-2: Reject empty cookie values
				if c.Value == "" {
					return "", ""
				}
				return c.Name, c.Value
			}
		}
	}
	return "", ""
}

// TestAuthResolver is a deterministic resolver for testing.
type TestAuthResolver struct {
	CookieName  string
	CookieValue string
}

// Resolve returns the configured session cookie.
func (r *TestAuthResolver) Resolve(ctx context.Context, authority *GeneratedLabAuthority) (TargetStateAuthInput, error) {
	if r.CookieName == "" || r.CookieValue == "" {
		return TargetStateAuthInput{}, fmt.Errorf("test resolver: empty cookie name or value")
	}
	return TargetStateAuthInput{
		CookieName:  r.CookieName,
		CookieValue: r.CookieValue,
	}, nil
}

// NoopAuthResolver is a resolver that always fails.
type NoopAuthResolver struct{}

// Resolve always returns an error.
func (r *NoopAuthResolver) Resolve(ctx context.Context, authority *GeneratedLabAuthority) (TargetStateAuthInput, error) {
	return TargetStateAuthInput{}, fmt.Errorf("%w: noop resolver", ErrAuthInputMissing)
}

// ResolveTargetAuth is a convenience function to resolve authentication.
func ResolveTargetAuth(ctx context.Context, authority *GeneratedLabAuthority, ephemeralPassword []byte) (TargetStateAuthInput, error) {
	resolver := &ProductionAuthResolver{
		EphemeralPassword: ephemeralPassword,
	}
	return resolver.Resolve(ctx, authority)
}

// clearCredential zeroes the byte slice for best-effort memory clearing.
// Note: Go strings are immutable, so this only clears the byte slice.
// This provides BEST-EFFORT bounded lifetime, not guaranteed erasure.
func clearCredential(cred []byte) {
	for i := range cred {
		cred[i] = 0
	}
}
