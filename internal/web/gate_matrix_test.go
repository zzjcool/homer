package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type gateCredential struct {
	name   string
	token  string
	cookie bool
}

func TestGateMatrix(t *testing.T) {
	fixture := authFixture(t)
	enrollment := &matrixEnrollmentService{codes: map[string]bool{"hr_live": true}}
	var forwardedPath string
	server, err := NewServer(ServeOptions{
		Addr:       "127.0.0.1:0",
		HomerHome:  fixture.home,
		Token:      "hub-token",
		Enrollment: enrollment,
		Agents:     &statusOnlySource{list: []AgentInfo{{AgentID: "box", Hostname: "box"}}},
		Overrides:  LoadAgentOverrides(fixture.home),
		AgentEndpointAuthorized: func(r *http.Request) bool {
			return r.Header.Get("Authorization") == "Bearer agent-secret-live"
		},
		AgentEndpoint: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			forwardedPath = r.URL.Path
			switch r.URL.Path {
			case "/agent/v1/stream":
				token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
				if token == "hub-token" || token == "agent-secret-live" || enrollment.ValidCode(token) {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
			case "/agent/v1/enroll", "/agent/v1/register", "/agent/v1/poll", "/agent/v1/report":
				w.WriteHeader(http.StatusGone)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := setupAndLogin(t, server.Handler())

	credentials := []gateCredential{
		{name: "none"},
		{name: "cookie", cookie: true},
		{name: "hub token", token: "hub-token"},
		{name: "unused enrollment code", token: "hr_live"},
		{name: "burned or expired enrollment code", token: "hr_burned"},
		{name: "per-agent secret", token: "agent-secret-live"},
		{name: "revoked per-agent secret", token: "agent-secret-revoked"},
	}
	type gatePath struct {
		name   string
		method string
		path   string
		body   string
		want   func(gateCredential) int
	}
	paths := []gatePath{
		{
			name: "GET /agent/v1/stream (web bypass; endpoint authenticates)",
			path: "/agent/v1/stream",
			want: func(credential gateCredential) int {
				switch credential.name {
				case "hub token", "unused enrollment code", "per-agent secret":
					return http.StatusOK
				default:
					return http.StatusUnauthorized
				}
			},
		},
		{
			name: "GET /agent/v1/enroll tombstone",
			path: "/agent/v1/enroll",
			want: func(gateCredential) int { return http.StatusGone },
		},
		{
			name: "GET /agent/v1/register tombstone",
			path: "/agent/v1/register",
			want: func(gateCredential) int { return http.StatusGone },
		},
		{
			name: "GET /agent/v1/poll tombstone",
			path: "/agent/v1/poll",
			want: func(gateCredential) int { return http.StatusGone },
		},
		{
			name: "GET /agent/v1/report tombstone",
			path: "/agent/v1/report",
			want: func(gateCredential) int { return http.StatusGone },
		},
		{
			name: "other /agent/*",
			path: "/agent/v1/other",
			want: func(gateCredential) int { return http.StatusNotFound },
		},
		{
			name: "/api/console",
			path: "/api/console",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/api/console") {
					return http.StatusOK
				}
				return http.StatusUnauthorized
			},
		},
		{
			name:   "GET /api/resolve/record",
			method: http.MethodGet,
			path:   "/api/resolve/record?agent=box",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/api/resolve/record") {
					return http.StatusNotImplemented
				}
				return http.StatusUnauthorized
			},
		},
		{
			name:   "POST /api/resolve/clear",
			method: http.MethodPost,
			path:   "/api/resolve/clear?agent=box&confirm=true",
			body:   `{"adapters":["pi"]}`,
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodPost, "/api/resolve/clear") {
					return http.StatusNotImplemented
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /api/plugins",
			path: "/api/plugins",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/api/plugins") {
					return http.StatusOK
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /api/agents/box/adapters",
			path: "/api/agents/box/adapters",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/api/agents/box/adapters") {
					return http.StatusOK
				}
				return http.StatusUnauthorized
			},
		},
		{
			name:   "POST /api/agents/box/adapters/keyring",
			method: http.MethodPost,
			path:   "/api/agents/box/adapters/keyring",
			body:   `{"disabled":true}`,
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodPost, "/api/agents/box/adapters/keyring") {
					return http.StatusServiceUnavailable // authenticated, but this fixture has no plugin state
				}
				return http.StatusUnauthorized
			},
		},
		{
			// POST /api/plugins/install 与 uninstall：未接插件状态的 fixture
			// 对合法凭证返回 503 plugins-disabled，闸门之外一律 401。
			name:   "POST /api/plugins/install",
			method: http.MethodPost,
			path:   "/api/plugins/install",
			body:   `{"id":"pi"}`,
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodPost, "/api/plugins/install") {
					return http.StatusServiceUnavailable
				}
				return http.StatusUnauthorized
			},
		},
		{
			name:   "POST /api/plugins/uninstall",
			method: http.MethodPost,
			path:   "/api/plugins/uninstall",
			body:   `{"id":"pi","force":true}`,
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodPost, "/api/plugins/uninstall") {
					return http.StatusServiceUnavailable
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /api/snapshot",
			path: "/api/snapshot",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/api/snapshot") {
					return http.StatusConflict // authenticated, but no generation exists yet
				}
				return http.StatusUnauthorized
			},
		},
		{
			name:   "POST /api/snapshot (agent data plane)",
			method: http.MethodPost,
			path:   "/api/snapshot",
			body:   "{}",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodPost, "/api/snapshot") {
					return http.StatusOK
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /api/auth/join",
			path: "/api/auth/join",
			want: func(credential gateCredential) int {
				if credential.cookie || credential.name == "hub token" {
					return http.StatusOK
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /dl/homer",
			path: "/dl/homer",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/dl/homer") {
					return http.StatusPartialContent
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "GET /dl/homer.gz",
			path: "/dl/homer.gz",
			want: func(credential gateCredential) int {
				if gateAllowsAPI(credential, http.MethodGet, "/dl/homer.gz") {
					return http.StatusPartialContent
				}
				return http.StatusUnauthorized
			},
		},
		{
			name: "public /install.sh",
			path: "/install.sh",
			want: func(gateCredential) int { return http.StatusOK },
		},
	}

	for _, path := range paths {
		path := path
		t.Run(path.name, func(t *testing.T) {
			for _, credential := range credentials {
				credential := credential
				t.Run(credential.name, func(t *testing.T) {
					forwardedPath = ""
					method := path.method
					if method == "" {
						method = http.MethodGet
					}
					request := httptest.NewRequest(method, path.path, strings.NewReader(path.body))
					if credential.token != "" {
						request.Header.Set("Authorization", "Bearer "+credential.token)
					}
					if credential.cookie {
						request.AddCookie(cookie)
					}
					if strings.HasPrefix(path.path, "/dl/") {
						request.Header.Set("Range", "bytes=0-0")
					}
					response := httptest.NewRecorder()
					server.Handler().ServeHTTP(response, request)
					if response.Code != path.want(credential) {
						t.Fatalf("status = %d, want %d, body=%s", response.Code, path.want(credential), response.Body)
					}
					if strings.HasPrefix(path.path, "/agent/") && forwardedPath != path.path {
						t.Fatalf("path was not delegated to AgentEndpoint: got %q", forwardedPath)
					}
				})
			}
		})
	}
}

