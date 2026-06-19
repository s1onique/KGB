package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// captureURLLabTest captures all requests made to the fake server for verification.
type captureURLLabTest struct {
	// requests records all requests received by the fake server.
	// Uses atomic operations for thread-safe access from test goroutines.
	requests atomic.Value // stores []capturedRequest

	// server is the httptest server serving fake tovarisch responses.
	server *httptest.Server

	// lastRequest captures the most recent request for simple assertions.
	lastRequest atomic.Value // stores *capturedRequest
}

type capturedRequest struct {
	Method string
	Path   string
	Query  map[string][]string
}

func newCaptureURLLabTest() *captureURLLabTest {
	lab := &captureURLLabTest{}
	lab.requests.Store([]capturedRequest{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the request
		req := capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
		}
		lab.lastRequest.Store(&req)

		// Append to requests slice
		existing := lab.requests.Load().([]capturedRequest)
		lab.requests.Store(append(existing, req))

		// Validate canonical endpoint: GET /status.json with include=network_diag
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/status.json" {
			http.Error(w, "not found: wrong path "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("include") != "network_diag" {
			http.Error(w, "bad request: missing include=network_diag", http.StatusBadRequest)
			return
		}

		// Return 200 with valid network_diag response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validTovarischStatusJSON()))
	})

	lab.server = httptest.NewServer(handler)
	return lab
}

func (lab *captureURLLabTest) URL() string {
	return lab.server.URL
}

func (lab *captureURLLabTest) Close() {
	lab.server.Close()
}

func (lab *captureURLLabTest) getRequests() []capturedRequest {
	return lab.requests.Load().([]capturedRequest)
}

func (lab *captureURLLabTest) getLastRequest() *capturedRequest {
	return lab.lastRequest.Load().(*capturedRequest)
}
