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
	cfg      *config.Config
	state    *state.Manager
	client   *scraper.Client
	listener *config.ListenConfig
	authMw   func(http.Handler) http.Handler
	devMode  bool
	server   *http.Server
	wg       sync.WaitGroup
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

	// Set up Basic Auth middleware
	s.authMw = auth.BasicAuthMiddleware(cfg.Auth.Username, cfg.Auth.PasswordSHA256)

	return s
}

// Start begins the server (HTTPS in production, HTTP in dev mode).
func (s *Server) Start() error {
	router := mux.NewRouter()

	// Public endpoints
	router.Handle("/api/v1/healthz", http.HandlerFunc(s.handleHealthz)).Methods(http.MethodGet)

	// Protected API endpoints
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(s.authMw)
	protected.Handle("/targets", http.HandlerFunc(s.handleTargets)).Methods(http.MethodGet)
	protected.Handle("/targets/{id}/snapshot", http.HandlerFunc(s.handleTargetSnapshot)).Methods(http.MethodGet)

	// Protected admin UI
	adminRouter := router.PathPrefix("").Subrouter()
	adminRouter.Use(s.authMw)
	adminRouter.Handle("/", http.HandlerFunc(s.handleAdmin)).Methods(http.MethodGet)

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
		http.Error(w, "Target not found", http.StatusNotFound)
		return
	}

	snap := s.state.GetSnapshot(targetID)
	if snap == nil {
		http.Error(w, "No snapshot available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// handleAdmin serves the embedded admin HTML page.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	adminTemplate.Execute(w, nil)
}