func TestEnrollmentCodeCannotAccessWriteEndpoints(t *testing.T) {
	fixture := authFixture(t)
	enrollment := &matrixEnrollmentService{codes: map[string]bool{"hr_live": true}}
	server, err := NewServer(ServeOptions{
		Addr:       "127.0.0.1:0",
		HomerHome:  fixture.home,
		Token:      "hub-token",
		Enrollment: enrollment,
		Agents:     &statusOnlySource{list: []AgentInfo{{AgentID: "box", Hostname: "box"}}},
		Overrides:  LoadAgentOverrides(fixture.home),
		AgentEndpointAuthorized: func(r *http.Request) bool {
			return r.Header.Get("Authorization") == "Bearer agent-secret-live"
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := setupAndLogin(t, server.Handler())
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/push"},
		{http.MethodPost, "/api/pull"},
		{http.MethodPost, "/api/sync"},
		{http.MethodPost, "/api/snapshot"},
		{http.MethodPost, "/api/resolve/record"},
		{http.MethodGet, "/api/resolve/record"},
		{http.MethodPost, "/api/resolve/clear"},
		{http.MethodPost, "/api/resolve"},
		{http.MethodPost, "/api/plugins/install"},
		{http.MethodPost, "/api/plugins/uninstall"},
		{http.MethodPost, "/api/keys"},
		{http.MethodPost, "/api/keys/encrypt"},
		{http.MethodPost, "/api/keys/unlock"},
		{http.MethodPost, "/api/keys/password"},
		{http.MethodPost, "/api/keys/op"},
		{http.MethodPost, "/api/agents/revoke"},
		{http.MethodPost, "/api/agents/remove"},
		{http.MethodPost, "/api/agents/box/adapters/keyring"},
		{http.MethodPost, "/api/agents/box/status"},
		{http.MethodPost, "/api/agents/box/diff"},
		{http.MethodPost, "/api/agents/box/push"},
		{http.MethodPost, "/api/agents/box/pull"},
		{http.MethodPost, "/api/agents/box/ssh-key"},
		{http.MethodPost, "/api/agents/box/keys"},
		{http.MethodPost, "/api/agents/box/upgrade"},
		{http.MethodPost, "/api/agents/box/tool-upgrade"},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			request := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer hr_live")
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with enrollment code = %d, want 401; body=%s", endpoint.method, endpoint.path, response.Code, response.Body)
			}
			for _, admin := range []struct {
				name   string
				token  string
				cookie *http.Cookie
			}{
				{name: "hub token", token: "hub-token"},
				{name: "session cookie", cookie: cookie},
			} {
				request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
				if admin.token != "" {
					request.Header.Set("Authorization", "Bearer "+admin.token)
				}
				if admin.cookie != nil {
					request.AddCookie(admin.cookie)
				}
				if !server.requireAuth(httptest.NewRecorder(), request) {
					t.Errorf("%s %s rejected %s at requireAuth", endpoint.method, endpoint.path, admin.name)
				}
			}
		})
	}
}

