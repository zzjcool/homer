# Worker-Web 交付报告：Hub 自更新/自重启的 Web 路由层

日期：2026-10-18
分支：`pi-subagent/worker-1-d9230fd8`
commit：`74a6f94`
PR：https://github.com/zzjcool/homer/pull/136

---

## 1. 改动文件清单

| 动作 | 路径 | 说明 |
|---|---|---|
| 改 | `internal/web/handlers.go` | 三个 case + `handleHealth` 扩字段 + draining 判定 + `writeHubError` / `writeHubOperationError` / `writeHubDisabled` 辅助函数 |
| 改 | `internal/web/server.go` | 仅补最小机制：`Server` 加 `bootID` / `startedAt` 两个字段、`newBootID()`、`bootIdentity()`、`NewServer` 里生成一次 |
| 新建 | `internal/web/hubops_routes_test.go` | 10 项必测 + 负向对照，共 10 个 Test 函数 |

**未触碰**（守住了任务卡的所有权边界）：
`internal/web/static/index.html`、`internal/web/copy_test.go`、`internal/web/ui_issues_test.go`（Front worker）、
`internal/cli/**`（CLI worker）、`internal/web/hubops.go`（共享面）。
`git diff --stat` 只有 3 个文件：handlers.go +164/-3、server.go +41/-3、hubops_routes_test.go 新建。**auth.go 未改**。

---

## 2. 逐条 D 编号落实

### D1 三端点、一个资源命名空间

三个 case 加在 `serveHTTP` 的 switch 内（`path == "/api/console"` 之后、`path == "/api/status"` 之前），
位于 `strings.HasPrefix(path, "/api/")` → `requireAuth` 门禁**之后**，**未**加入 auth 白名单。

| 方法 | 路径 | 实现 | 成功码 |
|---|---|---|---|
| GET | `/api/hub` | `s.handleHubStatus` → `writeJSON(w, 200, HubOps.HubStatus(ctx))` | 200 |
| POST | `/api/hub/upgrade` | `s.handleHubUpdate` → `StartHubUpdate` → `writeJSON(w, 202, accepted)` | 202 |
| POST | `/api/hub/restart` | `s.handleHubRestart` → `RestartHub` → `writeJSON(w, 202, accepted)` | 202 |

错误方法 → 既有 `writeMethodNotAllowed(w)`（已返回 `Allow: "GET, POST"`，未改）。未加 `mode` 参数、未加 `/api/hub/actions`。

### D2 nil → 501 hub-disabled（不是 404/503）

`writeHubDisabled(w)` = `writeError(w, 501, hubCodeDisabled, "这台 hub 不能自己更新", nil)`。
三个 handler 各自首行判 `s.opts.HubOps == nil`。放在 handler 内而不是 `serveHTTP` 里，是因为
三端点语义不同（GET 是状态、两个 POST 是动作），且 501 判定必须发生在方法校验之后才有 405 语义。

### D3 接口签名

只**消费** `hubops.go` 里已冻结的 `HubOperations` / `HubStatus` / `HubUpdateAccepted` /
`HubRestartAccepted` / `HubError` / `hubCode*` / `hubPhase*`，零改动。handler 直接 `writeJSON` 这些
结构体，JSON 字段名由结构体 tag 决定，web 层不复制字段名。

### D4 阶段

`rejectWhileRestarting` 只判等 `hubPhaseRestarting`；其余阶段（idle/checking/downloading/installing/failed）
一律放行。web 层不解析 `Note`、不解释 `RolledBack`，原样透传。

### D5 升级链

web 层不参与：`StartHubUpdate` 返回即 202，链在 cli 的后台任务里跑。符合「终点是进程自杀，响应必然被 reset」的取舍。

### D6/D7 版本通道与 dev 拒绝

属于 cli 层的 `HubStatus.CanUpdate` / `*HubError{409, hubCodeDevVersion}` 决策。web 层只做映射：
`GET /api/hub` 原样返回 `CanUpdate`；`POST /api/hub/upgrade` 收到 `hub-dev-version` 的 `*HubError` → 409，
Message 用 cli 传的冻结文案（见下「错误映射」）。**未提供 force**，web 层没有可加 force 的位置。

### D8 错误码与错误映射

```go
func writeHubOperationError(w http.ResponseWriter, err error) {
	if writeHubError(w, err) { return }
	writeErrorValue(w, err)   // 既有映射：500 internal / 502（AgentError 携带时）
}
```

`writeHubError` 用 `errors.As(err, &hubErr)` 认 `*HubError`，按其 `Status`/`Code`/`Message`/`Details`
经 `writeError` 输出并返回 `true`；非 `*HubError`（含 nil）返回 `false`，交给既有 `writeErrorValue`。
**未新造错误信封**——两者最终都落到 `writeError` → `errorEnvelope`。

