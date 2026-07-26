package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MCPServer is one Model Context Protocol server the sub-agents may use.
//
// Configuration used to be a single free-text field called
// "mcp_connection_string" with no list, no validation and no way to tell whether
// what you typed works. A wrong value produced no error here and no error later
// — the agents simply ran without the tools you thought you had given them.
type MCPServer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Desc    string            `json:"description"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	// URL is set instead of Command for servers reached over the network.
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	// Builtin marks an entry that came from the bundled catalogue rather than
	// from the user, so the UI can stop them being edited into something else.
	Builtin bool `json:"builtin"`

	LastCheckedAt time.Time `json:"last_checked_at"`
	LastStatus    string    `json:"last_status"` // "" | "ok" | "error"
	LastError     string    `json:"last_error"`
}

// MCPCatalogue is the set of servers offered without a web search. They are the
// ones published by the protocol's own maintainers, so the list is short and
// checkable rather than long and unverified.
//
// Adding one still requires the user to confirm: an MCP server runs commands on
// their machine with their permissions.
func MCPCatalogue() []MCPServer {
	return []MCPServer{
		{
			ID:      "filesystem",
			Name:    "Filesystem",
			Desc:    "Đọc/ghi tệp trong một thư mục được chỉ định. Thư mục truyền qua tham số.",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
			Builtin: true,
		},
		{
			ID:      "git",
			Name:    "Git",
			Desc:    "Đọc lịch sử commit, diff và trạng thái của một repository.",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-git"},
			Builtin: true,
		},
		{
			ID:      "fetch",
			Name:    "Fetch",
			Desc:    "Tải nội dung một URL và chuyển sang văn bản cho model đọc.",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-fetch"},
			Builtin: true,
		},
		{
			ID:      "memory",
			Name:    "Memory",
			Desc:    "Bộ nhớ dạng knowledge graph, giữ ngữ cảnh giữa các phiên.",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
			Builtin: true,
		},
		{
			ID:      "sqlite",
			Name:    "SQLite",
			Desc:    "Truy vấn một tệp cơ sở dữ liệu SQLite.",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sqlite"},
			Builtin: true,
		},
	}
}

// MCPStore persists the servers the user configured.
type MCPStore struct {
	mu      sync.Mutex
	path    string
	servers []MCPServer
}

func NewMCPStore(dir string) *MCPStore {
	s := &MCPStore{path: filepath.Join(dir, "mcp_servers.json")}
	s.load()
	return s
}

func (s *MCPStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var servers []MCPServer
	if json.Unmarshal(data, &servers) == nil {
		s.servers = servers
	}
}

// save writes atomically: a half-written file here means the app starts next time
// with no MCP servers at all and no indication why.
func (s *MCPStore) saveLocked() error {
	data, err := json.MarshalIndent(s.servers, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.path, data, 0644)
}

func (s *MCPStore) List() []MCPServer {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]MCPServer, len(s.servers))
	copy(out, s.servers)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Add stores a server, rejecting the shapes that cannot work rather than saving
// them for a failure at run time.
func (s *MCPStore) Add(server MCPServer) (MCPServer, error) {
	server.Name = strings.TrimSpace(server.Name)
	server.Command = strings.TrimSpace(server.Command)
	server.URL = strings.TrimSpace(server.URL)

	if server.Name == "" {
		return server, fmt.Errorf("cần đặt tên cho MCP server")
	}
	if server.Command == "" && server.URL == "" {
		return server, fmt.Errorf("cần có lệnh chạy (command) hoặc URL")
	}
	if server.Command != "" && server.URL != "" {
		return server, fmt.Errorf("chỉ chọn một: lệnh chạy hoặc URL, không dùng cả hai")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if server.ID == "" {
		server.ID = fmt.Sprintf("mcp-%d", len(s.servers)+1)
	}
	for _, existing := range s.servers {
		if existing.ID == server.ID || strings.EqualFold(existing.Name, server.Name) {
			return server, fmt.Errorf("đã có MCP server tên %q", server.Name)
		}
	}

	s.servers = append(s.servers, server)
	return server, s.saveLocked()
}

func (s *MCPStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("không tìm thấy MCP server %q", id)
}

func (s *MCPStore) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers[i].Enabled = enabled
			return s.saveLocked()
		}
	}
	return fmt.Errorf("không tìm thấy MCP server %q", id)
}

// TestServer checks whether the configured server can actually be started, and
// records the answer.
//
// Command-based servers are checked by resolving the executable on PATH: a
// missing `npx` is the overwhelmingly common failure and it is worth naming
// before a task fails. It deliberately does not run the server — starting a
// process that speaks a protocol over stdio and waiting for a handshake is a
// different job, and one that can hang.
func (s *MCPStore) TestServer(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.servers {
		if s.servers[i].ID != id {
			continue
		}

		srv := &s.servers[i]
		srv.LastCheckedAt = time.Now()

		if srv.Command != "" {
			path, err := exec.LookPath(srv.Command)
			if err != nil {
				srv.LastStatus = "error"
				srv.LastError = fmt.Sprintf("không tìm thấy lệnh %q trên PATH — cài đặt nó rồi thử lại", srv.Command)
				_ = s.saveLocked()
				return "", fmt.Errorf("%s", srv.LastError)
			}
			srv.LastStatus = "ok"
			srv.LastError = ""
			_ = s.saveLocked()
			return fmt.Sprintf("Tìm thấy %s tại %s", srv.Command, path), nil
		}

		if !strings.HasPrefix(srv.URL, "http://") && !strings.HasPrefix(srv.URL, "https://") {
			srv.LastStatus = "error"
			srv.LastError = "URL phải bắt đầu bằng http:// hoặc https://"
			_ = s.saveLocked()
			return "", fmt.Errorf("%s", srv.LastError)
		}

		srv.LastStatus = "ok"
		srv.LastError = ""
		_ = s.saveLocked()
		return "URL hợp lệ. App chưa gọi thử — sẽ kết nối khi agent chạy.", nil
	}

	return "", fmt.Errorf("không tìm thấy MCP server %q", id)
}
