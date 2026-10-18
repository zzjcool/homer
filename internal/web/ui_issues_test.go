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

// TestHubEntryIsHeaderOnlyAndHiddenOnGates pins contract D15/D16: the hub
// entry is a header text button (never the footer, never inside the plugin
// dialog), and it is toggled in BOTH showGate and showWorkspace — a button
// added to only one of them stays visible over the login form.
func TestHubEntryIsHeaderOnlyAndHiddenOnGates(t *testing.T) {
	source := string(staticIndex)
	headerStart := strings.Index(source, `<header class="topbar">`)
	headerEnd := strings.Index(source, `</header>`)
	if headerStart < 0 || headerEnd < headerStart {
		t.Fatal("index.html has no header")
	}
	header := source[headerStart:headerEnd]
	pluginsAt := strings.Index(header, `id="btn-plugins"`)
	hubAt := strings.Index(header, `id="btn-hub"`)
	if pluginsAt < 0 || hubAt < 0 {
		t.Fatalf("the header must carry both btn-plugins and btn-hub")
	}
	if hubAt < pluginsAt {
		t.Fatal("the Hub button belongs right after the plugin button")
	}
	if !strings.Contains(header[hubAt:], `type="button">Hub</button>`) {
		t.Fatalf("the Hub entry must read exactly Hub: %q", header[hubAt:])
	}

	// The footer is a status line (it is hidden segment-wise on small
	// screens), so a restart entry must never live there.
	footerAt := strings.Index(source, `<footer class="statusline">`)
	if footerAt < 0 {
		t.Fatal("index.html has no footer status line")
	}
	footerEnd := strings.Index(source[footerAt:], `</footer>`)
	if footerEnd < 0 {
		t.Fatal("the footer status line is not closed")
	}
	if strings.Contains(source[footerAt:footerAt+footerEnd], "btn-hub") {
		t.Fatal("the hub entry must not be in the footer")
	}
	// Nor inside the plugin dialog: that dialog's copy promises adapters,
	// carriers and machine actions only.
	pluginsDlg := uiIssueSection(t, source, `<dialog id="dlg-plugins"`, `</dialog>`)
	if strings.Contains(pluginsDlg, "btn-hub") || strings.Contains(pluginsDlg, "dlg-hub") {
		t.Fatal("hub ops must not be buried in the plugin dialog")
	}

	// The dialog itself sits after the last existing dialog and before the script.
	dialogAt := strings.Index(source, `<dialog id="dlg-hub"`)
	scriptAt := strings.Index(source, "\n<script>")
	if dialogAt < 0 || scriptAt < 0 {
		t.Fatalf("dlg-hub must exist before the inline script (dialog=%d script=%d)", dialogAt, scriptAt)
	}
	if dialogAt > scriptAt {
		t.Fatal("dlg-hub must be declared before the inline script")
	}
	if strings.Count(source[:scriptAt], "<dialog") != strings.Count(source[:scriptAt], "</dialog>") {
		t.Fatal("every dialog before the script must be closed")
	}

	gate := uiIssueSection(t, source, "function showGate(", "\nfunction showWorkspace(")
	workspace := uiIssueSection(t, source, "function showWorkspace(", "\n/* ---------- ① hero")
	requireUIIssueStrings(t, gate, `$("btn-hub").classList.add("hidden")`)
	requireUIIssueStrings(t, workspace, `$("btn-hub").classList.remove("hidden")`)
}

// TestHubListenersSitAfterPlugins pins contract D17: ui_issues_test.go slices
// the logout handler by string, so a listener inserted between btn-logout and
// btn-refresh silently pollutes that slice. Hub listeners go at the tail.
func TestHubListenersSitAfterPlugins(t *testing.T) {
	source := string(staticIndex)
	logoutAt := strings.Index(source, `$("btn-logout").addEventListener("click", async () => {`)
	refreshAt := strings.Index(source, `$("btn-refresh").addEventListener("click", () => refresh("manual"))`)
	pluginsAt := strings.Index(source, `$("btn-plugins").addEventListener("click", openPluginManager)`)
	if logoutAt < 0 || refreshAt < 0 || pluginsAt < 0 {
		t.Fatalf("console listener anchors moved (logout=%d refresh=%d plugins=%d)", logoutAt, refreshAt, pluginsAt)
	}
	for _, listener := range []string{
		`$("btn-hub").addEventListener("click", openHub)`,
		`$("btn-hub-upgrade").addEventListener("click", hubUpdateAndRestart)`,
		`$("btn-hub-restart").addEventListener("click", hubRestartNow)`,
	} {
		at := strings.Index(source, listener)
		if at < 0 {
			t.Fatalf("index.html is missing the hub listener %q", listener)
		}
		if at < pluginsAt {
			t.Fatalf("hub listener %q must come after the plugin listener", listener)
		}
	}
}

