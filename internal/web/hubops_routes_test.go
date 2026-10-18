package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// hubOpsStub is a controllable HubOperations for the route layer. The web
// package owns only the HTTP mapping, so the stub is the whole "hub" here:
// every branch the handlers can take is a field.
type hubOpsStub struct {
	status       HubStatus
	updateResult HubUpdateAccepted
	updateErr    error
	restart      HubRestartAccepted
	restartErr   error

	statusCalls  int
	updateCalls  int
	restartCalls int
}

func (s *hubOpsStub) HubStatus(context.Context) HubStatus {
	s.statusCalls++
	return s.status
}

func (s *hubOpsStub) StartHubUpdate(context.Context) (HubUpdateAccepted, error) {
	s.updateCalls++
	if s.updateErr != nil {
		return HubUpdateAccepted{}, s.updateErr
	}
	return s.updateResult, nil
}

func (s *hubOpsStub) RestartHub(context.Context) (HubRestartAccepted, error) {
	s.restartCalls++
	if s.restartErr != nil {
		return HubRestartAccepted{}, s.restartErr
	}
	return s.restart, nil
}

func newHubOpsTestServer(t *testing.T, hubOps HubOperations) *Server {
	t.Helper()
	fixture := authFixture(t)
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: fixture.home,
		Token:     "test-token",
		HubOps:    hubOps,
		Agents:    &statusOnlySource{list: []AgentInfo{{AgentID: "box", Hostname: "box"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

// TestHubRoutesDisabledWithoutHubOps freezes D2/D8: an embedded server with no
// hub lifecycle answers 501 hub-disabled on all three routes. Not 404 (the
// paths exist and the console links to them) and not 503 (the hub is not busy,
// it simply cannot do this).
func TestHubRoutesDisabledWithoutHubOps(t *testing.T) {
	fixture := authFixture(t)
	server := newWebServer(t, fixture, "test-token", nil, nil)
	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/hub"},
		{http.MethodPost, "/api/hub/upgrade"},
		{http.MethodPost, "/api/hub/restart"},
	} {
		response := request(t, server.Handler(), testCase.method, testCase.path)
		if response.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s = %d, want 501; body=%s", testCase.method, testCase.path, response.Code, response.Body)
		}
		body := decodeBody(t, response)
		detail, ok := body["error"].(map[string]any)
		if !ok || detail["code"] != hubCodeDisabled {
			t.Fatalf("%s %s error code = %#v, want %q", testCase.method, testCase.path, body["error"], hubCodeDisabled)
		}
		if detail["message"] != "这台 hub 不能自己更新" {
			t.Fatalf("%s %s message = %q", testCase.method, testCase.path, detail["message"])
		}
	}
}

// TestHubStatusRouteFields freezes the GET /api/hub wire: the console renders
// version, bootId, phase and download progress from exactly these keys (D1,
// D3, D4), so a rename here is a silent console regression.
func TestHubStatusRouteFields(t *testing.T) {
	stub := &hubOpsStub{status: HubStatus{
		Version:    "1.5.0",
		BootID:     "9f3c1a77",
		Phase:      hubPhaseDownloading,
		CanUpdate:  true,
		Latest:     "1.6.0",
		Downloaded: 4096,
		Total:      8192,
		Note:       "正在下载",
	}}
	server := newHubOpsTestServer(t, stub)
	response := request(t, server.Handler(), http.MethodGet, "/api/hub")
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/hub = %d, want 200; body=%s", response.Code, response.Body)
	}
	body := decodeBody(t, response)
	for key, want := range map[string]any{
		"version":    "1.5.0",
		"bootId":     "9f3c1a77",
		"phase":      hubPhaseDownloading,
		"canUpdate":  true,
		"latest":     "1.6.0",
		"downloaded": float64(4096),
		"total":      float64(8192),
		"note":       "正在下载",
	} {
		if body[key] != want {
			t.Fatalf("GET /api/hub %s = %#v, want %#v; body=%s", key, body[key], want, response.Body)
		}
	}
	if stub.statusCalls != 1 {
		t.Fatalf("HubStatus calls = %d, want 1", stub.statusCalls)
	}
}

// TestHubRestartRouteAccepted freezes the 202 body the console waits on:
// restartId for the log line, bootId as the "old boot" it compares against
// /api/health, busy to warn that a sync was interrupted (D3, D11).
func TestHubRestartRouteAccepted(t *testing.T) {
	stub := &hubOpsStub{restart: HubRestartAccepted{RestartID: "r-42", BootID: "9f3c1a77", Busy: true}}
	server := newHubOpsTestServer(t, stub)
	response := request(t, server.Handler(), http.MethodPost, "/api/hub/restart")
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST /api/hub/restart = %d, want 202; body=%s", response.Code, response.Body)
	}
	body := decodeBody(t, response)
	if body["restartId"] != "r-42" || body["bootId"] != "9f3c1a77" || body["busy"] != true {
		t.Fatalf("restart body = %s", response.Body)
	}
	if stub.restartCalls != 1 {
		t.Fatalf("RestartHub calls = %d, want 1", stub.restartCalls)
	}
}

