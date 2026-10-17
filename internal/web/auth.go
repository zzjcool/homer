package web

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// isLoopbackAddr checks the address passed to net/http's listener. An empty
// host (":7760") is explicitly treated as the frozen local shorthand; a
// wildcard numeric address is not.
func isLoopbackAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	host := addr
	if parsedHost, _, err := net.SplitHostPort(addr); err == nil {
		host = parsedHost
	} else if strings.HasPrefix(addr, "[") && strings.Contains(addr, "]") {
		end := strings.IndexByte(addr, ']')
		host = addr[1:end]
	} else if strings.Count(addr, ":") == 1 {
		// A host:port without a valid numeric port still has an unambiguous
		// host component for the startup safety check.
		host = addr[:strings.LastIndexByte(addr, ':')]
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type requestPrincipalClass uint8

const (
	principalNone requestPrincipalClass = iota
	principalAdmin
	principalAgentSecret
	principalEnrollCode
)

// principalClass identifies the strongest credential presented by a request.
// Browser sessions and the shared hub token are administrators; machine and
// one-time enrollment credentials stay in their narrower classes.
func (s *Server) principalClass(r *http.Request) requestPrincipalClass {
	if s.auth != nil {
		if _, ok := s.auth.validSession(s.opts.HomerHome, r); ok {
			return principalAdmin
		}
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return principalNone
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if provided == "" {
		return principalNone
	}
	if s.opts.Token != "" && provided == s.opts.Token {
		return principalAdmin
	}
	// Enrollment codes are checked but never burned here. Redemption remains
	// strictly one-shot in the authenticated agent stream hello.
	if s.opts.Enrollment != nil && s.opts.Enrollment.ValidCode(provided) {
		return principalEnrollCode
	}
	if s.opts.AgentEndpointAuthorized != nil && s.opts.AgentEndpointAuthorized(r) {
		return principalAgentSecret
	}
	return principalNone
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	principal := s.principalClass(r)
	if principal == principalAdmin || s.principalAllowedForEndpoint(principal, r.Method, r.URL.Path) {
		s.slideSession(w, r)
		return true
	}
	writeError(w, http.StatusUnauthorized, "unauthorized", "未授权：请提供有效的 Bearer token", nil)
	return false
}

// principalAllowedForEndpoint is a default-deny whitelist for non-admin
// credentials. A recognized credential is not generally authorized for /api
// or /dl data access: machine secrets get the agent data plane and listed
// read-only APIs, while enrollment codes get only bootstrap downloads, snapshot
// reads, and the stream handled by the agent protocol. Writes stay admin-only
// except for the machine snapshot upload protocol.
func (s *Server) principalAllowedForEndpoint(principal requestPrincipalClass, method, path string) bool {
	if method == http.MethodGet && strings.HasPrefix(path, "/dl/") {
		return principal == principalAgentSecret || principal == principalEnrollCode
	}
	if path == "/api/snapshot" {
		switch method {
		case http.MethodGet:
			return principal == principalAgentSecret || principal == principalEnrollCode
		case http.MethodPost:
			return principal == principalAgentSecret
		}
		return false
	}
	if principal != principalAgentSecret || method != http.MethodGet {
		return false
	}
	switch path {
	case "/api/console", "/api/status", "/api/diff", "/api/sync/choices",
		"/api/config", "/api/agents", "/api/storage", "/api/storage/file", "/api/keys":
		return true
	default:
		if strings.HasPrefix(path, "/api/agents/") {
			parts := strings.Split(strings.TrimPrefix(path, "/api/agents/"), "/")
			return len(parts) == 2 && parts[0] != "" && parts[1] == "adapters"
		}
		return false
	}
}

// slideSession extends an active browser session once less than half of the
// idle window remains. Bearer callers have nothing to extend.
func (s *Server) slideSession(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		return
	}
	expiry, ok := s.auth.validSession(s.opts.HomerHome, r)
	if !ok || time.Until(expiry) >= sessionMaxAge/2 {
		return
	}
	cookie, err := s.auth.issueSession(s.opts.HomerHome)
	if err != nil {
		return
	}
	http.SetCookie(w, cookie)
}