// TestHubRestartWaitsForANewBootId pins contract D19: the console decides a
// restart happened by comparing /api/health bootId, never by version (a pure
// restart keeps the version; a source build is always "newest").
func TestHubRestartWaitsForANewBootId(t *testing.T) {
	source := string(staticIndex)
	consts := uiIssueSection(t, source, "const HUB_RESTART_POLL_MS = 500;", "\nfunction hubSleep(")
	requireUIIssueStrings(t, consts,
		"const HUB_RESTART_TIMEOUT_MS = 60000;",
		"const HUB_UPDATE_POLL_MS = 1000;",
	)
	wait := uiIssueSection(t, source, "async function hubWaitForBoot(", "\nasync function hubAwaitRestart(")
	requireUIIssueStrings(t, wait,
		`fetch("/api/health"`,
		"if (!res.ok) continue;",
		"data.bootId && data.bootId !== oldBootId",
		"catch (_) {",
	)
	// An unknown baseline must not read "the old process still answers" as a
	// finished restart — the console resolves the old bootId first.
	requireUIIssueStrings(t, wait, "if (oldBootId && data && data.bootId && data.bootId !== oldBootId) {")
	requireUIIssueStrings(t, source[strings.Index(source, "async function hubOldBootId("):],
		`const status = await api("/api/hub")`,
		"return (status && status.bootId) || \"\";",
	)

	// Each poll needs its own try/catch: while the hub is down every request
	// rejects, and one unguarded await would abort the whole loop.
	loopStart := strings.Index(wait, "while (Date.now() < deadline)")
	if loopStart < 0 {
		t.Fatal("the restart check must poll on a deadline")
	}
	loopBody := wait[loopStart:]
	if !strings.Contains(loopBody, "try {") || !strings.Contains(loopBody, "catch (_) {") {
		t.Fatal("each restart poll must be wrapped in its own try/catch")
	}
	// The judgement itself must not read version: a pure restart keeps the
	// version unchanged, and a source build is newer than any release.
	judgement := loopBody[:strings.Index(loopBody, "return { ok: true")]
	if strings.Contains(judgement, "version") {
		t.Fatal("the restart check must not look at version: a pure restart keeps it unchanged")
	}

	restart := uiIssueSection(t, source, "async function hubAwaitRestart(", "\nasync function hubRestartNow(")
	requireUIIssueStrings(t, restart,
		"app.restarting = true;",
		"app.restarting = false;",
		"await loadHeaderVersion();",
		"await refresh();",
		"中心没有在预期时间内回来",
		"已经等待 60 秒，健康检查仍然失败。",
		"请在服务器上确认 homer 进程还在、查看它的日志；必要时重新执行 `homer serve`。",
		"重新加载页面",
		"中心已重启，版本没有变化",
	)
	requireUIIssueStrings(t, source, "正在重启中心…")
	// The unit name of the production service is not in this repository, so
	// naming it in the console would send operators to a unit that may not exist.
	if strings.Contains(source, "systemctl") {
		t.Fatal("the console must not name a systemd unit that this repository does not ship")
	}

	refreshed := uiIssueSection(t, source, "async function refresh(", "\nfunction loadAgentsRender")
	if !strings.Contains(refreshed, "if (app.restarting) return;") {
		t.Fatal("the 30s refresh must stand down while the hub is restarting")
	}
}