两处防御性默认（不改变契约语义，只保证畸形构造不会输出非法 HTTP）：
- `Status == 0` → 500（而非 `WriteHeader(0)` panic）
- `Code == ""` → `hubCodeFailed`；`Message == ""` → `errorMessage(code)` 兜底

`Details` 直传；`writeError` 内部把 nil 归一为 `[]`，所以 `hub-update-running` 的 `details` 是 `[]` 而非 `null`。

D8 最后一条：`/api/hub*` **未**加入 `principalAllowedForEndpoint`，默认拒绝。测试
`TestHubRoutesRequireAdmin` 用 `agent-secret-live` 实测三端点全 401，且断言 `HubOps` 零调用。

### D11 draining → 503 restarting

```
serveHTTP → requireAuth → handleAgentEndpoint 分派 → rejectWhileRestarting → switch
```

位置严格按要求：`/api/*` 鉴权门禁之后、switch 之前。

规则实现（`rejectWhileRestarting`）：
1. `s.opts.HubOps == nil` → 跳过（无 phase 可读，健康检查不受影响）
2. `r.Method == http.MethodGet` → 跳过（GET 不受影响）
3. 非 `/api/` 前缀 → 跳过
4. 排除 `/api/hub/restart`、`/api/hub/upgrade`（避免自锁）、`/api/auth/*`（登出/登录不能被挡）
5. **一次** `HubStatus(ctx)` 读取，判 `Phase != hubPhaseRestarting` → 放行
6. 命中 → `writeError(w, 503, hubCodeRestarting, "中心正在重启，请稍后重试", nil)`

`hubCodeRestarting = "restarting"`，与既有 `write-busy` 是**不同码**。
每请求至多一次 `HubStatus` 读取——测试逐请求断言 `statusCalls`，`/api/hub` 的 GET 断言恰好 1 次（gate 未加读）。

`/api/auth/*` 的排除在实现上是双保险：这些请求在门禁之前就被 `handleAuthAPI` 分流了，门禁里的前缀判断
保证「即使将来顺序变动也不会挡登出」。

### D19 `/api/health` 扩字段

```json
{"ok":true,"status":"healthy","version":"1.5.0","homerHome":"...",
 "bootId":"9f3c1a77","startedAt":"2026-10-18T07:12:33Z"}
```

- `bootId`：`crypto/rand` 4 字节 → `hex` → 8 字符。`NewServer` 里生成一次。
- `startedAt`：`NewServer` 里的 `time.Now()`，RFC3339（UTC）输出。
- `HubOps != nil` → 用 `HubStatus.BootID`（cli 是唯一知道 re-exec 发生过的层）；`BootID == ""` 时回落本进程值，不输出空字段。
- `HubOps == nil` → 本进程 fallback，**仍 200**，不 500。
- health 仍免鉴权（`/api/health` 分支在 `requireAuth` 之前），仍只读。

`Server` 字面量构造（`credentials_test.go`、`storage_file_test.go` 这么干）时字段为空，
`bootIdentity()` 回落到包级 `processBootID` / `processStartedAt`，所以任何路径都能拿到合法值。

### D20 前端文案

本 worker 不产前端文案，`copy_test.go` 未动。

---

## 3. 测试覆盖（`internal/web/hubops_routes_test.go`）

stub `hubOpsStub` 实现 `HubOperations`，字段可控、计数可断言（`statusCalls`/`updateCalls`/`restartCalls`）。

| # | 任务卡必测项 | 测试函数 |
|---|---|---|
| 1 | `HubOps == nil` → 三端点全 501 `hub-disabled` | `TestHubRoutesDisabledWithoutHubOps`（含文案断言） |
| 2 | `GET /api/hub` 200 + 字段齐全 | `TestHubStatusRouteFields`（version/bootId/phase/canUpdate/latest/downloaded/total/note，并断言 `HubStatus` 只读 1 次） |
| 3 | `POST /api/hub/restart` 202 + restartId/bootId/busy | `TestHubRestartRouteAccepted` |
| 4 | `POST /api/hub/upgrade` 202 | `TestHubUpdateRouteAccepted` |
| 5 | `*HubError` 映射 409/409/502 | `TestHubErrorMapping`（+ `hub-restart-unsupported` 501、以及普通 error 走 `writeErrorValue` 的 500 分支，逐条断言 code/message/details） |
| 6 | 错误方法 405 | `TestHubRoutesMethodNotAllowed`（5 组方法×路径，断言不触达 `StartHubUpdate`/`RestartHub`，且 status 读取 ≤1） |
| 7 | 未鉴权 401 | `TestHubRoutesRequireAdmin`（三端点各验一次） |
| 8 | agent secret 访问 `/api/hub` → 401 | 同上（`AgentEndpointAuthorized` 只认 `agent-secret-live`，三端点全 401） |
| 9 | `/api/health` 含 `bootId`/`startedAt`，`HubOps == nil` 仍 200 | `TestHealthCarriesBootIdentity`（子测：无 HubOps 时 8 hex + RFC3339 可解析；有 HubOps 时以 `HubStatus.BootID` 为准） |
| 10 | `restarting` 时 POST 503 / GET 不受影响 / restart 不自锁 / logout 不被挡 | `TestRestartingPhaseRejectsWrites`（4 个子测） |