// TestHubUpdateRouteAccepted: the update is asynchronous by construction (a
// successful one ends in an exec), so acceptance is 202 with the accepted
// phase, never a 200 with a result.
func TestHubUpdateRouteAccepted(t *testing.T) {
	stub := &hubOpsStub{updateResult: HubUpdateAccepted{Phase: hubPhaseChecking, BootID: "9f3c1a77"}}
	server := newHubOpsTestServer(t, stub)
	response := request(t, server.Handler(), http.MethodPost, "/api/hub/upgrade")
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST /api/hub/upgrade = %d, want 202; body=%s", response.Code, response.Body)
	}
	body := decodeBody(t, response)
	if body["phase"] != hubPhaseChecking || body["bootId"] != "9f3c1a77" {
		t.Fatalf("upgrade body = %s", response.Body)
	}
	if stub.updateCalls != 1 {
		t.Fatalf("StartHubUpdate calls = %d, want 1", stub.updateCalls)
	}
}

// TestHubErrorMapping freezes D8's HTTP mapping. The cli layer constructs
// *HubError; the web layer must not reinterpret it, and must not invent a
// second envelope for anything else.
func TestHubErrorMapping(t *testing.T) {
	cases := []struct {
		name        string
		route       string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
		wantDetails []string
	}{
		{
			name:        "update already running",
			route:       "/api/hub/upgrade",
			err:         &HubError{Status: http.StatusConflict, Code: hubCodeRunning, Message: "更新正在进行，完成后会自动重启"},
			wantStatus:  http.StatusConflict,
			wantCode:    hubCodeRunning,
			wantMessage: "更新正在进行，完成后会自动重启",
			wantDetails: []string{},
		},
		{
			name:        "source build refuses to self-update",
			route:       "/api/hub/upgrade",
			err:         &HubError{Status: http.StatusConflict, Code: hubCodeDevVersion, Message: "这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启"},
			wantStatus:  http.StatusConflict,
			wantCode:    hubCodeDevVersion,
			wantMessage: "这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启",
			wantDetails: []string{},
		},
		{
			name:        "update failed carries the reason",
			route:       "/api/hub/upgrade",
			err:         &HubError{Status: http.StatusBadGateway, Code: hubCodeFailed, Message: "更新失败", Details: []string{"checksum mismatch"}},
			wantStatus:  http.StatusBadGateway,
			wantCode:    hubCodeFailed,
			wantMessage: "更新失败",
			wantDetails: []string{"checksum mismatch"},
		},
		{
			name:        "restart unsupported platform",
			route:       "/api/hub/restart",
			err:         &HubError{Status: http.StatusNotImplemented, Code: hubCodeUnsupported, Message: "此平台不支持自重启，请手动重启 homer serve"},
			wantStatus:  http.StatusNotImplemented,
			wantCode:    hubCodeUnsupported,
			wantMessage: "此平台不支持自重启，请手动重启 homer serve",
			wantDetails: []string{},
		},
		{
			name:        "plain error keeps the existing mapping",
			route:       "/api/hub/restart",
			err:         errors.New("exec boom"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "internal",
			wantMessage: "内部错误",
			wantDetails: []string{"exec boom"},
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			stub := &hubOpsStub{restartErr: testCase.err, updateErr: testCase.err}
			server := newHubOpsTestServer(t, stub)
			response := request(t, server.Handler(), http.MethodPost, testCase.route)
			if response.Code != testCase.wantStatus {
				t.Fatalf("%s = %d, want %d; body=%s", testCase.route, response.Code, testCase.wantStatus, response.Body)
			}
			body := decodeBody(t, response)
			detail, ok := body["error"].(map[string]any)
			if !ok {
				t.Fatalf("%s: no error envelope; body=%s", testCase.route, response.Body)
			}
			if detail["code"] != testCase.wantCode || detail["message"] != testCase.wantMessage {
				t.Fatalf("%s error = %#v, want code=%q message=%q", testCase.route, detail, testCase.wantCode, testCase.wantMessage)
			}
			// A plain error must go through writeErrorValue, which always
			// materialises an array — not a null a console would crash on.
			details, ok := detail["details"].([]any)
			if !ok {
				t.Fatalf("%s details = %#v, want array", testCase.route, detail["details"])
			}
			if len(details) != len(testCase.wantDetails) {
				t.Fatalf("%s details = %#v, want %#v", testCase.route, details, testCase.wantDetails)
			}
			for index, want := range testCase.wantDetails {
				if details[index] != want {
					t.Fatalf("%s details[%d] = %#v, want %#v", testCase.route, index, details[index], want)
				}
			}
		})
	}
}

