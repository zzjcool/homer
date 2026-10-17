package web

import (
	"encoding/json"
	"testing"
)

type credentialStatusStub struct {
	testEnrollmentService
	enrolled map[string]bool
}

func (s credentialStatusStub) HasSecret(agentID string) bool {
	return s.enrolled[agentID]
}

func TestAgentCredentialStatusIsIncludedInMachineLists(t *testing.T) {
	fixture := makeFixture(t, "base\n", "base\n")
	source := &sourceStub{list: []AgentInfo{
		{AgentID: "active", Hostname: "active-box"},
		{AgentID: "revoked", Hostname: "revoked-box"},
	}}
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: fixture.home,
		Token:     "test-token",
		Agents:    source,
		Enrollment: credentialStatusStub{enrolled: map[string]bool{
			"active": true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []struct {
		path  string
		field string
	}{
		{path: "/api/agents", field: "agents"},
		{path: "/api/console", field: "machines"},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			response := request(t, server.Handler(), "GET", endpoint.path)
			if response.Code != 200 {
				t.Fatalf("GET %s = %d: %s", endpoint.path, response.Code, response.Body)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s response: %v", endpoint.path, err)
			}
			var machines []struct {
				AgentID  string `json:"agentId"`
				Enrolled *bool  `json:"enrolled"`
			}
			if err := json.Unmarshal(body[endpoint.field], &machines); err != nil {
				t.Fatalf("decode %s.%s: %v", endpoint.path, endpoint.field, err)
			}
			if len(machines) != 2 {
				t.Fatalf("%s returned %d machines, want 2", endpoint.path, len(machines))
			}
			got := make(map[string]*bool, len(machines))
			for _, machine := range machines {
				got[machine.AgentID] = machine.Enrolled
			}
			for id, want := range map[string]bool{"active": true, "revoked": false} {
				if got[id] == nil || *got[id] != want {
					t.Errorf("%s machine %q enrolled = %v, want %t", endpoint.path, id, got[id], want)
				}
			}
		})
	}
}
