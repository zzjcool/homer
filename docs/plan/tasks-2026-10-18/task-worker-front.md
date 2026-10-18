# 任务卡：Worker-Front — 控制台 Hub 入口与一键重启 UI

项目 `/root/code/homer-cli`（Go + 单文件内嵌控制台）。基线 = master HEAD（已含 commit `382ebdf`）。

## 必读（先读完再动手）

1. `docs/plan/2026-10-18-hub-self-update-contract.md` —— **冻结契约，权威**。重点 §3（D15–D20）。
2. `internal/web/hubops.go` —— 后端 JSON 字段名以它为准：`HubStatus{version,bootId,phase,canUpdate,latest,downloaded,total,note,rolledBack}`、`HubUpdateAccepted{phase,bootId}`、`HubRestartAccepted{restartId,bootId,busy}`。

**不要修改这两个文件。**

## 文件所有权（只能动这些，越界即冲突）

| 动作 | 路径 |
|---|---|
| 改动 | `internal/web/static/index.html` |
| 改动 | `internal/web/copy_test.go`（同步文案冻结清单） |
| 改动（仅在必要时，且不得破坏现有断言） | `internal/web/ui_issues_test.go` |
| 新建 | `tests/e2e/hub_ops_ui_test.go`（chromedp + stub，模仿 `tests/e2e/tool_upgrade_ui_test.go:198` 范式） |

**禁止碰**：`internal/web/handlers.go`、`internal/web/server.go`、`internal/web/hubops.go`、`internal/cli/**`。

## 破坏 CI 的四个坑（先读这些，再动 HTML）

1. **文案冻结**：`internal/web/copy_test.go:13` 的 banned 列表含「推送」「拉取」「仓库」「commit」「bare」「处理改动」「同步到其他机器」「以本机为准」「这台机器的改动」等。**新文案绝对不能含这些词**。required 列表已含「更新程序」「全部更新」「a.outdated」——**不要删除**。
2. **切片脆弱**：`internal/web/ui_issues_test.go:44` 用字符串切片 `$("btn-logout").addEventListener("click", async () => {` → `\n$("btn-refresh")` 切 logout 函数体做顺序断言。**新的 addEventListener 绝对不能插在这两者之间**。全部追加在 `$("btn-plugins").addEventListener(...)` 之后。
3. **header 显隐两处硬编码**：`showGate()`（约 :1083）与 `showWorkspace()`（约 :1093）各自逐个 `add("hidden")`/`remove("hidden")`。新按钮**必须两处都加**，否则会残留在登录页。
4. **语法门**：inline `<script>` 会被 `node --check` 检查（`copy_test.go:102`）。语法错 CI 立刻红。

## 必须实现的行为

### 1. 入口（D15）
- header 中，在 `<button class="text" id="btn-plugins">` **之后**新增 `<button class="text" id="btn-hub" type="button">Hub</button>`。
- 点击打开新建的 `<dialog id="dlg-hub">`；dialog 放在 `</dialog>` 群末尾、`<script>` 之前。
- **不要**做 footer 入口，**不要**塞进插件 dialog。

### 2. dialog 内容
显示：当前版本、最新版本（未知显示「未知」）、phase 进度（downloading 时显示 `downloaded/total` 字节或百分比）、`note` 文案；两个主按钮「更新并重启」「仅重启」；失败时给提示；`rolledBack:true` 时显示「已回滚到上一个版本」。

- 用现成的 `el(tag, cls, text)` / `clear(n)` 构造 DOM，**不要用 innerHTML 拼 HTML**（项目约定）。
- 可复用 `toast()`、`setBusy()/clearBusy()`、`showAlert(title, lines, hint, actions)`。

### 3. 交互范式（D18）
照抄 `upgradeAgents()`（约 :2095-2117）的结构：
```
window.confirm → setBusy → POST → clearBusy → 轮询 health（不是 await refresh()）→ toast / showAlert
```

确认框文案（**冻结字符串，逐字使用**）：
- 更新（已知 latest）：「把中心从 v1.4.0 更新到 v1.5.0？下载完成后中心会重启，控制台大约 10 秒不可用。」（版本号动态填入）
- 更新（latest 未知）：「更新到最新版本？会先检查再下载；完成后中心会重启，控制台大约 10 秒不可用。」
- 仅重启：「重新启动中心？控制台大约 10 秒不可用，正在进行的同步会被中断。」
- **不要求管理员密码二次确认**（与 `upgradeAgent`/吊销/移除的既有范式一致）。
- 必须警告「控制台不可用」，**不**写「机器离线」（agent 会自动重连，用户会误以为机器掉了）。

### 4. 重启确认（D19）
- 轮询 `GET /api/health`：每 500ms，上限 60s，**每次单独 try/catch**（服务停机时 `api()` 会 reject）。
- 判据严格为 `res.ok && data.bootId && data.bootId !== oldBootId`。**不看 version**（纯重启后 version 不变）。
- 成功文案：「中心已重启（vX）」或「中心已重启，版本没有变化」。
- 超时文案（冻结）：标题「中心没有在预期时间内回来」；明细「已经等待 60 秒，健康检查仍然失败。」；提示「请在服务器上确认 homer 进程还在、查看它的日志；必要时重新执行 `homer serve`。」；动作「重新加载页面」→ `location.reload()`。
- **不许**写 `systemctl --user status homer-serve`（该 unit 名不在仓库内，写死会误导——这是 advisor 之间相互推翻后确定的结论）。

### 5. 抑制主轮询（D19）
`app.restarting = true` 期间让 `refresh()` 直接 return。
⚠ 不要破坏 `ui_issues_test.go` 对 refresh 的断言：`if (app.refreshing) {`、`app.refreshing = true;`、`finally`、`btn.disabled = true`、`setAttribute("aria-busy", "true")` 必须都还在。

### 6. 成功确认后
**必须**重新调 `loadHeaderVersion()` 再 `await refresh()`（现在 `loadHeaderVersion` 只在 `boot()` 调一次，约 :5328），否则 header 版本永远显示旧值。

### 7. 更新进度轮询
更新进行中用 `GET /api/hub` 每 1s 刷进度，直到 phase 变为 `restarting` / `failed` / `idle`。

## 文案冻结同步

在 `internal/web/copy_test.go` 的 required 列表加入你新增的冻结串（至少包含：「Hub」「更新并重启」「仅重启」「中心已重启」「中心没有在预期时间内回来」「正在重启中心」），并确保它们**真的出现在 index.html 里**。所有新文案自查不得含 banned 词。

## 验收（必须全绿，自己跑并把输出贴进 verdict）

```sh
cd /root/code/homer-cli
test -z "$(gofmt -l . | grep -v vendor)"
go build ./...
go test ./internal/web/ -run 'TestConsoleUserCopy|TestIndexInlineScriptParses|TestCopyJoinFallsBackWhenClipboardIsUnavailable|TestLogoutFailureStaysInWorkspace|TestJoinCodePollingShowsRepeatedFailures|TestManualRefreshIsBusyGuardedWithoutRemovingAutoRefresh' -count=1
go test ./internal/web/ -count=1
```

chromedp e2e 若本机无 chromium 会自动 skip，可接受（但测试文件本身必须能编译，`go build ./...` 会覆盖）。
若 `go test ./internal/web/` 有**既有基线红**（不是本任务引入），要精确说明哪个测试、贴失败输出。

## 交付

自报 verdict：改了哪些、**逐条 D15–D20** 如何落实、测试结果、契约矛盾点（**有就明说，不要自行解释绕过**）。