// TestHubRoutesMethodNotAllowed: the three routes are hand-written switch
// cases, so a missing method guard would silently fall through to 404.
func TestHubRoutesMethodNotAllowed(t *testing.T) {
	stub := &hubOpsStub{}
	server := newHubOpsTestServer(t, stub)
	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/hub"},
		{http.MethodDelete, "/api/hub"},
		{http.MethodGet, "/api/hub/upgrade"},
		{http.MethodGet, "/api/hub/restart"},
		{http.MethodPut, "/api/hub/restart"},
	} {
		stub.statusCalls, stub.updateCalls, stub.restartCalls = 0, 0, 0
		response := request(t, server.Handler(), testCase.method, testCase.path)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, want 405; body=%s", testCase.method, testCase.path, response.Code, response.Body)
		}
		if stub.updateCalls != 0 || stub.restartCalls != 0 {
			t.Fatalf("%s %s reached HubOps: %#v", testCase.method, testCase.path, stub)
		}
		// At most one status read per request: writes pass the draining
		// gate first (it needs the phase), and it must never read twice.
		if stub.statusCalls > 1 {
			t.Fatalf("%s %s read HubStatus %d times, want at most 1", testCase.method, testCase.path, stub.statusCalls)
		}
	}
}

// TestHubRoutesRequireAdmin: /api/hub* must stay behind requireAuth and must
// NOT be added to principalAllowedForEndpoint. A machine secret is a data-plane
// credential; it must never be able to restart the hub (D8's last line).
func TestHubRoutesRequireAdmin(t *testing.T) {
	fixture := authFixture(t)
	stub := &hubOpsStub{}
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: fixture.home,
		Token:     "test-token",
		HubOps:    stub,
		AgentEndpointAuthorized: func(r *http.Request) bool {
			return r.Header.Get("Authorization") == "Bearer agent-secret-live"
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/hub"},
		{http.MethodPost, "/api/hub/upgrade"},
		{http.MethodPost, "/api/hub/restart"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			anonymous := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, anonymous)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s without credential = %d, want 401; body=%s", route.method, route.path, response.Code, response.Body)
			}

			agent := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			agent.Header.Set("Authorization", "Bearer agent-secret-live")
			response = httptest.NewRecorder()
			server.Handler().ServeHTTP(response, agent)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with a machine secret = %d, want 401; body=%s", route.method, route.path, response.Code, response.Body)
			}
		})
	}
	if stub.statusCalls+stub.updateCalls+stub.restartCalls != 0 {
		t.Fatalf("a rejected credential reached HubOps: %#v", stub)
	}
}

