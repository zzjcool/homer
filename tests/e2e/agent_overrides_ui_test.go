package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/zzjcool/homer/internal/core"
	"github.com/zzjcool/homer/internal/web"
)

func TestAgentAdapterOverridesUI(t *testing.T) {
	if _, err := chromiumExecutable(t); err != nil {
		t.Skipf("chromium is not installed: %v", err)
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	homer := filepath.Join(root, ".homer")
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return homer
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
	server, err := web.NewServer(web.ServeOptions{
		Addr: "127.0.0.1:0", HomerHome: homer, Agents: agentOverridesUIAgentSource{},
	})
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

	const admin = "adapter-overrides-browser-admin-password"
	err = chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.WaitVisible(`#in-setup-pw`, chromedp.ByQuery),
		chromedp.SendKeys(`#in-setup-pw`, admin, chromedp.ByQuery),
		chromedp.SendKeys(`#in-setup-pw2`, admin, chromedp.ByQuery),
		chromedp.Click(`#btn-setup`, chromedp.ByQuery),
		chromedp.WaitVisible(`//div[@id="agent-list"]//button[normalize-space()="查看"]`, chromedp.BySearch),
	)
	if err != nil {
		t.Fatalf("browser setup: %v\npage:\n%s", err, browserText(ctx))
	}

	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		window.__agentOverridesUIRequests = [];
		window.__agentOverridesDisabled = false;
		const fetchOriginal = window.fetch.bind(window);
		const jsonResponse = (data, status = 200) => new Response(JSON.stringify(data), {
			status,
			headers: { "Content-Type": "application/json" },
		});
		window.fetch = (resource, init = {}) => {
			const url = typeof resource === "string" ? resource : resource.url;
			const method = (init.method || "GET").toUpperCase();
			if (url === "/api/agents/ui-box/adapters") {
				window.__agentOverridesUIRequests.push({ url, method, body: init.body ? JSON.parse(init.body) : null });
				if (method === "GET") {
					// 后端按 role 过滤：keyring（carrier）永不出现在 GET 响应里，
					// mock 与真实实现保持同形状（reviewer minor：不要用真实后端
					// 不可能产生的输入做断言）。
					return Promise.resolve(jsonResponse({ adapters: [
						{ id: "pi", name: "pi", disabled: window.__agentOverridesDisabled, installed: true },
					] }));
				}
			}
			if (url === "/api/agents/ui-box/adapters/pi" && method === "POST") {
				const body = init.body ? JSON.parse(init.body) : {};
				window.__agentOverridesUIRequests.push({ url, method, body });
				window.__agentOverridesDisabled = body.disabled === true;
				return Promise.resolve(jsonResponse({ ok: true }));
			}
			if (url.startsWith("/api/sync/choices?direction=collect&agent=ui-box&stream=1")) {
				window.__agentOverridesUIRequests.push({ url, method, body: null });
				const event = {
					type: "done",
					hint: "选择要收取的适配器。",
					adapters: [{
						id: "pi",
						enabled: false,
						reason: "已在这台机器停用",
						checked: true,
						categories: [],
					}],
				};
				return Promise.resolve(new Response(JSON.stringify(event) + "\n", {
					status: 200,
					headers: { "Content-Type": "application/x-ndjson" },
				}));
			}
			return fetchOriginal(resource, init);
		};
	})()`, nil)); err != nil {
		t.Fatalf("install adapter override API mocks: %v", err)
	}

	err = chromedp.Run(ctx,
		chromedp.Click(`//div[@id="agent-list"]//button[normalize-space()="查看"]`, chromedp.BySearch),
		chromedp.WaitVisible(`#view-body [data-adapter-override="pi"]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("open machine view: %v\npage:\n%s", err, browserText(ctx))
	}
	var initialView struct {
		PiToggle       string `json:"piToggle"`
		KeyringPresent bool   `json:"keyringPresent"`
		KeyringToggle  bool   `json:"keyringToggle"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => ({
		piToggle: document.querySelector('#view-body [data-adapter-override="pi"]').textContent,
		keyringPresent: !!document.querySelector('#view-body .view-folder[data-adapter-id="keyring"]'),
		keyringToggle: !!document.querySelector('#view-body [data-adapter-override="keyring"]'),
	}))()`, &initialView)); err != nil {
		t.Fatal(err)
	}
	if initialView.PiToggle != "停用" || !initialView.KeyringPresent || initialView.KeyringToggle {
		t.Fatalf("adapter controls = %+v, want pi 停用 and no keyring switch", initialView)
	}

	// Opening the shared confirmation dialog is not enough to send the change.
	if err := chromedp.Run(ctx,
		chromedp.Click(`#view-body [data-adapter-override="pi"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#dlg-confirm[open]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open disable confirmation: %v\npage:\n%s", err, browserText(ctx))
	}
	var confirmation struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Note        string `json:"note"`
		Button      string `json:"button"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({
		title: document.getElementById("confirm-title").textContent,
		description: document.getElementById("confirm-desc").textContent,
		note: document.getElementById("confirm-note").textContent,
		button: document.getElementById("btn-confirm-ok").textContent,
	})`, &confirmation)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(confirmation.Title, "确认停用") || confirmation.Description != "停用后这台机器不再同步该适配器，本地文件保留。" || confirmation.Button != "确认停用" {
		t.Fatalf("disable confirmation = %+v", confirmation)
	}
	var postsBeforeConfirm int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__agentOverridesUIRequests.filter(request => request.method === "POST").length`, &postsBeforeConfirm)); err != nil {
		t.Fatal(err)
	}
	if postsBeforeConfirm != 0 {
		t.Fatalf("adapter override was posted before confirmation: %d requests", postsBeforeConfirm)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`#btn-confirm-cancel`, chromedp.ByQuery),
		chromedp.Poll(`!document.getElementById("dlg-confirm").open && app.pendingAction === null`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
	); err != nil {
		t.Fatalf("cancel disable confirmation: %v\npage:\n%s", err, browserText(ctx))
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__agentOverridesUIRequests.filter(request => request.method === "POST").length`, &postsBeforeConfirm)); err != nil {
		t.Fatal(err)
	}
	if postsBeforeConfirm != 0 {
		t.Fatalf("cancelled adapter override still posted: %d requests", postsBeforeConfirm)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`#view-body [data-adapter-override="pi"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#dlg-confirm[open]`, chromedp.ByQuery),
		chromedp.Click(`#btn-confirm-ok`, chromedp.ByQuery),
		chromedp.Poll(`window.__agentOverridesUIRequests.some(request => request.url === "/api/agents/ui-box/adapters/pi" && request.method === "POST")`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
		chromedp.Poll(`(() => {
			const group = document.querySelector('#view-body .view-folder[data-adapter-id="pi"]');
			return !!group && !!group.querySelector(".view-adapter-disabled-badge") &&
				group.querySelector('[data-adapter-override="pi"]').textContent === "启用";
		})()`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
	); err != nil {
		t.Fatalf("confirm adapter disable and refresh the view: %v\npage:\n%s", err, browserText(ctx))
	}
	var disableCall struct {
		URL    string `json:"url"`
		Method string `json:"method"`
		Body   struct {
			Disabled bool `json:"disabled"`
		} `json:"body"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__agentOverridesUIRequests.find(request => request.method === "POST")`, &disableCall)); err != nil {
		t.Fatal(err)
	}
	if disableCall.URL != "/api/agents/ui-box/adapters/pi" || disableCall.Method != http.MethodPost || !disableCall.Body.Disabled {
		t.Fatalf("disable request = %+v, want POST /api/agents/ui-box/adapters/pi {disabled:true}", disableCall)
	}
	var disabledGroupText string
	if err := chromedp.Run(ctx, chromedp.Text(`#view-body .view-folder[data-adapter-id="pi"]`, &disabledGroupText, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disabledGroupText, "↑1 未收取") || !strings.Contains(disabledGroupText, "已停用") || !strings.Contains(disabledGroupText, "启用") {
		t.Fatalf("disabled adapter group lost its status or controls: %q", disabledGroupText)
	}

	// The disabled adapter remains visible in collect choices, but is grey and cannot be selected.
	if err := chromedp.Run(ctx,
		chromedp.Click(`#dlg-view menu button`, chromedp.ByQuery),
		chromedp.Click(`//div[@id="agent-list"]//button[normalize-space()="收取"]`, chromedp.BySearch),
		chromedp.WaitVisible(`#confirm-choices input[data-adapter="pi"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open collect choices: %v\npage:\n%s", err, browserText(ctx))
	}
	var collectRow struct {
		Disabled bool   `json:"disabled"`
		Checked  bool   `json:"checked"`
		Grey     bool   `json:"grey"`
		Class    string `json:"class"`
		Text     string `json:"text"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const input = document.querySelector('#confirm-choices input[data-adapter="pi"]');
		const label = input && input.closest("label");
		return {
			disabled: !!input && input.disabled,
			checked: !!input && input.checked,
			grey: !!label && Number.parseFloat(getComputedStyle(label).opacity) < 1,
			class: label ? label.className : "",
			text: label ? label.innerText : "",
		};
	})()`, &collectRow)); err != nil {
		t.Fatal(err)
	}
	if !collectRow.Disabled || collectRow.Checked || !collectRow.Grey || !strings.Contains(collectRow.Class, "disabled") || !strings.Contains(collectRow.Text, "已在这台机器停用") {
		t.Fatalf("disabled collect row = %+v, want grey, unchecked, disabled and show the reason", collectRow)
	}

	// Enabling uses the same confirmation path and posts disabled:false.
	if err := chromedp.Run(ctx,
		chromedp.Click(`#btn-confirm-cancel`, chromedp.ByQuery),
		chromedp.Poll(`!document.getElementById("dlg-confirm").open && app.pendingAction === null`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
		chromedp.Click(`//div[@id="agent-list"]//button[normalize-space()="查看"]`, chromedp.BySearch),
		chromedp.WaitVisible(`#view-body [data-adapter-override="pi"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#view-body [data-adapter-override="pi"]').textContent === "启用"`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
		chromedp.Click(`#view-body [data-adapter-override="pi"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#dlg-confirm[open]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open enable confirmation: %v\npage:\n%s", err, browserText(ctx))
	}
	var enableDescription string
	if err := chromedp.Run(ctx, chromedp.Text(`#confirm-desc`, &enableDescription, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if enableDescription != "启用则立即恢复该适配器同步。" {
		t.Fatalf("enable confirmation description = %q", enableDescription)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`#btn-confirm-ok`, chromedp.ByQuery),
		chromedp.Poll(`window.__agentOverridesUIRequests.some(request => request.url === "/api/agents/ui-box/adapters/pi" && request.method === "POST" && request.body.disabled === false)`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
		chromedp.Poll(`(() => {
			const group = document.querySelector('#view-body .view-folder[data-adapter-id="pi"]');
			return !!group && !group.querySelector(".view-adapter-disabled-badge") &&
				group.querySelector('[data-adapter-override="pi"]').textContent === "停用";
		})()`, nil, chromedp.WithPollingInterval(100*time.Millisecond)),
	); err != nil {
		t.Fatalf("confirm adapter enable and refresh the view: %v\npage:\n%s", err, browserText(ctx))
	}
}

type agentOverridesUIAgentSource struct{}

func (agentOverridesUIAgentSource) ListAgents() []web.AgentInfo {
	return []web.AgentInfo{{AgentID: "ui-box", Hostname: "UI 测试机器", LastSeen: time.Now()}}
}

func (agentOverridesUIAgentSource) AgentStatus(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"adapters":[{"id":"pi","push":1,"categories":[{"name":"files","label":"配置","files":[{"path":"settings.json","size":12}]}]},{"id":"keyring","categories":[]}],"errors":[]}`), nil
}

func (agentOverridesUIAgentSource) AgentDiff(context.Context, string, web.DiffParams) (string, error) {
	return "", nil
}

func (agentOverridesUIAgentSource) AgentPush(context.Context, string, bool, web.SyncScope) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true,"status":"pushed"}`), nil
}

func (agentOverridesUIAgentSource) AgentPull(context.Context, string, bool, web.SyncScope) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true,"status":"applied"}`), nil
}

func (agentOverridesUIAgentSource) RemoveAgent(string) bool { return true }
