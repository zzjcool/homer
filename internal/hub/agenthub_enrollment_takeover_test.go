package hub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zzjcool/homer/internal/stream"
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

// TestAgentHubAllowsReenrollAfterLostWelcome covers the recovery path the
// takeover guard must not break: a machine whose welcome frame never arrived
// holds no secret, so its hub-side binding is an orphan. Re-enrolling the
// same agentID with a fresh code must succeed instead of reporting
// agent-id-taken, otherwise the machine can never join without an
// administrator revoking it by hand.
func TestAgentHubAllowsReenrollAfterLostWelcome(t *testing.T) {
	const agentID = "lost-welcome-agent"

	enrollment := NewEnrollmentManager()
	firstCode, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, server := newTestAgentHub(t, NewRegistry(), enrollment, HubOptions{})

	// First attempt mints and binds a secret, but the agent never receives it
	// (simulating a dropped welcome), so nothing proves possession.
	firstSecret, alreadyBound, err := enrollment.RedeemAndBindIfUnbound(agentID, firstCode)
	if err != nil || alreadyBound {
		t.Fatalf("first redemption = (%q, %v, %v), want success", firstSecret, alreadyBound, err)
	}
	if enrollment.IsConfirmed(agentID) {
		t.Fatal("a freshly redeemed binding must stay unconfirmed until proven held")
	}

	secondCode, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	conn := dialTestAgent(t, server.URL, secondCode)
	response := sendEnrollmentHello(t, conn, agentID)
	if response.T != stream.TRes || !response.OK {
		t.Fatalf("re-enrollment after lost welcome = %+v, want success", response)
	}
	var welcome WelcomeResult
	if err := json.Unmarshal(response.P, &welcome); err != nil {
		t.Fatalf("decode welcome: %v", err)
	}
	if welcome.AgentSecret == "" || !enrollment.VerifySecret(agentID, welcome.AgentSecret) {
		t.Fatal("re-enrolled agent did not receive a bound per-agent secret")
	}
	if enrollment.VerifySecret(agentID, firstSecret) {
		t.Fatal("the undelivered first secret is still accepted")
	}

	// A machine that proves it holds its secret closes the window again.
	enrollment.MarkConfirmed(agentID)
	thirdCode, err := enrollment.Mint(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	conn = dialTestAgent(t, server.URL, thirdCode)
	response = sendEnrollmentHello(t, conn, agentID)
	if response.T != stream.TRes || response.E == nil || response.E.Code != "agent-id-taken" {
		code := "<nil>"
		if response.E != nil {
			code = response.E.Code
		}
		t.Fatalf("takeover after confirmation error code = %q, want agent-id-taken", code)
	}
	expectRemoteClose(t, awaitRemoteClose(conn), stream.CloseRevoked)
}
