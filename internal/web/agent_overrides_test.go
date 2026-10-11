package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/zzjcool/homer-cli/internal/cli/commands"
	"github.com/zzjcool/homer-cli/internal/gens"
	"github.com/zzjcool/homer-cli/internal/pluginruntime"
)

func TestAgentOverridesPersistenceRoundTrip(t *testing.T) {
	home := t.TempDir()
	overrides := LoadAgentOverrides(home)
	if err := overrides.SetDisabled("box", "vscode", true); err != nil {
		t.Fatal(err)
	}
	if err := overrides.SetDisabled("box", "pi", true); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, agentOverridesFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("agent overrides mode = %04o, want 0600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved agentOverridesDiskState
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode saved overrides: %v", err)
	}
	if saved.SchemaVersion != agentOverridesSchemaVersion || !saved.Disabled["box"]["pi"] || !saved.Disabled["box"]["vscode"] {
		t.Fatalf("saved override state = %+v", saved)
	}

	reloaded := LoadAgentOverrides(home)
	if !reloaded.Disabled("box", "pi") || !reloaded.Disabled("box", "vscode") {
		t.Fatalf("reloaded overrides = %#v", reloaded.DisabledAdapters("box"))
	}
	ids := reloaded.DisabledAdapters("box")
	if want := []string{"pi", "vscode"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("DisabledAdapters() = %#v, want %#v", ids, want)
	}
	ids[0] = "mutated"
	if got := reloaded.DisabledAdapters("box"); !reflect.DeepEqual(got, []string{"pi", "vscode"}) {
		t.Fatalf("DisabledAdapters() did not return a detached copy: %#v", got)
	}

	if err := reloaded.SetDisabled("box", "pi", false); err != nil {
		t.Fatal(err)
	}
	reloaded = LoadAgentOverrides(home)
	if reloaded.Disabled("box", "pi") || !reloaded.Disabled("box", "vscode") {
		t.Fatalf("enable did not remove only the selected override: %#v", reloaded.DisabledAdapters("box"))
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if _, exists := saved.Disabled["box"]["pi"]; exists || !saved.Disabled["box"]["vscode"] {
		t.Fatalf("enable persisted a non-disabled fact: %+v", saved.Disabled)
	}
}

func TestAgentOverridesRejectsInvalidIdentifiers(t *testing.T) {
	overrides := LoadAgentOverrides(t.TempDir())
	for _, testCase := range []struct {
		agentID   string
		adapterID string
	}{
		{agentID: "", adapterID: "pi"},
		{agentID: "  ", adapterID: "pi"},
		{agentID: "box", adapterID: ""},
		{agentID: "box", adapterID: "bad_id"},
	} {
		if err := overrides.SetDisabled(testCase.agentID, testCase.adapterID, true); err == nil {
			t.Errorf("SetDisabled(%q, %q, true) unexpectedly succeeded", testCase.agentID, testCase.adapterID)
		}
	}
}

func TestAgentOverridesConcurrentAccess(t *testing.T) {
	home := t.TempDir()
	overrides := LoadAgentOverrides(home)
	const workers = 12
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			agentID := fmt.Sprintf("box-%02d", index)
			if err := overrides.SetDisabled(agentID, "pi", true); err != nil {
				errors <- err
				return
			}
			for range 8 {
				if !overrides.Disabled(agentID, "pi") {
					errors <- fmt.Errorf("%s lost its disabled state", agentID)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	reloaded := LoadAgentOverrides(home)
	for index := 0; index < workers; index++ {
		agentID := fmt.Sprintf("box-%02d", index)
		if !overrides.Disabled(agentID, "pi") || !reloaded.Disabled(agentID, "pi") {
			t.Errorf("concurrent write for %s was lost in memory or on disk", agentID)
		}
	}
}

type overrideTestAgents struct {
	sourceStub
	reported map[string][]string
}

func (s *overrideTestAgents) AgentReportedAdapters(agentID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.reported[agentID]...)
}

func newOverrideTestServer(t *testing.T, home string, agents AgentsSource, overrides *AgentOverrides, plugins *pluginruntime.State) *Server {
	t.Helper()
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: home,
		Token:     "test-token",
		Agents:    agents,
		Overrides: overrides,
		Plugins:   plugins,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestAgentAdaptersAPIListsInstalledAndReportedAdapters(t *testing.T) {
	fixture := makeFixtureAtHome(t, t.TempDir(), "base\n")
	plugins := pluginruntime.NewWithLegacyDefault(fixture.home)
	overrides := LoadAgentOverrides(fixture.home)
	if err := overrides.SetDisabled("box", "pi", true); err != nil {
		t.Fatal(err)
	}
	agents := &overrideTestAgents{
		sourceStub: sourceStub{list: []AgentInfo{{AgentID: "box", Hostname: "box"}}},
		reported:   map[string][]string{"box": {"custom-editor", "pi", "keyring", "ssh-key"}},
	}
	server := newOverrideTestServer(t, fixture.home, agents, overrides, plugins)

	response := request(t, server.Handler(), http.MethodGet, "/api/agents/box/adapters")
	if response.Code != http.StatusOK {
		t.Fatalf("GET adapters = %d %s", response.Code, response.Body)
	}
	var body struct {
		Adapters []agentAdapterView `json:"adapters"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(body.Adapters))
	byID := make(map[string]agentAdapterView, len(body.Adapters))
	for _, adapter := range body.Adapters {
		ids = append(ids, adapter.ID)
		byID[adapter.ID] = adapter
	}
	if want := []string{"custom-editor", "herdr", "opencode", "pi", "vscode"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("adapter IDs = %#v, want %#v", ids, want)
	}
	if !byID["pi"].Disabled || byID["pi"].Name != "pi" {
		t.Fatalf("pi view = %+v, want disabled installed adapter", byID["pi"])
	}
	if byID["custom-editor"].Disabled || byID["custom-editor"].Name != "custom-editor" {
		t.Fatalf("reported adapter view = %+v", byID["custom-editor"])
	}
	if _, ok := byID["keyring"]; ok {
		t.Fatal("carrier plugin keyring must not appear as a per-machine adapter")
	}
	if _, ok := byID["ssh-key"]; ok {
		t.Fatal("action plugin ssh-key must not appear as a per-machine adapter")
	}
}

func TestAgentAdapterOverrideAPIRoleValidationAndEnable(t *testing.T) {
	fixture := makeFixtureAtHome(t, t.TempDir(), "base\n")
	plugins := pluginruntime.NewWithLegacyDefault(fixture.home)
	overrides := LoadAgentOverrides(fixture.home)
	agents := &overrideTestAgents{sourceStub: sourceStub{list: []AgentInfo{{AgentID: "box"}}}}
	server := newOverrideTestServer(t, fixture.home, agents, overrides, plugins)

	for _, adapterID := range []string{"keyring", "ssh-key", "not-installed", "bad_id"} {
		response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/adapters/"+adapterID, `{"disabled":true}`)
		if response.Code != http.StatusUnprocessableEntity || errorCode(t, response) != "override-invalid" {
			t.Errorf("disable %q = %d %s, want 422 override-invalid", adapterID, response.Code, response.Body)
		}
	}
	if response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/adapters/pi", `{}`); response.Code != http.StatusBadRequest || errorCode(t, response) != "override-invalid" {
		t.Fatalf("missing disabled body = %d %s, want 400 override-invalid", response.Code, response.Body)
	}

	if response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/adapters/pi", `{"disabled":true}`); response.Code != http.StatusOK {
		t.Fatalf("disable installed adapter = %d %s", response.Code, response.Body)
	}
	if !overrides.Disabled("box", "pi") {
		t.Fatal("successful disable was not persisted")
	}
	if response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/adapters/pi", `{"disabled":false}`); response.Code != http.StatusOK {
		t.Fatalf("enable installed adapter = %d %s", response.Code, response.Body)
	}
	if overrides.Disabled("box", "pi") {
		t.Fatal("enable did not remove the disabled fact")
	}

	// Removing a stale override is allowed even if its former plugin is no
	// longer installed; only the act of disabling requires an installed adapter role.
	if err := overrides.SetDisabled("box", "custom-editor", true); err != nil {
		t.Fatal(err)
	}
	if response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/adapters/custom-editor", `{"disabled":false}`); response.Code != http.StatusOK {
		t.Fatalf("clear stale override = %d %s", response.Code, response.Body)
	}
}

func TestAgentPullRejectsExplicitlyDisabledAdapter(t *testing.T) {
	home := t.TempDir()
	if _, err := gens.New(home).Publish(map[string]map[string]string{
		"pi": {"settings/settings.json": "center"},
	}, []byte(`{"adapters":{"pi":{"enabled":true}}}`)); err != nil {
		t.Fatal(err)
	}
	overrides := LoadAgentOverrides(home)
	if err := overrides.SetDisabled("box", "pi", true); err != nil {
		t.Fatal(err)
	}
	agents := &overrideTestAgents{
		sourceStub: sourceStub{
			list:    []AgentInfo{{AgentID: "box", Hostname: "box"}},
			pullRaw: json.RawMessage(`{"ok":true,"status":"synced"}`),
		},
	}
	server := newOverrideTestServer(t, home, agents, overrides, nil)

	response := request(t, server.Handler(), http.MethodPost, "/api/agents/box/pull", `{"adapters":["pi"]}`)
	if response.Code != http.StatusBadRequest || errorCode(t, response) != "adapter-disabled-for-agent" {
		t.Fatalf("disabled pull = %d %s, want 400 adapter-disabled-for-agent", response.Code, response.Body)
	}
	if !reflect.DeepEqual(agents.pulledAgent, []string(nil)) {
		t.Fatalf("pull was forwarded despite disabled adapter: %#v", agents.pulledAgent)
	}
	if !strings.Contains(response.Body.String(), "pi 已在这台机器停用") {
		t.Fatalf("disabled pull message = %s", response.Body)
	}
}

func TestFanoutFiltersDisabledAdaptersAndReportsSkipped(t *testing.T) {
	home := t.TempDir()
	publishOverrideTestCenter(t, home)
	overrides := LoadAgentOverrides(home)
	if err := overrides.SetDisabled("alpha", "editor", true); err != nil {
		t.Fatal(err)
	}
	for _, adapterID := range []string{"editor", "shell"} {
		if err := overrides.SetDisabled("beta", adapterID, true); err != nil {
			t.Fatal(err)
		}
	}
	agents := &overrideTestAgents{
		sourceStub: sourceStub{
			list:    []AgentInfo{{AgentID: "alpha", Hostname: "A"}, {AgentID: "beta", Hostname: "B"}},
			pullRaw: json.RawMessage(`{"ok":true,"status":"synced"}`),
		},
	}
	server := newOverrideTestServer(t, home, agents, overrides, nil)

	response := request(t, server.Handler(), http.MethodPost, "/api/sync?direction=dispatch&confirm=true", `{"adapters":["editor","shell"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("fanout dispatch = %d %s", response.Code, response.Body)
	}
	var report syncReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Agents) != 2 || report.Agents[0].AgentID != "alpha" || report.Agents[1].AgentID != "beta" {
		t.Fatalf("fanout results = %+v", report.Agents)
	}
	if !report.Agents[1].Skipped || report.Agents[1].Reason != "adapters disabled for this agent" {
		t.Fatalf("fully disabled machine result = %+v", report.Agents[1])
	}

	agents.mu.Lock()
	defer agents.mu.Unlock()
	if !reflect.DeepEqual(agents.pulledAgent, []string{"alpha"}) {
		t.Fatalf("pulled agents = %#v, want alpha only", agents.pulledAgent)
	}
	if len(agents.pullScopes) != 1 || !agents.pullScopes[0].Explicit || !reflect.DeepEqual(agents.pullScopes[0].Adapters, []string{"shell"}) {
		t.Fatalf("alpha pull scope = %#v, want explicit shell only", agents.pullScopes)
	}
}

func TestFanoutFiltersDefaultScopeUsingCenterAdapters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, err := gens.New(home).Publish(map[string]map[string]string{
		"shell": {"config/config.toml": "shell"},
	}, []byte(`{"version":1,"adapters":{"editor":{"root":"~/.editor","categories":{"config":{"paths":["settings.json"],"mode":"mirror"}}},"shell":{"root":"~/.shell","categories":{"config":{"paths":["config.toml"],"mode":"mirror"}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	overrides := LoadAgentOverrides(home)
	if err := overrides.SetDisabled("alpha", "editor", true); err != nil {
		t.Fatal(err)
	}
	agents := &overrideTestAgents{
		sourceStub: sourceStub{
			list:    []AgentInfo{{AgentID: "alpha", Hostname: "A"}},
			pullRaw: json.RawMessage(`{"ok":true,"status":"synced"}`),
		},
	}
	server := newOverrideTestServer(t, home, agents, overrides, nil)

	response := request(t, server.Handler(), http.MethodPost, "/api/sync?direction=dispatch&confirm=true", `{"configPolicy":{"editor":"center","shell":"keep"}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("default-scope fanout = %d %s", response.Code, response.Body)
	}
	if len(agents.pulledAgent) != 1 || agents.pulledAgent[0] != "alpha" {
		t.Fatalf("pulled agents = %#v", agents.pulledAgent)
	}
	if len(agents.pullScopes) != 1 {
		t.Fatalf("pull scopes = %#v", agents.pullScopes)
	}
	got := agents.pullScopes[0]
	if !got.Explicit || !reflect.DeepEqual(got.Adapters, []string{"shell"}) {
		t.Fatalf("default scope after filtering = %+v, want explicit shell", got)
	}
	if !reflect.DeepEqual(got.ConfigPolicy, map[string]string{"shell": "keep"}) {
		t.Fatalf("configPolicy after filtering = %#v, want only shell", got.ConfigPolicy)
	}
}

func TestCollectChoicesKeepDisabledAdapterVisibleButDisabled(t *testing.T) {
	fixture := makeFixtureAtHome(t, t.TempDir(), "base\n")
	overrides := LoadAgentOverrides(fixture.home)
	if err := overrides.SetDisabled("box", "pi", true); err != nil {
		t.Fatal(err)
	}
	server := newOverrideTestServer(t, fixture.home, nil, overrides, nil)
	report := commands.StatusReport{Adapters: []commands.StatusAdapterReport{
		{ID: "pi", Push: 2},
		{ID: "herdr", Push: 1},
	}}

	response := server.buildCollectChoicesResponse("box", report, nil, nil)
	if len(response.Adapters) != 2 {
		t.Fatalf("collect rows = %#v, want both visible", response.Adapters)
	}
	var pi AdapterChoice
	for _, choice := range response.Adapters {
		if choice.ID == "pi" {
			pi = choice
		}
	}
	if pi.ID == "" || pi.Enabled || pi.Checked || pi.Reason != "已在这台机器停用" {
		t.Fatalf("disabled collect row = %+v", pi)
	}
}

func TestSnapshotMetaRemainsGenerationOwnedWithoutMachineIdentity(t *testing.T) {
	home := t.TempDir()
	meta := []byte(`{"version":1,"adapters":{"pi":{"enabled":true}}}`)
	generation, err := gens.New(home).Publish(map[string]map[string]string{
		"pi": {"settings/settings.json": "center"},
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	overrides := LoadAgentOverrides(home)
	if err := overrides.SetDisabled("box", "pi", true); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: home,
		Token:     "test-token",
		Overrides: overrides,
		AgentIDOfRequest: func(*http.Request) (string, bool) {
			return "", false // shared hub-token requests have no machine identity
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	response := request(t, server.Handler(), http.MethodGet, "/api/snapshot")
	if response.Code != http.StatusOK {
		t.Fatalf("snapshot download = %d %s", response.Code, response.Body)
	}
	var payload snapshotPayload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Generation != generation || payload.HomerJSON != string(meta) {
		t.Fatalf("snapshot meta was rewritten without a machine identity: generation=%d meta=%s", payload.Generation, payload.HomerJSON)
	}
}

func publishOverrideTestCenter(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	_, err := gens.New(home).Publish(map[string]map[string]string{
		"editor": {"settings/settings.json": "editor"},
		"shell":  {"config/config.toml": "shell"},
	}, []byte(`{"version":1,"adapters":{"editor":{"root":"~/.editor","categories":{"config":{"paths":["settings.json"],"mode":"mirror"}}},"shell":{"root":"~/.shell","categories":{"config":{"paths":["config.toml"],"mode":"mirror"}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
}

var _ AgentsSource = (*overrideTestAgents)(nil)
var _ ReportedAdaptersSource = (*overrideTestAgents)(nil)
