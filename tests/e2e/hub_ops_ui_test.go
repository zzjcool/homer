package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/zzjcool/homer/internal/core"
	"github.com/zzjcool/homer/internal/web"
)

// hubOpsAgentSource is the smallest AgentsSource that renders an empty
// console: this suite is about the hub dialog, not about machines.
type hubOpsAgentSource struct{}

func (hubOpsAgentSource) ListAgents() []web.AgentInfo { return nil }
func (hubOpsAgentSource) AgentStatus(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"adapters":[],"errors":[]}`), nil
}
func (hubOpsAgentSource) AgentDiff(context.Context, string, web.DiffParams) (string, error) {
	return "", nil
}
func (hubOpsAgentSource) AgentPush(context.Context, string, bool, web.SyncScope) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true,"status":"pushed"}`), nil
}
func (hubOpsAgentSource) AgentPull(context.Context, string, bool, web.SyncScope) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true,"status":"applied"}`), nil
}
func (hubOpsAgentSource) RemoveAgent(string) bool { return false }

// hubOpsInitScript answers the hub's own three routes (and /api/health) inside
// the page, in exactly the JSON shapes frozen by internal/web/hubops.go.
//
// The dialog under test only ever talks to those routes, so stubbing them in
// the page keeps this suite about the console contract (D15–D19) instead of
// about the router. It also lets the test make the hub "go away and come back
// with a new bootId", which a real process cannot do inside a browser test.
const hubOpsInitScript = `
window.__confirms = [];
window.confirm = (m) => { window.__confirms.push(String(m)); return true; };
window.__copied = [];
window.prompt = () => null;
Object.defineProperty(navigator, 'clipboard', { configurable: true,
  value: { writeText: (t) => { window.__copied.push(String(t)); return Promise.resolve(); } } });

