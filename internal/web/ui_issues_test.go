package web

import (
	"strings"
	"testing"
)

func uiIssueSection(t *testing.T, source, start, end string) string {
	t.Helper()
	startAt := strings.Index(source, start)
	if startAt < 0 {
		t.Fatalf("index.html is missing section start %q", start)
	}
	endAt := strings.Index(source[startAt+len(start):], end)
	if endAt < 0 {
		t.Fatalf("index.html section %q is missing end %q", start, end)
	}
	return source[startAt : startAt+len(start)+endAt]
}

func requireUIIssueStrings(t *testing.T, section string, required ...string) {
	t.Helper()
	for _, value := range required {
		if !strings.Contains(section, value) {
			t.Errorf("index.html section is missing %q", value)
		}
	}
}

func TestCopyJoinFallsBackWhenClipboardIsUnavailable(t *testing.T) {
	source := string(staticIndex)
	copyJoin := uiIssueSection(t, source, "function copyJoin(inputId, buttonId) {", "\n$(\"btn-copy\")")
	requireUIIssueStrings(t, copyJoin,
		"navigator.clipboard && navigator.clipboard.writeText",
		"try {",
		"catch (_) {",
		"input.focus(); input.select();",
		"$(\"copy-fallback\").classList.remove(\"hidden\")",
	)
}

func TestLogoutFailureStaysInWorkspace(t *testing.T) {
	source := string(staticIndex)
	logout := uiIssueSection(t, source, "$(\"btn-logout\").addEventListener(\"click\", async () => {", "\n$(\"btn-refresh\")")
	requireUIIssueStrings(t, logout,
		"catch (e) {",
		"toast(\"退出失败",
	)
	tryAt := strings.Index(logout, "try {")
	logoutAt := strings.Index(logout, "await api(\"/api/auth/logout\"")
	clearAt := strings.Index(logout, "app.console = null;")
	gateAt := strings.Index(logout, "showGate(\"login\");")
	catchAt := strings.Index(logout, "catch (e) {")
	if tryAt < 0 || logoutAt < tryAt || clearAt < logoutAt || gateAt < clearAt || catchAt < gateAt {
		t.Fatalf("logout must clear the session and show the login gate only after the request succeeds")
	}
}

func TestJoinCodePollingShowsRepeatedFailures(t *testing.T) {
	source := string(staticIndex)
	watch := uiIssueSection(t, source, "function watchJoinCode(code) {", "\nasync function openJoin()")
	requireUIIssueStrings(t, watch,
		"let failures = 0",
		"if (failures < 3) return;",
		"join-err",
		"检测不到接入状态（可能是网络问题）",
		"stopJoinWatch();",
	)
	requireUIIssueStrings(t, source,
		"id=\"join-err\"",
		"id=\"join-wait\"",
		"连接状态检测正常时，机器接入后窗口会自动关闭；若检测中断，请按提示手动完成。",
	)
}

func TestManualRefreshIsBusyGuardedWithoutRemovingAutoRefresh(t *testing.T) {
	source := string(staticIndex)
	refresh := uiIssueSection(t, source, "async function refresh(", "\nfunction loadAgentsRender")
	requireUIIssueStrings(t, refresh,
		"if (app.refreshing) {",
		"app.refreshing = true;",
		"finally",
		"btn.disabled = true",
		"setAttribute(\"aria-busy\", \"true\")",
	)
	requireUIIssueStrings(t, source,
		"$(\"btn-refresh\").addEventListener(\"click\", () => refresh(\"manual\"))",
		"setInterval(() => { if (!document.hidden && workspaceVisible()) refresh(); }, 30000)",
		"document.addEventListener(\"visibilitychange\", () => { if (!document.hidden && workspaceVisible()) refresh(); })",
	)
}