额外负向对照（防止「恒 503」这类假绿灯）：
- `TestIdlePhaseKeepsWritesOpen` —— 非 restarting 阶段写请求不被 draining 门禁拦截
- `TestHubStatusJSONOmitsEmptyOptionalFields` —— idle 时 `latest`/`note`/`rolledBack` 不出现在 wire 上（`omitempty` 语义冻结）

`bootId` 用 `^[0-9a-f]{8}$` 正则断言，`startedAt` 用 `time.Parse(RFC3339)` + `!IsZero()` 断言，不是「非空字符串」糊过去。

---

## 4. 验证输出（原样）

```sh
$ cd /root/code/homer-cli/.pi-subagents/runs/r-bad8f37c/worktrees/worker-1
$ test -z "$(gofmt -l . | grep -v vendor)"
GOFMT_OK
$ go build ./...
BUILD_OK
$ go vet ./internal/web/...
VET_OK
$ go test ./internal/web/... -count=1
ok  	github.com/zzjcool/homer/internal/web	23.205s
```

任务卡要求的四条验收命令**全绿**。附加（非必需，自查用）：

```sh
$ go vet ./...
VET_ALL_OK
```

**无既有红**：`internal/web` 23.2s 全绿；`internal/gitx TestEnsureLocalIdentityWritesOnlyThisRepo` 基线红不在本次 verify
范围（`./internal/web/...`）内，未触及。本轮跑了几次 `go test ./internal/web/...`，均为 `ok`。

---

## 5. 契约矛盾点

**无阻塞性矛盾。** 三点需要主 agent 知情（都不是需要修订契约的问题，是我的实现选择，均有契约依据）：

1. **`Task`「`/api/health` 不得因 `HubOps == nil` 而 500」与 hubops.go 注释的措辞差异** —
   契约 D19 只说 health 新增两字段；hubops.go 说 `HubOps == nil` 时「every /api/hub* route answers 501」。
   `/api/health` 不是 `/api/hub*`，所以不冲突。实现上 health 在 `HubOps == nil` 时用本进程 fallback，**永不 501/500**。
   已按任务卡 §3 明确要求处理。

2. **D11 的排除清单只列了 `/api/hub/restart` 与 `/api/hub/upgrade`、未提 `/api/hub` 的 POST** —
   任务卡说「必须排除 `/api/hub/restart` 与 `/api/hub/upgrade` 自身」，我**严格只排除这两个**。
   因此 `POST /api/hub`（错误方法）在 draining 期间会先被 draining 门禁判 503，再轮到 405。
   我认为这符合「排除清单是穷举」的读法（一个错误方法不必享受排除），但如果主 agent 期望 draining 期间
   方法校验优先于 draining 门禁，请裁决——目前 `TestHubRoutesMethodNotAllowed` 断言的是**非 draining** 状态下 405，
   不与该读法冲突。

3. **D19「`bootId` 每进程随机 8 hex」与 cli 层的 bootId 来源** —
   契约 D19 把 bootId 描述为 web 层新增字段，但 cli 的 `HubStatus` 也有 `BootID`，
   且只有 cli 知道 re-exec 是否发生过（以及是否 rollback）。我让 web 层**优先用 `HubStatus.BootID`**、
   仅在 `HubOps == nil` 或 `BootID == ""` 时用 `NewServer` 生成的本进程值。若 cli 实现的 `BootID` 恒定
   非空，则 `NewServer` 的 fallback 值在 HubOps 存在时永远不被使用（这是刻意的：重启判据必须反映
   re-exec，而不是 `Server` 结构体何时构造）。CLI worker 只要保证重新 exec 后 `BootID` 变化即可，
   两边可以独立并行、无需协调。

另：任务卡 §3 写「`HubOps == nil` → 用本进程 fallback bootId（`crypto/rand` 8 hex，在 `NewServer` 里生成一次，
存到 `Server` 结构体）」——已照做。`Server` 加 2 个字段属任务卡明确授权的「仅必要时补最小机制」，未重构。
