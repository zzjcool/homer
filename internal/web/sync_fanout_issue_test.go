package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zzjcool/homer-cli/internal/gens"
)

type fanoutTestAgentResult struct {
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped"`
	Error   string `json:"error"`
	Applied struct {
		Written   []json.RawMessage `json:"written"`
		Deleted   []json.RawMessage `json:"deleted"`
		Conflicts []json.RawMessage `json:"conflicts"`
	} `json:"applied"`
	Conflicts []struct {
		RelPath string `json:"relPath"`
		Local   string `json:"local"`
		Remote  string `json:"remote"`
	} `json:"conflicts"`
}

type fanoutTestReport struct {
	OK      bool                    `json:"ok"`
	Status  string                  `json:"status"`
	Applied int                     `json:"applied"`
	Skipped int                     `json:"skipped"`
	Errors  []string                `json:"errors"`
	Agents  []fanoutTestAgentResult `json:"agents"`
}

func publishFanoutGeneration(t *testing.T, home string) {
	t.Helper()
	if _, err := gens.New(home).Publish(map[string]map[string]string{
		"pi": {"settings/settings.json": "center\n"},
	}, []byte("{}")); err != nil {
		t.Fatal(err)
	}
}

func TestFanoutDispatchReturnsPerAgentApplyDetailsAndHumanErrors(t *testing.T) {
	fixture := makeFixture(t, "base\n", "base\n")
	publishFanoutGeneration(t, fixture.home)
	source := &sourceStub{
		list:    []AgentInfo{{AgentID: "box", Hostname: "box"}},
		pullRaw: json.RawMessage(`{"ok":false,"status":"conflicts-remain","applied":{"written":[{"adapterId":"pi","relPath":"new.json"}],"deleted":[{"adapterId":"pi","relPath":"old.json"}],"conflicts":[{"adapterId":"pi","relPath":"nested.json"}]},"conflicts":[{"adapterId":"pi","relPath":"settings.json","local":"local value","remote":"center value"}],"errors":["检测到 1 个冲突（已保留本地）"]}`),
	}
	server := newWebServer(t, fixture, "test-token", source, nil)
	response := syncPost(t, server.Handler(), "direction=dispatch&confirm=true")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dispatch = %d body=%s, want %d", response.Code, response.Body, http.StatusUnprocessableEntity)
	}
	var report fanoutTestReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode fanout response: %v", err)
	}
	if report.Status != "partial" || report.OK {
		t.Fatalf("report status/ok = %q/%v, want partial/false", report.Status, report.OK)
	}
	if len(report.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(report.Agents))
	}
	agent := report.Agents[0]
	if len(agent.Applied.Written) != 1 || len(agent.Applied.Deleted) != 1 || len(agent.Applied.Conflicts) != 1 {
		t.Fatalf("per-agent applied details were lost: %+v", agent.Applied)
	}
	if len(agent.Conflicts) != 1 || agent.Conflicts[0].RelPath != "settings.json" || agent.Conflicts[0].Local != "local value" || agent.Conflicts[0].Remote != "center value" {
		t.Fatalf("per-agent conflict details were lost: %+v", agent.Conflicts)
	}
	if agent.Error != "box：检测到 1 个冲突（已保留本地）" {
		t.Fatalf("agent error = %q, want the machine's human error", agent.Error)
	}
	if len(report.Errors) != 1 || report.Errors[0] != agent.Error {
		t.Fatalf("report errors = %#v, want the failed machine's human error", report.Errors)
	}
	if report.Applied != 1 || report.Skipped != 0 {
		t.Fatalf("applied/skipped = %d/%d, want 1/0", report.Applied, report.Skipped)
	}
}

func TestFanoutDispatchReportsWhenEveryMachineIsSkipped(t *testing.T) {
	fixture := makeFixture(t, "base\n", "base\n")
	publishFanoutGeneration(t, fixture.home)
	source := &sourceStub{list: []AgentInfo{
		{AgentID: "off-1", Hostname: "off-1", Stale: true},
		{AgentID: "off-2", Hostname: "off-2", Stale: true},
	}}
	server := newWebServer(t, fixture, "test-token", source, nil)
	response := syncPost(t, server.Handler(), "direction=dispatch&confirm=true")
	if response.Code != http.StatusOK {
		t.Fatalf("dispatch = %d body=%s, want %d", response.Code, response.Body, http.StatusOK)
	}
	var report fanoutTestReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode fanout response: %v", err)
	}
	if !report.OK || report.Status != "skipped" {
		t.Fatalf("report ok/status = %v/%q, want true/skipped", report.OK, report.Status)
	}
	if report.Applied != 0 || report.Skipped != 2 {
		t.Fatalf("applied/skipped = %d/%d, want 0/2", report.Applied, report.Skipped)
	}
	if len(source.pullValues) != 0 {
		t.Fatalf("pulled %d skipped machines, want 0", len(source.pullValues))
	}
}

func TestFanoutConsoleCopyCountsSkippedMachinesAndHidesStatusTokens(t *testing.T) {
	html := string(staticIndex)
	for _, required := range []string{
		`const skipped = agents.filter(agent => agent.skipped);`,
		`const received = agents.length - skipped.length;`,
		`if (received === 0) {`,
		`e.message = (data.errors || []).join("；") || "操作未完成";`,
	} {
		if !strings.Contains(html, required) {
			t.Errorf("console script is missing %q", required)
		}
	}
	if strings.Contains(html, `|| data.status ||`) {
		t.Fatal("console error envelope exposes the internal status token as user-facing copy")
	}
}
