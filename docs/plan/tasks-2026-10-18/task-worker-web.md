# 任务卡：Worker-Web — Hub 自更新/自重启的 Web 路由层

项目 `/root/code/homer-cli`（Go 1.23.4）。基线 = master HEAD（已含 commit `382ebdf`）。

## 必读（先读完再动手）

1. `docs/plan/2026-10-18-hub-self-update-contract.md` —— **冻结契约，权威**。重点 §1（D1–D8）与 §2 的 D11/§3 的 D19。
2. `internal/web/hubops.go` —— 已定义好的共享契约面：`HubOperations` 接口、`HubStatus`、`HubUpdateAccepted`、`HubRestartAccepted`、`HubError`、`hubCode*` 常量、`hubPhase*` 常量。

**不要修改这两个文件。** 它们是三个并行 worker 的共享面，由主 agent 独占。

## 文件所有权（只能动这些，越界即冲突）

| 动作 | 路径 |
|---|---|
| 改动 | `internal/web/handlers.go` —— 三个 case + `handleHealth` 扩字段 + draining 判定 + `writeHubError` 辅助函数 |
| 改动 | `internal/web/server.go` —— **仅**在必要时补最小机制（见下），不要重构 |
| 新建 | `internal/web/hubops_routes_test.go` |

**禁止碰**：`internal/web/static/index.html`、`internal/web/copy_test.go`、`internal/web/ui_issues_test.go`（Front worker 拥有）、`internal/cli/**`（CLI worker 拥有）、`internal/web/hubops.go`。

## 必须实现的行为

### 1. 三端点（D1）
在 `internal/web/handlers.go` 的 `serveHTTP` switch 中新增（**必须位于 `/api/*` requireAuth 门禁之后**的 switch 区域内，不要加到 auth 白名单）：

| 方法 | 路径 | 行为 | 成功码 |
|---|---|---|---|
| GET | `/api/hub` | `writeJSON(w, 200, s.opts.HubOps.HubStatus(r.Context()))` | 200 |
| POST | `/api/hub/upgrade` | `s.opts.HubOps.StartHubUpdate(r.Context())` → `writeJSON(w, 202, accepted)` | 202 |
| POST | `/api/hub/restart` | `s.opts.HubOps.RestartHub(r.Context())` → `writeJSON(w, 202, accepted)` | 202 |

错误方法 → `writeMethodNotAllowed(w)`（既有实现已返回 `Allow: "GET, POST"`，无需改）。

### 2. 错误映射（D2、D8）
- `s.opts.HubOps == nil` → 三端点全部 `writeError(w, 501, "hub-disabled", "这台 hub 不能自己更新", nil)`。**不许**用 404 或 503。
- `*web.HubError`（`errors.As`）→ 按其 `Status`/`Code`/`Message`/`Details` 输出（用 `writeError`）。
- 其他 error → 走既有 `writeErrorValue`（不得新造错误信封）。
- 新增辅助函数 `writeHubError(w http.ResponseWriter, err error) bool`，放 `handlers.go`（**不要放 hubops.go，那是共享面**）。

### 3. `/api/health` 扩字段（D19）
现有返回 `{ok,status,version,homerHome}`，新增 `bootId` 与 `startedAt`：
- `HubOps != nil` → 用 `HubStatus.BootID`。
- `HubOps == nil` → 用本进程 fallback bootId（`crypto/rand` 8 hex，在 `NewServer` 里生成一次，存到 `Server` 结构体）。
- `startedAt` = `NewServer` 时的 `time.Now()`（同样存到 `Server`）。
- **health 仍免鉴权、仍只读，不得因 `HubOps == nil` 而 500。**

### 4. draining → 503 `restarting`（D11）
当 `HubStatus.Phase == "restarting"` 时，写操作请求应尽快返回 `503 restarting` +「中心正在重启，请稍后重试」（**码必须区别于既有的 `write-busy`**）。

实现位置：`serveHTTP` 中 `/api/*` 鉴权门禁之后、switch 之前。规则：
- 仅 `r.Method != http.MethodGet` 时生效；GET 不受影响。
- **必须排除** `/api/hub/restart` 与 `/api/hub/upgrade` 自身（避免自锁）。
- **必须排除** `/api/auth/*`（登出/登录不能被挡）。
- `HubOps == nil` 时跳过（无 phase 可读）。
- 只在需要时调一次 `HubStatus`（它有 mutex，不要每请求调两次）。

### 5. 路由不得泄露给机器凭证
`/api/hub*` **不得**加入 `auth.go` 的 `principalAllowedForEndpoint` 白名单。默认拒绝即可，无需改 auth.go。需写测试验证：agent secret 访问 `/api/hub` 得到 401。

## 测试（`internal/web/hubops_routes_test.go`）

用 stub `HubOperations` 实现。范式参考 `internal/web/gate_matrix_test.go`（凭证矩阵）与 `internal/web/handlers_version_test.go`（health/版本断言）。

必须覆盖：
1. `HubOps == nil` → 三端点全 501 `hub-disabled`
2. `GET /api/hub` 200 + JSON 字段齐全（`version`/`bootId`/`phase`/`canUpdate`/`downloaded`/`total`）
3. `POST /api/hub/restart` 202 + `restartId`/`bootId`/`busy`
4. `POST /api/hub/upgrade` 202
5. `*HubError` 映射：409 `hub-update-running`、409 `hub-dev-version`、502 `hub-update-failed`
6. 错误方法 405
7. 未鉴权 401
8. agent secret 访问 `/api/hub` → 401
9. `/api/health` 含 `bootId` 与 `startedAt`，且 `HubOps == nil` 时仍 200
10. `phase == "restarting"` 时：POST 其他写端点 → 503 `restarting`；GET 不受影响；POST `/api/hub/restart` 不自锁；`/api/auth/logout` 不被挡

## 验收（必须全绿，自己跑并把输出贴进 verdict）

```sh
cd /root/code/homer-cli
test -z "$(gofmt -l . | grep -v vendor)"
go build ./...
go vet ./internal/web/...
go test ./internal/web/... -count=1
```

`internal/web` 包测试约 28s。已知基线红与本任务无关：`internal/gitx TestEnsureLocalIdentityWritesOnlyThisRepo`。
若 `go test ./internal/web/...` 出现**不是我引入的**既有红，要精确说明是哪个测试、贴失败输出，不要含糊带过。

## 交付

自报 verdict：改了哪些文件、**逐条 D 编号**说明如何落实、测试结果、契约矛盾点（**有就明说，不要自行解释绕过**）。
