// Package server provides the HTTP server for UVB-76.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/state"
)

//go:embed admin.html
var adminFS embed.FS

var adminTemplate = template.Must(template.ParseFS(adminFS, "admin.html"))

// Server is the main HTTP server for UVB-76.
type Server struct {
	cfg        *config.Config
	state      *state.Manager
	client     *scraper.Client
	listener   *config.ListenConfig
	sessions   *auth.SessionStore
	devMode    bool
	server     *http.Server
	wg         sync.WaitGroup
}

// NewServer creates a new server (HTTPS in production, HTTP in dev mode).
func NewServer(cfg *config.Config, st *state.Manager, client *scraper.Client, devMode bool) *Server {
	s := &Server{
		cfg:      cfg,
		state:    st,
		client:   client,
		listener: &cfg.Listen,
		devMode:  devMode,
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

	// Admin UI - served without Basic Auth challenge
	// Unauthenticated users see the login form; authenticated users see the dashboard
	router.Handle("/", http.HandlerFunc(s.handleAdmin)).Methods(http.MethodGet)
	router.Handle("/index.html", http.HandlerFunc(s.handleAdmin)).Methods(http.MethodGet)
	// SPA fallback - serve admin for any other path
	router.PathPrefix("/").HandlerFunc(s.handleAdmin).Methods(http.MethodGet)

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
		"username":       session.Username,
	})
}

// handleTargets returns the list of configured targets.
func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.cfg.Targets)
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

// handleAdmin serves the embedded admin HTML page.
// This is the SPA entry point - it serves the app shell regardless of auth state.
// The frontend JavaScript handles showing the login form or dashboard.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	adminTemplate.Execute(w, nil)
}
