// Package server provides the HTTP server for UVB-76.
package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/state"
)

// webContent is the embedded web filesystem, set by main.go.
var webContent fs.FS

// SetWebFS sets the embedded web filesystem.
// This must be called before the server starts.
func SetWebFS(fs fs.FS) {
	webContent = fs
}

// Server is the main HTTP server for UVB-76.
type Server struct {
	cfg       *config.Config
	state     *state.Manager
	client    *scraper.Client
	listener  *config.ListenConfig
	sessions  *auth.SessionStore
	devMode   bool
	server    *http.Server
	wg        sync.WaitGroup
	startedAt time.Time // captured once at construction
}

// NewServer creates a new server (HTTPS in production, HTTP in dev mode).
func NewServer(cfg *config.Config, st *state.Manager, client *scraper.Client, devMode bool) *Server {
	s := &Server{
		cfg:       cfg,
		state:     st,
		client:    client,
		listener:  &cfg.Listen,
		devMode:   devMode,
		startedAt: time.Now().UTC(), // captured once at construction
	}

	// Initialize session store with a secret key (in production, use environment variable)
	// For now, we use a static secret - in production this should be configurable
	s.sessions = auth.NewSessionStore("uvb76-session-secret-change-in-production")

	return s
}

// Start begins the server (HTTPS in production, HTTP in dev mode).
func (s *Server) Start() error {
	router := mux.NewRouter()

	// Public endpoints (no auth required)
	router.Handle("/api/v1/healthz", http.HandlerFunc(s.handleHealthz)).Methods(http.MethodGet)
	// Status endpoint is public - it only exposes telemetry, no sensitive data
	router.Handle("/api/v1/status", http.HandlerFunc(s.handleStatus)).Methods(http.MethodGet)

	// Auth endpoints (public, but create/clear sessions)
	router.Handle("/api/v1/auth/login", http.HandlerFunc(s.handleLogin)).Methods(http.MethodPost)
	router.Handle("/api/v1/auth/logout", http.HandlerFunc(s.handleLogout)).Methods(http.MethodPost)
	router.Handle("/api/v1/auth/check", http.HandlerFunc(s.handleAuthCheck)).Methods(http.MethodGet)

	// Protected API endpoints - use session auth
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(s.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(s.handleTargets)).Methods(http.MethodGet)
	protected.Handle("/targets/{id}/snapshot", http.HandlerFunc(s.handleTargetSnapshot)).Methods(http.MethodGet)

	// Latency API endpoints
	protected.Handle("/targets/{id}/latency", http.HandlerFunc(s.handleTargetLatency)).Methods(http.MethodGet)
	protected.Handle("/targets/{id}/latency/samples", http.HandlerFunc(s.handleTargetLatencySamples)).Methods(http.MethodGet)
	protected.Handle("/latency", http.HandlerFunc(s.handleAllLatency)).Methods(http.MethodGet)
	protected.Handle("/latency/series", http.HandlerFunc(s.handleTargetLatencySeries)).Methods(http.MethodGet)
	protected.Handle("/latency/spikes", http.HandlerFunc(s.handleTargetLatencySpikes)).Methods(http.MethodGet)

	// Diagnostics API endpoints
	protected.Handle("/diagnostics/capture-cooldown", http.HandlerFunc(s.handleCaptureCooldownDiagnostics)).Methods(http.MethodGet)
	protected.Handle("/diagnostics/cooldown/anchors/{peer_name}", http.HandlerFunc(s.handleGetCooldownAnchorForPeer)).Methods(http.MethodGet)

	// Capture lookup endpoints
	protected.Handle("/captures/{capture_id}/anchor", http.HandlerFunc(s.handleGetAnchorCapture)).Methods(http.MethodGet)

	// Web UI - serve from embedded filesystem
	// Assets are served from /assets/* path
	router.PathPrefix("/assets/").Handler(
		http.StripPrefix("/assets/", serveWebDir("assets")),
	)
	// Root and index.html serve the SPA
	router.Handle("/", serveWebFile("index.html")).Methods(http.MethodGet)
	router.Handle("/index.html", serveWebFile("index.html")).Methods(http.MethodGet)
	// SPA fallback for any unmatched path
	router.PathPrefix("/").HandlerFunc(s.handleSPA)

	s.server = &http.Server{
		Addr:    s.listener.Addr,
		Handler: router,
	}

	if s.devMode {
		log.Printf("Starting HTTP dev server on %s", s.listener.Addr)
		if err := s.server.ListenAndServe(); err != nil {
			return err
		}
	} else {
		log.Printf("Starting HTTPS server on %s", s.listener.Addr)
		if err := s.server.ListenAndServeTLS(s.listener.TLSCertFile, s.listener.TLSKeyFile); err != nil {
			return err
		}
	}

	return nil
}

// serveWebFile returns a handler that serves a specific file from the embedded web content.
func serveWebFile(filename string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := webContent.Open(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer content.Close()

		// Set content type based on file extension
		contentType := contentTypeFor(path.Ext(filename))
		w.Header().Set("Content-Type", contentType)

		// Copy content to response - fs.File doesn't implement io.ReadSeeker
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content)
	})
}

// serveWebDir returns a handler that serves files from a subdirectory.
// The outer route already strips /assets/, so we serve the subdir contents directly.
func serveWebDir(subdir string) http.Handler {
	subFS, err := fs.Sub(webContent, subdir)
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(subFS))
}

// handleSPA serves the SPA index.html for any unmatched routes.
// This enables client-side routing.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	content, err := webContent.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer content.Close()

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content)
}