// TestHealthCarriesBootIdentity freezes D19's restart predicate: the console
// decides "the hub came back" only by bootId changing, so /api/health must
// carry bootId and startedAt even on an embedded server with no HubOps.
func TestHealthCarriesBootIdentity(t *testing.T) {
	hexBootID := regexp.MustCompile(`^[0-9a-f]{8}$`)
	assertBootIdentity := func(t *testing.T, body map[string]any) {
		t.Helper()
		bootID, ok := body["bootId"].(string)
		if !ok || !hexBootID.MatchString(bootID) {
			t.Fatalf("bootId = %#v, want 8 hex characters", body["bootId"])
		}
		startedAt, ok := body["startedAt"].(string)
		if !ok || startedAt == "" {
			t.Fatalf("startedAt = %#v", body["startedAt"])
		}
		parsed, err := time.Parse(time.RFC3339, startedAt)
		if err != nil || parsed.IsZero() {
			t.Fatalf("startedAt = %q: %v", startedAt, err)
		}
	}

	t.Run("without HubOps", func(t *testing.T) {
		fixture := authFixture(t)
		server := newWebServer(t, fixture, "test-token", nil, nil)
		// Health stays open and read-only: no credential, still 200.
		request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /api/health = %d, want 200; body=%s", response.Code, response.Body)
		}
		assertBootIdentity(t, decodeBody(t, response))
	})

	t.Run("HubOps is authoritative", func(t *testing.T) {
		stub := &hubOpsStub{status: HubStatus{Version: "1.5.0", BootID: "9f3c1a77", Phase: hubPhaseIdle}}
		server := newHubOpsTestServer(t, stub)
		response := request(t, server.Handler(), http.MethodGet, "/api/health")
		if response.Code != http.StatusOK {
			t.Fatalf("GET /api/health = %d, want 200; body=%s", response.Code, response.Body)
		}
		body := decodeBody(t, response)
		if body["bootId"] != "9f3c1a77" {
			t.Fatalf("bootId = %#v, want the HubOps BootID", body["bootId"])
		}
		if version := body["version"]; version != Version {
			t.Fatalf("version = %#v, want %q", version, Version)
		}
	})
}

