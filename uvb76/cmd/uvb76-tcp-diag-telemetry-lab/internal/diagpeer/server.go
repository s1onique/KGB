// Package diagpeer provides a hermetic diagnostic peer for testing.
package diagpeer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TovarischStatusResponse is the expected response format from tovarisch.
type TovarischStatusResponse struct {
	Service  string               `json:"service"`
	Version  string               `json:"version"`
	NodeID   string               `json:"node_id"`
	Status   string               `json:"status"`
	NetworkDiag *NetworkDiagData  `json:"network_diag,omitempty"`
}

// NetworkDiagData represents network diagnostic data.
type NetworkDiagData struct {
	StartedAt   string              `json:"started_at"`
	Status      string              `json:"status"`
	Interfaces  []InterfaceDiagData `json:"interfaces"`
	Routes      []RouteDiagData    `json:"routes"`
	UnderlayTCP []TcpSocketDiagData `json:"underlay_tcp"`
	Events      []DiagEventData    `json:"events"`
}

type InterfaceDiagData struct {
	Name      string `json:"name"`
	OperState string `json:"operstate"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
}

type RouteDiagData struct {
	Target    string `json:"target"`
	Interface string `json:"interface"`
	Source    string `json:"source"`
	Status    string `json:"status"`
}

type TcpSocketDiagData struct {
	Name           string   `json:"name"`
	State          string   `json:"state"`
	Local          string   `json:"local"`
	Remote         string   `json:"remote"`
	RTTMs          *float64 `json:"rtt_ms,omitempty"`
	RTTVarMs       *float64 `json:"rttvar_ms,omitempty"`
	RTOMs          *int64   `json:"rto_ms,omitempty"`
	Retransmits    *int64   `json:"retransmits,omitempty"`
	Unacked        *int64   `json:"unacked,omitempty"`
	Cwnd           *int32   `json:"cwnd,omitempty"`
	SendQueueBytes *int64   `json:"send_queue_bytes,omitempty"`
	RecvQueueBytes *int64   `json:"recv_queue_bytes,omitempty"`
	Status         string   `json:"status"`
}

type DiagEventData struct {
	Timestamp string  `json:"ts"`
	Severity  string  `json:"severity"`
	Source    string  `json:"source"`
	Message   string  `json:"message"`
	Fields    *string `json:"fields,omitempty"`
}

// Server is a hermetic diagnostic peer that serves realistic tovarisch responses.
type Server struct {
	port         int
	includeTCP   bool
	httpServer   *http.Server
	stopCh       chan struct{}
	requestLog   []RequestLogEntry
}

type RequestLogEntry struct {
	Method string
	Path   string
	Time   time.Time
}

// NewServer creates a new hermetic diagnostic peer server.
func NewServer(port int, includeTCP bool) *Server {
	return &Server{
		port:       port,
		includeTCP: includeTCP,
		stopCh:     make(chan struct{}),
		requestLog: make([]RequestLogEntry, 0),
	}
}

// Start starts the diagnostic peer server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/status.json", s.handleStatusJSON)
	mux.HandleFunc("/status", s.handleStatus)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", s.port),
		Handler: mux,
	}

	go func() {
		log.Printf("Diagnostic peer server starting on port %d", s.port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Diagnostic peer server error: %v", err)
		}
	}()

	// Wait for server to be ready
	for i := 0; i < 50; i++ {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/status.json", s.port))
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("Diagnostic peer server ready on port %d", s.port)
	return nil
}

// Stop stops the diagnostic peer server.
func (s *Server) Stop() error {
	close(s.stopCh)
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// GetRequestLog returns the recorded request log.
func (s *Server) GetRequestLog() []RequestLogEntry {
	return s.requestLog
}

func (s *Server) handleStatusJSON(w http.ResponseWriter, r *http.Request) {
	// Log the request
	s.requestLog = append(s.requestLog, RequestLogEntry{
		Method: r.Method,
		Path:   r.URL.RequestURI(),
		Time:   time.Now(),
	})

	// Check if network_diag is requested
	include := r.URL.Query().Get("include")
	if include != "network_diag" {
		// Return basic status without network_diag
		resp := TovarischStatusResponse{
			Service:  "tovarisch",
			Version:  "0.1.0",
			NodeID:   "test-node",
			Status:   "ok",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Build response with network_diag
	resp := s.buildStatusResponse()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Log the request
	s.requestLog = append(s.requestLog, RequestLogEntry{
		Method: r.Method,
		Path:   r.URL.RequestURI(),
		Time:   time.Now(),
	})

	// Return basic status (no JSON extension, no network_diag)
	resp := TovarischStatusResponse{
		Service: "tovarisch",
		Version: "0.1.0",
		NodeID:  "test-node",
		Status:  "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) buildStatusResponse() TovarischStatusResponse {
	now := time.Now().UTC().Format(time.RFC3339)

	resp := TovarischStatusResponse{
		Service:  "tovarisch",
		Version:  "0.1.0",
		NodeID:   "test-node",
		Status:   "ok",
		NetworkDiag: &NetworkDiagData{
			StartedAt:  now,
			Status:     "ok",
			Interfaces: s.buildInterfaces(),
			Routes:     s.buildRoutes(),
		},
	}

	if s.includeTCP {
		resp.NetworkDiag.UnderlayTCP = s.buildTCPData()
	} else {
		resp.NetworkDiag.UnderlayTCP = []TcpSocketDiagData{}
	}

	resp.NetworkDiag.Events = []DiagEventData{}

	return resp
}

func (s *Server) buildInterfaces() []InterfaceDiagData {
	return []InterfaceDiagData{
		{
			Name:      "wg0",
			OperState: "up",
			RxBytes:   1024000,
			TxBytes:   512000,
		},
		{
			Name:      "eth0",
			OperState: "up",
			RxBytes:   2048000,
			TxBytes:   1024000,
		},
	}
}

func (s *Server) buildRoutes() []RouteDiagData {
	return []RouteDiagData{
		{
			Target:    "0.0.0.0/0",
			Interface: "eth0",
			Source:    "dynamic",
			Status:    "up",
		},
		{
			Target:    "10.0.0.0/24",
			Interface: "wg0",
			Source:    "static",
			Status:    "up",
		},
	}
}

func (s *Server) buildTCPData() []TcpSocketDiagData {
	rttMs := 15.5
	rttVarMs := 2.3
	rtoMs := int64(1000)
	retransmits := int64(0)
	unacked := int64(0)
	cwnd := int32(29200)
	sendQueueBytes := int64(0)
	recvQueueBytes := int64(0)

	return []TcpSocketDiagData{
		{
			Name:           "wg0",
			State:          "ESTABLISHED",
			Local:          "10.0.0.1:51820",
			Remote:         "10.0.0.2:51820",
			RTTMs:          &rttMs,
			RTTVarMs:       &rttVarMs,
			RTOMs:          &rtoMs,
			Retransmits:    &retransmits,
			Unacked:        &unacked,
			Cwnd:           &cwnd,
			SendQueueBytes: &sendQueueBytes,
			RecvQueueBytes: &recvQueueBytes,
			Status:         "active",
		},
	}
}
