package hub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zzjcool/homer-cli/internal/stream"
)

func sendEnrollmentHello(t *testing.T, conn stream.Conn, agentID string) *stream.Frame {
	t.Helper()
	payload, err := json.Marshal(HelloParams{Proto: stream.ProtocolVersion, AgentID: agentID})
	if err != nil {
		t.Fatal(err)
	}
	sendTestFrame(t, conn, &stream.Frame{T: stream.TReq, ID: "hello-enrollment-test", M: MethodHello, P: payload})
	return readTestFrame(t, conn)
}

func TestAgentHubEnrollmentCodeAllowsUnboundAgentID(t *testing.T) {
	enrollment := NewEnrollmentManager()
	code, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, server := newTestAgentHub(t, NewRegistry(), enrollment, HubOptions{})

	conn := dialTestAgent(t, server.URL, code)
	response := sendEnrollmentHello(t, conn, "new-agent")
	if response.T != stream.TRes || !response.OK {
		t.Fatalf("enrollment hello response = %+v, want success", response)
	}
	var welcome WelcomeResult
	if err := json.Unmarshal(response.P, &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}
	if welcome.AgentSecret == "" || !enrollment.VerifySecret("new-agent", welcome.AgentSecret) {
		t.Fatal("new agent did not receive a bound per-agent secret")
	}
	if enrollment.ValidCode(code) {
		t.Fatal("successful enrollment did not consume its one-time code")
	}
}

func TestAgentHubEnrollmentCodeAllowsRevokedAgentID(t *testing.T) {
	const agentID = "revoked-agent"
	const oldSecret = "revoked-machine-secret"

	enrollment := NewEnrollmentManager()
	enrollment.BindAgent(agentID, oldSecret)
	if !enrollment.Revoke(agentID) {
		t.Fatal("failed to revoke existing agent secret")
	}
	code, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, server := newTestAgentHub(t, NewRegistry(), enrollment, HubOptions{})

	conn := dialTestAgent(t, server.URL, code)
	response := sendEnrollmentHello(t, conn, agentID)
	if response.T != stream.TRes || !response.OK {
		t.Fatalf("re-enrollment hello response = %+v, want success", response)
	}
	var welcome WelcomeResult
	if err := json.Unmarshal(response.P, &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}
	if welcome.AgentSecret == "" || !enrollment.VerifySecret(agentID, welcome.AgentSecret) {
		t.Fatal("revoked agent did not receive a new bound per-agent secret")
	}
	if enrollment.VerifySecret(agentID, oldSecret) {
		t.Fatal("revoked agent's old secret became valid again")
	}
}

func TestAgentHubRejectsEnrollmentCodeForBoundAgentID(t *testing.T) {
	const agentID = "already-enrolled-agent"
	const existingSecret = "victim-machine-secret"

	enrollment := NewEnrollmentManager()
	enrollment.BindAgent(agentID, existingSecret)
	code, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, server := newTestAgentHub(t, NewRegistry(), enrollment, HubOptions{})

	conn := dialTestAgent(t, server.URL, code)
	response := sendEnrollmentHello(t, conn, agentID)
	if response.T != stream.TRes || response.E == nil || response.E.Code != "agent-id-taken" {
		code := "<nil>"
		if response.E != nil {
			code = response.E.Code
		}
		t.Fatalf("enrollment hello error code = %q, want agent-id-taken", code)
	}
	if !enrollment.VerifySecret(agentID, existingSecret) {
		t.Fatal("rejected enrollment changed the existing agent secret")
	}
	if !enrollment.ValidCode(code) {
		t.Fatal("rejected enrollment burned the one-time code")
	}
	expectRemoteClose(t, awaitRemoteClose(conn), stream.CloseRevoked)
}