// TestRestartingPhaseRejectsWrites freezes D11's draining rule: once the hub
// is draining into a restart, writes stop being accepted (503 restarting, a
// code distinct from write-busy) while reads keep working — the console polls
// reads through the outage to confirm the restart.
func TestRestartingPhaseRejectsWrites(t *testing.T) {
	stub := &hubOpsStub{
		status:  HubStatus{Version: "1.5.0", BootID: "9f3c1a77", Phase: hubPhaseRestarting},
		restart: HubRestartAccepted{RestartID: "r-1", BootID: "9f3c1a77"},
		updateResult: HubUpdateAccepted{
			Phase: hubPhaseChecking, BootID: "9f3c1a77",
		},
	}
	server := newHubOpsTestServer(t, stub)

	t.Run("writes are refused", func(t *testing.T) {
		for _, path := range []string{"/api/push", "/api/pull", "/api/sync", "/api/agents/revoke"} {
			stub.statusCalls = 0
			response := request(t, server.Handler(), http.MethodPost, path)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("POST %s while restarting = %d, want 503; body=%s", path, response.Code, response.Body)
			}
			body := decodeBody(t, response)
			detail, ok := body["error"].(map[string]any)
			if !ok || detail["code"] != hubCodeRestarting {
				t.Fatalf("POST %s code = %#v, want %q", path, body["error"], hubCodeRestarting)
			}
			if detail["message"] != "中心正在重启，请稍后重试" {
				t.Fatalf("POST %s message = %q", path, detail["message"])
			}
			// One status read per write: HubStatus takes the
			// implementation's mutex.
			if stub.statusCalls != 1 {
				t.Fatalf("POST %s read HubStatus %d times, want 1", path, stub.statusCalls)
			}
		}
	})

	t.Run("reads are untouched", func(t *testing.T) {
		for _, path := range []string{"/api/agents", "/api/hub"} {
			stub.statusCalls = 0
			response := request(t, server.Handler(), http.MethodGet, path)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s while restarting = %d, want 200; body=%s", path, response.Code, response.Body)
			}
			wantCalls := 1 // /api/hub reads it to answer; the gate must not add one.
			if path != "/api/hub" {
				wantCalls = 0
			}
			if stub.statusCalls != wantCalls {
				t.Fatalf("GET %s read HubStatus %d times, want %d", path, stub.statusCalls, wantCalls)
			}
		}
	})

	t.Run("hub actions do not lock themselves out", func(t *testing.T) {
		response := request(t, server.Handler(), http.MethodPost, "/api/hub/restart")
		if response.Code != http.StatusAccepted {
			t.Fatalf("POST /api/hub/restart while restarting = %d, want 202; body=%s", response.Code, response.Body)
		}
		response = request(t, server.Handler(), http.MethodPost, "/api/hub/upgrade")
		if response.Code != http.StatusAccepted {
			t.Fatalf("POST /api/hub/upgrade while restarting = %d, want 202; body=%s", response.Code, response.Body)
		}
	})

	t.Run("auth routes stay reachable", func(t *testing.T) {
		// An operator must still be able to log out of a hub that got stuck
		// draining; /api/auth/* is handled before the gate.
		response := request(t, server.Handler(), http.MethodPost, "/api/auth/logout")
		if response.Code != http.StatusOK {
			t.Fatalf("POST /api/auth/logout while restarting = %d, want 200; body=%s", response.Code, response.Body)
		}
	})
}

// TestIdlePhaseKeepsWritesOpen is the negative control for the draining gate:
// a non-restarting phase must not intercept writes, or every write in the
// product would start answering 503.
func TestIdlePhaseKeepsWritesOpen(t *testing.T) {
	stub := &hubOpsStub{status: HubStatus{Version: "1.5.0", Phase: hubPhaseDownloading}}
	server := newHubOpsTestServer(t, stub)
	// /api/agents/revoke with an unknown agent is a cheap authenticated
	// write that proves the gate let the request through.
	response := request(t, server.Handler(), http.MethodPost, "/api/agents/revoke?confirm=true", `{"agentId":"missing"}`)
	if response.Code == http.StatusServiceUnavailable {
		body := decodeBody(t, response)
		if detail, ok := body["error"].(map[string]any); ok && detail["code"] == hubCodeRestarting {
			t.Fatalf("write was refused with %q during phase %q", hubCodeRestarting, hubPhaseDownloading)
		}
	}
}

// TestHubStatusJSONOmitsEmptyOptionalFields keeps the console's truthiness
// checks honest: latest/note/rolledBack are optional on the wire, so an idle
// hub must not send `"latest":""`.
func TestHubStatusJSONOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(HubStatus{Version: "1.5.0", BootID: "9f3c1a77", Phase: hubPhaseIdle, CanUpdate: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"latest", "note", "rolledBack"} {
		if strings.Contains(string(data), `"`+absent+`"`) {
			t.Fatalf("idle HubStatus JSON contains %q: %s", absent, data)
		}
	}
}