// contentTypeFor returns the MIME type for a file extension.
func contentTypeFor(ext string) string {
	switch strings.ToLower(ext) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// sessionAuthMw returns the session authentication middleware.
func (s *Server) sessionAuthMw() func(http.Handler) http.Handler {
	return auth.SessionAuthMiddleware(s.sessions)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}

// handleHealthz returns the server health status.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleLogin processes login requests.
// POST /api/v1/auth/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		auth.JSONError(w, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}

	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		auth.JSONError(w, "invalid_request", http.StatusBadRequest)
		return
	}

	// Validate credentials using constant-time comparison for both username and password
	if !auth.AuthenticateFull(req.Username, req.Password, s.cfg.Auth.Username, s.cfg.Auth.PasswordSHA256) {
		// Return clean JSON error, no WWW-Authenticate
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(auth.LoginResponse{
			Success: false,
			Error:   "invalid_credentials",
		})
		return
	}

	// Generate session token
	token, err := s.sessions.GenerateToken(req.Username)
	if err != nil {
		auth.JSONError(w, "session_creation_failed", http.StatusInternalServerError)
		return
	}

	// Set session cookie (Secure=true in production, false in dev mode)
	auth.SetSessionCookie(w, token, !s.devMode)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(auth.LoginResponse{
		Success: true,
	})
}

// handleLogout clears the session.
// POST /api/v1/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		auth.JSONError(w, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get and invalidate session token
	token := auth.GetSessionToken(r)
	if token != "" {
		s.sessions.InvalidateToken(token)
	}

	// Clear session cookie
	auth.ClearSessionCookie(w, !s.devMode)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// handleAuthCheck verifies if the current session is valid.
// GET /api/v1/auth/check
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := auth.GetSessionToken(r)
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Token from GetSessionToken is already base64-encoded, which is the storage key
	session, ok := s.sessions.ValidateToken(token)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"username":      session.Username,
	})
}

// TargetInfo represents a target with its effective probe URL and diagnostic capture URL for debugging.
type TargetInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	BaseURL           string `json:"base_url"`
	ProbeURL          string `json:"probe_url,omitempty"`
	EffectiveProbeURL string `json:"effective_probe_url"`
	Enabled           bool   `json:"enabled"`
	// Diagnostic capture info is empty/omitted if no diagnostics peer targets this target.
	DiagnosticPeerName  string `json:"diagnostic_peer_name,omitempty"`
	DiagnosticBaseURL   string `json:"diagnostic_base_url,omitempty"`
	EffectiveCaptureURL string `json:"effective_capture_url,omitempty"`
}

// handleTargets returns the list of configured targets with effective probe URLs and diagnostic capture URLs.
// NOTE: This handler does NOT call net/url.Parse - all diagnostic URLs are precomputed
// at config load time via DiagnosticsConfig.PrecomputeCaptureURLs().
func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Build target->diagPeer mapping
	targetToPeer := s.cfg.Diagnostics.TargetToDiagPeers()

	targets := make([]TargetInfo, len(s.cfg.Targets))
	for i, t := range s.cfg.Targets {
		info := TargetInfo{
			ID:                t.ID,
			Name:              t.Name,
			BaseURL:           t.BaseURL,
			ProbeURL:          t.ProbeURL,
			EffectiveProbeURL: config.TargetProbeURL(&t),
			Enabled:           t.Enabled,
		}

		// Add diagnostic capture info if this target has a diagnostics peer.
		// Uses precomputed EffectiveCaptureURL - no net/url.Parse at request time.
		if peer, ok := targetToPeer[t.ID]; ok {
			info.DiagnosticPeerName = peer.Name
			info.DiagnosticBaseURL = peer.BaseURL
			info.EffectiveCaptureURL = peer.EffectiveCaptureURL
		}

		targets[i] = info
	}
	json.NewEncoder(w).Encode(targets)
}

// handleTargetSnapshot returns the latest snapshot for a specific target.
func (s *Server) handleTargetSnapshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetID := vars["id"]

	// Find target in config
	var found bool
	for _, t := range s.cfg.Targets {
		if t.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "target_not_found"})
		return
	}

	snap := s.state.GetSnapshot(targetID)
	if snap == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no_snapshot_available"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// =============================================================================
// Cooldown Diagnostics
// =============================================================================

// CaptureCooldownDiagnostics represents the diagnostics output for cooldown state.
type CaptureCooldownDiagnostics struct {
	ServerStartedAt    string                                 `json:"server_started_at"`
	CurrentTime        string                                 `json:"current_time"`
	CooldownAnchors    map[string]state.CaptureCooldownAnchor `json:"cooldown_anchors"`
	ActiveCooldownKeys []string                               `json:"active_cooldown_keys"`
	TotalCaptures      int                                    `json:"total_captures"`
}

// handleCaptureCooldownDiagnostics returns diagnostic information about cooldown state.
// This is an admin-only read-only endpoint for debugging cooldown issues.
func (s *Server) handleCaptureCooldownDiagnostics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	captureStore := s.state.GetCaptureStore()
	anchors := captureStore.GetAllLastCaptureAnchors()

	// Build list of active cooldown keys (peers with anchors)
	activeKeys := make([]string, 0, len(anchors))
	for peerName := range anchors {
		activeKeys = append(activeKeys, peerName)
	}

	diagnostics := CaptureCooldownDiagnostics{
		ServerStartedAt:    s.startedAt.Format(time.RFC3339),
		CurrentTime:        time.Now().UTC().Format(time.RFC3339),
		CooldownAnchors:    anchors,
		ActiveCooldownKeys: activeKeys,
		TotalCaptures:      captureStore.Count(),
	}

	json.NewEncoder(w).Encode(diagnostics)
}
