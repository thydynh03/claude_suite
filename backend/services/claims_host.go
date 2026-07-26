package services

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"claude_suite/backend/claims"
)

// ClaimsHostService runs the adjudication host so agents on other machines can
// reach it.
//
// It binds synchronously for the same reason the webhook service does: starting
// a listener in a goroutine and returning nil reports success for a port that
// was never taken, and the UI then shows a host nobody can connect to.
type ClaimsHostService struct {
	mu       sync.Mutex
	server   *http.Server
	host     *claims.Host
	addr     string
	running  bool
	onEvent  func(sessionID, message string)
	sessions []string
}

func NewClaimsHostService() *ClaimsHostService {
	return &ClaimsHostService{}
}

// SetEventHandler receives progress lines for the UI.
func (s *ClaimsHostService) SetEventHandler(fn func(sessionID, message string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = fn
	if s.host != nil {
		s.host.OnEvent = fn
	}
}

// Start listens on port and serves the websocket endpoint.
//
// workspace is where the check catalogue is read from and where falsifiers run.
func (s *ClaimsHostService) Start(port int, workspace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}
	if port == 0 {
		port = 9111
	}

	catalogue, err := claims.CatalogueFor(workspace)
	if err != nil {
		return fmt.Errorf("load check catalogue: %w", err)
	}

	host := claims.NewHost(&claims.Runner{Workspace: workspace, Catalogue: catalogue})
	host.OnEvent = s.onEvent

	// Bind before returning so a taken port is reported to the caller.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("claims host cannot listen on port %d: %w", port, err)
	}

	server := &http.Server{
		Handler:           host.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.server = server
	s.host = host
	s.addr = listener.Addr().String()
	s.running = true

	go func() {
		_ = server.Serve(listener)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	return nil
}

// Stop shuts the host down.
func (s *ClaimsHostService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil || !s.running {
		return nil
	}
	s.running = false
	return s.server.Close()
}

// IsRunning reports whether agents can currently connect.
func (s *ClaimsHostService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Addr is what the host is listening on.
func (s *ClaimsHostService) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// CatalogueNames lists the checks a claim may point at.
func (s *ClaimsHostService) CatalogueNames(workspace string) ([]string, error) {
	catalogue, err := claims.CatalogueFor(workspace)
	if err != nil {
		return nil, err
	}
	return catalogue.Names(), nil
}

// Open starts a session and returns its id and join token.
func (s *ClaimsHostService) Open(subject string, ttlMinutes int) (string, string, error) {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return "", "", fmt.Errorf("the claims host is not running")
	}

	ttl := time.Duration(ttlMinutes) * time.Minute
	id, token, err := host.Open(subject, ttl)
	if err != nil {
		return "", "", err
	}

	s.mu.Lock()
	s.sessions = append(s.sessions, id)
	s.mu.Unlock()
	return id, token, nil
}

// SessionIDs lists sessions opened on this host, newest last.
func (s *ClaimsHostService) SessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sessions...)
}

// Session exposes one session for inspection.
func (s *ClaimsHostService) Session(id string) (*claims.Session, bool) {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return nil, false
	}
	return host.Session(id)
}

// Say posts a chat message into a session and broadcasts it, so agents
// listening over a websocket hear the arbiter immediately instead of at the
// next unrelated event.
func (s *ClaimsHostService) Say(sessionID, author, text string) error {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return fmt.Errorf("the claims host is not running")
	}
	return host.Say(sessionID, author, text)
}

// ForceAdjudicate ends a collect window that is waiting on an agent that will
// not report finished.
func (s *ClaimsHostService) ForceAdjudicate(sessionID string) error {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return fmt.Errorf("the claims host is not running")
	}
	return host.ForceAdjudicate(sessionID)
}

// OpenDebateRound moves a revealed session into discussion. Only opinions are
// discussable; everything the checks settled stays settled.
func (s *ClaimsHostService) OpenDebateRound(sessionID string) error {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return fmt.Errorf("the claims host is not running")
	}
	return host.OpenDebateRound(sessionID)
}

// Finish closes a session and returns its outcome.
func (s *ClaimsHostService) Finish(sessionID string) (*claims.Outcome, error) {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	if host == nil {
		return nil, fmt.Errorf("the claims host is not running")
	}
	return host.Finish(sessionID)
}