// TestHubDialogOffersBothActionsAndRolledBackNotice pins the dialog's frozen
// user-visible copy and its DOM construction convention (el()/textContent,
// never innerHTML).
func TestHubDialogOffersBothActionsAndRolledBackNotice(t *testing.T) {
	source := string(staticIndex)
	dialog := uiIssueSection(t, source, `<dialog id="dlg-hub"`, `</dialog>`)
	requireUIIssueStrings(t, dialog,
		`id="hub-current"`,
		`id="hub-latest"`,
		`id="hub-phase"`,
		`id="hub-progress-bar"`,
		`id="hub-note"`,
		`id="hub-rollback"`,
		`id="btn-hub-upgrade">更新并重启</button>`,
		`id="btn-hub-restart">仅重启</button>`,
		"已回滚到上一个版本",
	)

	render := uiIssueSection(t, source, "function renderHubDialog(status) {", "\nasync function openHub()")
	requireUIIssueStrings(t, render,
		`$("hub-latest").textContent = view.latest ? hubVersionText(view.latest) : "未知"`,
		`if (phase === "downloading")`,
		"hubProgressText(view)",
		`$("hub-rollback").classList.toggle("hidden", view.rolledBack !== true)`,
	)
	phases := uiIssueSection(t, source, "function hubPhaseText(phase) {", "\nfunction hubProgressPercent(status)")
	requireUIIssueStrings(t, phases, "正在重启中心…", "正在下载新版本…", "正在安装新版本…")
	if strings.Contains(render, "innerHTML") {
		t.Fatal("the hub dialog must build DOM with el()/textContent, not innerHTML")
	}

	question := uiIssueSection(t, source, "function hubUpdateQuestion(status) {", "\n// 重启成功判据只看 bootId")
	requireUIIssueStrings(t, question,
		"把中心从 ",
		" 更新到 ",
		"？下载完成后中心会重启，控制台大约 10 秒不可用。",
		"更新到最新版本？会先检查再下载；完成后中心会重启，控制台大约 10 秒不可用。",
	)
	// D18: no administrator password re-prompt — the cookie is already admin.
	flow := uiIssueSection(t, source, "async function hubRestartNow() {", "\n/* ---------- theme")
	if strings.Contains(flow, "prompt(") || strings.Contains(flow, "password") {
		t.Fatal("the hub flows must not ask for a password on top of the admin session")
	}
	if strings.Contains(source, "机器离线") {
		t.Fatal("the console must warn that the console is unavailable, never that a machine went offline")
	}
}

// TestHubUpdatePollsItsOwnProgress pins contract D5/D19's progress loop: the
// console reloads /api/hub once a second and hands over to the health poll as
// soon as the hub says it is restarting, because from then on /api/hub is gone.
func TestHubUpdatePollsItsOwnProgress(t *testing.T) {
	source := string(staticIndex)
	watch := uiIssueSection(t, source, "async function hubWatchUpdate(", "\n// 后端只用 note 区分两条失败分支")
	requireUIIssueStrings(t, watch,
		`status = await api("/api/hub")`,
		`if (status.phase === "restarting")`,
		"await hubAwaitRestart(oldBootId, versionBefore);",
		`if (status.phase === "failed")`,
		"hubShowUpdateFailure(status)",
		`if (status.phase === "idle")`,
	)
	failure := uiIssueSection(t, source, "function hubShowUpdateFailure(status) {", "\n$(")
	requireUIIssueStrings(t, failure,
		"中心没有更新成功",
		"hubUpdateFailureHint(note)",
	)
	hint := uiIssueSection(t, source, "function hubUpdateFailureHint(note) {", "\nfunction hubShowUpdateFailure(status) {")
	requireUIIssueStrings(t, hint,
		"新版本已经装好，重启中心就会生效。",
		"这次没有替换任何文件，可以直接重试。",
	)
	// The dialog is modal, so the page-level banner would be hidden behind it.
	if !strings.Contains(source, `if (dlg.open) dlg.close();`) {
		t.Fatal("the hub dialog must close before reporting a failure through the page alert")
	}
	// Contract D11: a restart accepted while a write was in flight says so.
	requireUIIssueStrings(t, uiIssueSection(t, source, "async function hubRestartNow() {", "\nasync function hubUpdateAndRestart() {"),
		"accepted.busy === true",
		"有同步被中断",
	)
}