func TestResolveRecordRouteMethods(t *testing.T) {
	fixture := authFixture(t)
	server := newWebServer(t, fixture, "test-token", nil, nil)
	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/api/resolve/record"},
		{method: http.MethodGet, path: "/api/resolve/clear"},
	} {
		response := request(t, server.Handler(), testCase.method, testCase.path)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d %s, want 405", testCase.method, testCase.path, response.Code, response.Body)
		}
	}
}

func gateAllowsAPI(credential gateCredential, method, path string) bool {
	if credential.cookie || credential.name == "hub token" {
		return true
	}
	if method == "" {
		method = http.MethodGet
	}
	if method == http.MethodGet && strings.HasPrefix(path, "/dl/") {
		return credential.name == "unused enrollment code" || credential.name == "per-agent secret"
	}
	if path == "/api/snapshot" {
		return method == http.MethodGet && (credential.name == "unused enrollment code" || credential.name == "per-agent secret") ||
			method == http.MethodPost && credential.name == "per-agent secret"
	}
	if credential.name != "per-agent secret" || method != http.MethodGet {
		return false
	}
	switch path {
	case "/api/console", "/api/status", "/api/diff", "/api/sync/choices", "/api/config", "/api/agents", "/api/storage", "/api/storage/file", "/api/keys":
		return true
	default:
		if strings.HasPrefix(path, "/api/agents/") {
			parts := strings.Split(strings.TrimPrefix(path, "/api/agents/"), "/")
			return len(parts) == 2 && parts[0] != "" && parts[1] == "adapters"
		}
		return false
	}
}

type matrixEnrollmentService struct {
	codes map[string]bool
}

func (s *matrixEnrollmentService) Mint(time.Duration) (string, error) {
	s.codes["hr_live"] = true
	return "hr_live", nil
}

func (*matrixEnrollmentService) Revoke(string) bool { return false }
func (s *matrixEnrollmentService) ValidCode(code string) bool {
	return s.codes[code]
}