(() => {
  const H = {
    version: "v1.4.0", bootId: "0ld0boot", phase: "idle", canUpdate: true,
    latest: "v1.5.0", downloaded: 0, total: 0, note: "", rolledBack: false,
    // outcome is what the pretend update ends with: ok | failed
    outcome: "ok",
    // failNote is the backend's explanation when outcome is failed. The two
    // frozen branches differ only by this sentence (there is no separate
    // machine-readable field), so the console must read it.
    failNote: "下载校验没有通过，新版本没有写入",
    // busy is what POST /api/hub/restart reports about a write in flight
    busy: false,
    nextVersion: "v1.5.0", nextBootId: "new0boot",
    pending: null,        // "upgrade" | "restart" while a restart is coming
    phaseAfterPost: 0,    // polls of GET /api/hub since the POST
    drainPolls: 0,        // health polls the OLD process still answers
    healthMisses: 0,      // health polls that fail before the hub is back
    posts: [],
    seenPhase: [],
    phaseLabels: [],
    consoleFetches: 0,
    toasts: [],
  };
  window.__hub = H;
  const json = (data, status) => Promise.resolve(new Response(JSON.stringify(data), {
    status, headers: { "Content-Type": "application/json" },
  }));
  const down = () => Promise.reject(new TypeError("Failed to fetch"));
  const realFetch = window.fetch.bind(window);
  window.fetch = (resource, init = {}) => {
    const url = typeof resource === "string" ? resource : resource.url;
    const method = (init.method || "GET").toUpperCase();
    if (url === "/api/hub" && method === "GET") {
      H.seenPhase.push(H.phase);
      if (H.pending === "upgrade") {
        H.phaseAfterPost += 1;
        if (H.outcome === "failed" && H.phaseAfterPost >= 1) {
          H.phase = "failed";
          H.note = H.failNote;
        } else if (H.phaseAfterPost <= 1) {
          H.phase = "downloading"; H.downloaded = 512 * 1024; H.total = 1024 * 1024;
        } else {
          H.phase = "restarting";
        }
      }
      return json({
        version: H.version, bootId: H.bootId, phase: H.phase, canUpdate: H.canUpdate,
        latest: H.latest, downloaded: H.downloaded, total: H.total,
        note: H.note, rolledBack: H.rolledBack,
      }, 200);
    }
    if (url === "/api/console") {
      H.consoleFetches += 1;
      return realFetch(resource, init);
    }
    if (url === "/api/hub/upgrade" && method === "POST") {
      H.posts.push({ url, method });
      H.pending = "upgrade"; H.phaseAfterPost = 0; H.healthMisses = 0; H.drainPolls = 0;
      return json({ phase: "checking", bootId: H.bootId }, 202);
    }
    if (url === "/api/hub/restart" && method === "POST") {
      H.posts.push({ url, method, busy: H.busy });
      H.pending = "restart"; H.phase = "restarting"; H.healthMisses = 0; H.drainPolls = 0;
      return json({ restartId: "rs-1", bootId: H.bootId, busy: H.busy }, 202);
    }
    if (url === "/api/health") {
      if (H.pending) {
        // The old process keeps answering health while it drains (contract
        // D11), so a 200 alone is NOT proof the restart happened — only a
        // changed bootId is (D19). Then it goes away for a moment: every
        // request rejects, which is the window the console must survive.
        if (H.drainPolls < 2) {
          H.drainPolls += 1;
          return json({ ok: true, status: "healthy", version: H.version, bootId: H.bootId }, 200);
        }
        if (H.healthMisses < 2) {
          H.healthMisses += 1;
          return down();
        }
        if (H.healthMisses === 2) {
          H.healthMisses = 3;
          H.bootId = H.nextBootId;
          if (H.pending === "upgrade") H.version = H.nextVersion;
          H.pending = null; H.phase = "idle"; H.note = "";
          H.latest = H.version === H.nextVersion ? "" : H.latest;
        }
        return json({ ok: true, status: "healthy", version: H.version, bootId: H.bootId }, 200);
      }
      return json({ ok: true, status: "healthy", version: H.version, bootId: H.bootId }, 200);
    }
    return realFetch(resource, init);
  };
  // Record every toast and every rendered phase label so short-lived states
  // stay assertable: the nodes keep their text after the state changes.
  setInterval(() => {
    const node = document.getElementById("toast");
    if (node) {
      const text = node.textContent || "";
      if (text && H.toasts[H.toasts.length - 1] !== text) H.toasts.push(text);
    }
    const phase = document.getElementById("hub-phase");
    if (phase) {
      const text = phase.textContent || "";
      if (text && H.phaseLabels[H.phaseLabels.length - 1] !== text) H.phaseLabels.push(text);
    }
  }, 40);
})();
`

type hubUI struct {
	t   *testing.T
	ctx context.Context
}

func (u *hubUI) eval(js string, out any) {
	u.t.Helper()
	if err := chromedp.Run(u.ctx, chromedp.Evaluate(js, out)); err != nil {
		u.t.Fatalf("eval %s: %v\npage: %s", js, err, browserText(u.ctx))
	}
}

// evalAsync awaits the promise returned by js before reading the result.
func (u *hubUI) evalAsync(js string, out any) {
	u.t.Helper()
	action := chromedp.Evaluate(js, out, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})
	if err := chromedp.Run(u.ctx, action); err != nil {
		u.t.Fatalf("eval async %s: %v\npage: %s", js, err, browserText(u.ctx))
	}
}

func (u *hubUI) text(js string) string {
	u.t.Helper()
	var out string
	u.eval(`(() => { const v = (`+js+`); return v == null ? "" : String(v); })()`, &out)
	return out
}

func (u *hubUI) wait(expr string, timeout time.Duration) {
	u.t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var ready bool
		err := chromedp.Run(u.ctx, chromedp.Evaluate(`!!(`+expr+`)`, &ready))
		if err == nil && ready {
			return
		}
		if err != nil {
			last = err.Error()
		}
		time.Sleep(100 * time.Millisecond)
	}
	u.t.Fatalf("timed out waiting for %s (%s)\npage: %s", expr, last, browserText(u.ctx))
}

func (u *hubUI) click(js string) {
	u.t.Helper()
	got := u.text(`(() => { const el = ` + js + `; if (!el) return 'missing'; if (el.disabled) return 'disabled'; el.click(); return 'ok' })()`)
	if got != "ok" {
		u.t.Fatalf("click %s = %s\npage: %s", js, got, browserText(u.ctx))
	}
}

// setHub mutates the page's pretend hub state (see hubOpsInitScript).
func (u *hubUI) setHub(js string) {
	u.t.Helper()
	u.eval(`(() => { const H = window.__hub; `+js+`; return true })()`, nil)
}

// TestConsoleHubOps drives the hub entry and its one-click restart/update
// flows in a real browser. It covers contract D15–D19: header-only entry,
// dialog contents, the frozen confirmation copy, bootId-based restart
// detection, the 30s refresh standing down while the hub is away, and the
// two failure explanations.
func TestConsoleHubOps(t *testing.T) {
	if _, err := chromiumExecutable(t); err != nil {
		t.Skipf("chromium is not installed: %v", err)
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	homerHome := filepath.Join(root, ".homer")
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return homerHome
		}
		return ""
	})
	if err := core.SaveConfig(paths, core.HomerConfig{Version: 1, Adapters: map[string]core.AdapterConfig{}}); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := web.NewServer(web.ServeOptions{Addr: "127.0.0.1:0", HomerHome: homerHome, Agents: hubOpsAgentSource{}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: server.Handler()}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() { _ = httpServer.Close() })
	pageURL := "http://" + listener.Addr().String() + "/"

	browser, cancel := newKeyBrowser(t)
	defer cancel()
	ctx, stop := context.WithTimeout(browser, 180*time.Second)
	defer stop()
	ui := &hubUI{t: t, ctx: ctx}

	// Any uncaught error on the page fails the test: a console that throws
	// while a restart is in flight would otherwise look merely "empty".
	var errMu sync.Mutex
	var pageErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if exception, ok := ev.(*runtime.EventExceptionThrown); ok {
			errMu.Lock()
			pageErrors = append(pageErrors, exception.ExceptionDetails.Error())
			errMu.Unlock()
		}
	})
	t.Cleanup(func() {
		errMu.Lock()
		defer errMu.Unlock()
		if len(pageErrors) > 0 {
			t.Errorf("the console threw: %v", pageErrors)
		}
	})

	const admin = "hub-ops-browser-admin-pw"
	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(hubOpsInitScript).Do(ctx)
			return err
		}),
		chromedp.Navigate(pageURL),
		chromedp.WaitVisible(`#in-setup-pw`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("open console: %v\n%s", err, browserText(ctx))
	}

	// D16: the entry is part of the signed-in header only. On the setup gate
	// it must be hidden, or it renders over the password form.
	if got := ui.text(`document.getElementById('btn-hub').classList.contains('hidden')`); got != "true" {
		t.Fatalf("the Hub button is visible on the setup gate (hidden = %s)\npage: %s", got, browserText(ctx))
	}

	err = chromedp.Run(ctx,
		chromedp.SendKeys(`#in-setup-pw`, admin, chromedp.ByQuery),
		chromedp.SendKeys(`#in-setup-pw2`, admin, chromedp.ByQuery),
		chromedp.Click(`#btn-setup`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("finish setup: %v\npage: %s", err, browserText(ctx))
	}
	ui.wait(`!document.getElementById('btn-hub').classList.contains('hidden')`, 20*time.Second)
	// D15: right after the plugin button, in the header, and not in the footer.
	if got := ui.text(`(() => { const h = document.querySelector('header.topbar'); return [...h.querySelectorAll('button')].map(b => b.id).join(',') })()`); !strings.Contains(got, "btn-plugins,btn-hub") {
		t.Fatalf("header buttons = %q, want btn-hub right after btn-plugins", got)
	}
	if got := ui.text(`document.querySelectorAll('footer .statusline button, footer button').length`); got != "0" {
		t.Fatalf("the footer status line carries %s buttons", got)
	}

	// 1. The dialog reports both versions, the phase and the update offer.
	ui.click(`document.getElementById('btn-hub')`)
	ui.wait(`document.getElementById('dlg-hub').open`, 5*time.Second)
	ui.wait(`document.getElementById('hub-current').textContent.includes('v1.4.0')`, 5*time.Second)
	dialog := ui.text(`document.getElementById('dlg-hub').innerText`)
	for _, want := range []string{"当前版本", "v1.4.0", "最新版本", "v1.5.0", "状态", "空闲", "更新并重启", "仅重启"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("hub dialog = %q, missing %q", dialog, want)
		}
	}

	// 2. 更新并重启: frozen confirmation, progress polling, then a restart
	// confirmed by the bootId change — not by the version.
	ui.click(`document.getElementById('btn-hub-upgrade')`)
	ui.wait(`window.__hub.posts.some(p => p.url === '/api/hub/upgrade')`, 10*time.Second)
	ui.wait(`window.__hub.toasts.some(m => m.includes('中心已重启'))`, 30*time.Second)
	asked := ui.text(`window.__confirms[0]`)
	for _, want := range []string{"把中心从 v1.4.0 更新到 v1.5.0", "中心会重启", "控制台大约 10 秒不可用"} {
		if !strings.Contains(asked, want) {
			t.Fatalf("the update confirmation %q does not mention %q", asked, want)
		}
	}
	if strings.Contains(asked, "机器离线") {
		t.Fatalf("the confirmation must warn that the console is unavailable, never that a machine is offline: %q", asked)
	}
	if got := ui.text(`window.__hub.toasts[window.__hub.toasts.length - 1]`); !strings.Contains(got, "中心已重启（v1.5.0）") {
		t.Fatalf("success notice = %q, want the new version", got)
	}
	if phases := ui.text(`window.__hub.seenPhase.join(',')`); !strings.Contains(phases, "downloading") {
		t.Fatalf("the console polled phases %q, want the download progress", phases)
	}
	// The dialog shows the phase as it moves: download progress, then restart.
	if labels := ui.text(`window.__hub.phaseLabels.join(' | ')`); !strings.Contains(labels, "正在下载新版本…") || !strings.Contains(labels, "正在重启中心…") {
		t.Fatalf("the dialog showed phases %q, want the download progress and then the restart", labels)
	}
	// D19: the header version is read once at boot, so a successful restart
	// has to reload it or the header keeps the old one forever.
	ui.wait(`document.getElementById('ver').textContent === 'v1.5.0'`, 10*time.Second)
	ui.wait(`document.getElementById('hub-current').textContent.includes('v1.5.0')`, 10*time.Second)
	// The 30s refresh stands down during the restart: the hero never flips to
	// "状态加载失败" while the hub is unreachable.
	if hero := ui.text(`document.getElementById('hero').innerText`); strings.Contains(hero, "状态加载失败") {
		t.Fatalf("the hero reported a failure while the hub was restarting: %q", hero)
	}
	// Directly pin the suppression (D19): the 30s interval and the
	// visibilitychange hook both just call refresh(), so a restarting flag must
	// make refresh() a no-op — otherwise the hero is clobbered by a failed poll.
	// (The stub's outage is only a second long, so waiting for the real 30s tick
	// would test nothing.)
	var consoleFetchesDuringRestart int
	ui.evalAsync(`(async () => {
		const before = window.__hub.consoleFetches;
		app.restarting = true;
		try { await refresh(); } finally { app.restarting = false; }
		return window.__hub.consoleFetches - before;
	})()`, &consoleFetchesDuringRestart)
	if consoleFetchesDuringRestart != 0 {
		t.Fatalf("refresh() hit /api/console %d times while the hub was restarting, want 0", consoleFetchesDuringRestart)
	}

	// 3. 仅重启 keeps the version and says so, and reports an interrupted sync
	// when the hub was busy (contract D19's "版本没有变化" branch).
	ui.setHub(`H.busy = true; H.nextBootId = 'boot-two'`)
	ui.click(`document.getElementById('btn-hub-restart')`)
	ui.wait(`window.__hub.posts.some(p => p.url === '/api/hub/restart')`, 10*time.Second)
	ui.wait(`window.__hub.toasts.some(m => m.includes('中心已重启，版本没有变化'))`, 30*time.Second)
	if asked := ui.text(`window.__confirms[1]`); asked != "重新启动中心？控制台大约 10 秒不可用，正在进行的同步会被中断。" {
		t.Fatalf("restart confirmation = %q, want the frozen sentence", asked)
	}
	if got := ui.text(`window.__hub.toasts.some(m => m.includes('有同步被中断'))`); got != "true" {
		t.Fatalf("a busy restart must tell the user a sync was interrupted (toasts: %s)", ui.text(`window.__hub.toasts.join(' | ')`))
	}

	// 4. A failed update explains itself and points at the next step. The
	// note decides the hint: nothing was written, so retrying is enough.
	ui.setHub(`H.outcome = 'failed'; H.latest = 'v1.6.0'`)
	ui.click(`document.getElementById('btn-hub')`)
	ui.wait(`document.getElementById('dlg-hub').open`, 5*time.Second)
	ui.click(`document.getElementById('btn-hub-upgrade')`)
	ui.wait(`document.getElementById('alert-slot').innerText.includes('中心没有更新成功')`, 30*time.Second)
	alert := ui.text(`document.getElementById('alert-slot').innerText`)
	for _, want := range []string{"中心没有更新成功", "下载校验没有通过，新版本没有写入", "可以直接重试"} {
		if !strings.Contains(alert, want) {
			t.Fatalf("failed-update alert = %q, missing %q", alert, want)
		}
	}

	// 4b. The other failed branch (the new binary IS installed but the hub did
	// not come back) must point at the restart instead of inviting a retry.
	ui.setHub(`H.failNote = '新版本已装好，但中心没有重启；请在服务器上重新执行 homer serve'`)
	ui.click(`document.getElementById('btn-hub')`)
	ui.wait(`document.getElementById('dlg-hub').open`, 5*time.Second)
	ui.click(`document.getElementById('btn-hub-upgrade')`)
	ui.wait(`document.getElementById('alert-slot').innerText.includes('新版本已经装好')`, 30*time.Second)
	if alert := ui.text(`document.getElementById('alert-slot').innerText`); strings.Contains(alert, "可以直接重试") {
		t.Fatalf("an installed-but-not-restarted hub must not invite a re-download: %q", alert)
	}

	// 5. A rollback is surfaced instead of quietly showing the old version.
	ui.setHub(`H.rolledBack = true; H.pending = null; H.phase = 'idle'; H.note = ''`)
	ui.click(`document.getElementById('btn-hub')`)
	ui.wait(`document.getElementById('dlg-hub').open`, 5*time.Second)
	ui.wait(`!document.getElementById('hub-rollback').classList.contains('hidden')`, 5*time.Second)
	if got := ui.text(`document.getElementById('hub-rollback').textContent`); !strings.Contains(got, "已回滚到上一个版本") {
		t.Fatalf("rollback notice = %q", got)
	}
}
