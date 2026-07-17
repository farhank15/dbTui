package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/farhank15/dbTui/internal/db"
)

type Server struct {
	manager *db.Manager
	scanner *bufio.Scanner
	writer  *json.Encoder
	logger  *log.Logger
	startAt time.Time
	mu      sync.Mutex
}

func NewServer() *Server {
	return &Server{
		manager: db.NewManager(),
		scanner: bufio.NewScanner(os.Stdin),
		writer:  json.NewEncoder(os.Stdout),
		logger:  log.New(os.Stderr, "", log.LstdFlags),
		startAt: time.Now(),
	}
}

func (s *Server) Run() error {
	s.log("[INFO] agent started (pid=%d)", os.Getpid())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		s.log("[INFO] shutdown (signal=%v)", sig)
		s.cleanup()
		os.Exit(0)
	}()

	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(0, ErrInvalidParams, "invalid JSON: "+err.Error(), nil)
			continue
		}

		resp := s.handleRequest(req)
		s.sendResponse(resp)
	}

	s.log("[INFO] stdin EOF, shutting down")
	s.cleanup()
	return nil
}

func (s *Server) handleRequest(req Request) Response {
	if req.JSONRPC != "2.0" {
		return s.makeError(req.ID, ErrInvalidParams, "jsonrpc must be 2.0", nil)
	}

	switch req.Method {
	case MethodPing:
		return s.handlePing(req)
	case MethodConnect:
		return s.handleConnect(req)
	case MethodDisconnect:
		return s.handleDisconnect(req)
	case MethodList:
		return s.handleList(req)
	case MethodQuery:
		return s.handleQuery(req)
	case MethodSchema:
		return s.handleSchema(req)
	case MethodTables:
		return s.handleTables(req)
	case MethodExplain:
		return s.handleExplain(req)
	case MethodStats:
		return s.handleStats(req)
	default:
		return s.makeError(req.ID, ErrInvalidParams, "unknown method: "+req.Method, nil)
	}
}

func (s *Server) sendResponse(resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writer.Encode(resp)
}

func (s *Server) sendError(id int, code int, msg string, data interface{}) {
	s.sendResponse(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObj{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	})
}

func (s *Server) makeResult(id int, result interface{}) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) makeError(id int, code int, msg string, data interface{}) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &ErrorObj{Code: code, Message: msg, Data: data},
	}
}

func (s *Server) cleanup() {
	for _, state := range s.manager.GetActiveConnections() {
		s.manager.Disconnect(state.Connection.ID)
	}
	s.log("[INFO] agent shutdown (uptime=%s)", time.Since(s.startAt).Round(time.Second))
}

func (s *Server) log(format string, args ...interface{}) {
	s.logger.Printf(format, args...)
}

func (s *Server) redactURL(rawURL string) string {
	// postgres://user:pass@host/db → postgres://user:***@host/db
	afterScheme := strings.Index(rawURL, "://")
	if afterScheme == -1 {
		return rawURL
	}
	rest := rawURL[afterScheme+3:]
	atIdx := strings.LastIndex(rest, "@")
	if atIdx == -1 {
		return rawURL
	}
	return rawURL[:afterScheme+3] + "user:***" + rest[atIdx:]
}

func (s *Server) queryTimeout() time.Duration {
	return 30 * time.Second
}

func (s *Server) defaultMaxRows() int {
	return 1000
}

// safeString converts any value to string, nil → empty
func safeString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
