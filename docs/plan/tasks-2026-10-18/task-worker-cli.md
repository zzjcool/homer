# 任务卡：Worker-CLI — Hub 自更新 + 自重启的 CLI 侧实现

项目 `/root/code/homer-cli`（Go 1.23.4）。基线 = master HEAD（已含 commit `382ebdf`）。

## 必读（先读完再动手）

1. `docs/plan/2026-10-18-hub-self-update-contract.md` —— **冻结契约，权威**。逐条对照 D1–D14。
2. `internal/web/hubops.go` —— 已定义好的共享契约面：`HubOperations` 接口、`HubStatus`、`HubUpdateAccepted`、`HubRestartAccepted`、`HubError`、`hubCode*` 与 `hubPhase*` 常量。

**不要修改这两个文件。** 它们是三个并行 worker 的共享面，由主 agent 独占。

## 文件所有权（只能动这些，越界即冲突）

| 动作 | 路径 |
|---|---|
| 新建 | `internal/cli/hubops.go`（HubOperations 实现、状态机、回滚） |
| 新建 | `internal/cli/hubreexec_unix.go`（build tag `linux \|\| darwin`） |
| 新建 | `internal/cli/hubreexec_other.go`（build tag `!linux && !darwin`） |
| 新建 | `internal/cli/hubops_test.go` |
| 改动 | `internal/cli/hub.go` —— **仅**在 `runServe` 里装配 `web.ServeOptions.HubOps` 并暴露重启所需的 shutdown 能力 |

**禁止碰**：`internal/web/static/index.html`、`internal/web/handlers.go`、`internal/web/copy_test.go`、`internal/web/ui_issues_test.go`、`internal/web/hubops.go`、`internal/web/server.go`。

## 必须实现的行为（逐条对照契约）

### 1. 重启机制（D9–D10、D14）
- 主路 `syscall.Exec(exe, normalizedArgs, os.Environ())`。
- `--addr` 规范化为**已绑定地址**（缺失则追加）；其余参数与 environ 原样透传。
- 重绑端口退避重试：100ms × 20 次。
- `!linux && !darwin` → 返回错误（web 层回 501 `hub-restart-unsupported`），**绝不 exit**。

### 2. Exec 前顺序（D11，不可调）
```
置 draining → agentHub.Shutdown(≤3s)（发 close 1001 "hub restarting"）
→ httpServer.Shutdown(≤2s，超时即弃，不等待 12min pull）
→ 2s 探针试拿 writeMutex（拿不到也继续，Busy=true 上报）
→ syscall.Exec
```
- **禁止持 `writeMutex` 时 Exec**。
- **不拒绝重启**：拿不到锁也照常重启，只把 `Busy` 标出来。

### 3. draining 状态如何让 web 看见（重要实现决策）
`internal/web/server.go` 是共享面，你不能改。可行做法：把 `phase == "restarting"` 作为 web 层判定 draining 的唯一依据（Web worker 会读 `HubOps.HubStatus(ctx).Phase`）。
所以你在 Exec 之前必须先把 phase 置为 `restarting` 并短暂留出时间窗（建议 ≥200ms 的宽限，让在途写请求能拿到 503）。在代码注释里写明这条契约。

### 4. Exec 失败回退链（D12）
| 情形 | 行为 |
|---|---|
| Exec 返错 且 `INVOCATION_ID` 为空 | **保持服务不退出**，`log.Printf` 报 errno + 目标路径，draining 复位，返回 `*web.HubError{502, hub-update-failed}` |
| Exec 返错 且 `INVOCATION_ID` 非空 | 优雅停机 + `exit(1)`，交 supervisor 拉起 |
| Exec 成功 | 永不返回 |
**永不调用 systemctl 重启自身。**

### 5. 防崩溃循环（D13）
- 替换前先把现二进制 rename 成 `<self>.prev`；失败即放弃本次替换。
- 标记文件 `<home>/keys/.hub-boot`，内容 `{sha256, startedAt, attempt}`。启动时立即写；ready（`net.Listen` 成功）即删。
- 回滚**仅当**：标记存在 && 距 `startedAt` < 90s && 标记 sha256 == 当前二进制 sha256 && `attempt == 2`。
- 回滚 = `.prev` rename 覆盖 + 一次 Exec 进回滚版本 + `HubStatus.RolledBack = true`。
- 回滚后**禁止**再触发第二次。不做多代历史、不做版本 pin。

### 6. 升级链（D5，顺序不可调）
```
FetchLatest → IsNewer → InstallArchive(temp+rename) → RestartRunningAgents(≤10s, best-effort) → phase=restarting → Exec
```
- `upgrade.NewClient(os.Getenv("HOMER_INSTALL_BASE_URL"))` + `FetchLatest()`
- `upgrade.IsNewer(latest.TagName, web.Version)`；不是新版本 → `idle` + `note:"已是最新版本 vX"`
- `upgrade.NewDownloader(同 baseURL).InstallArchive(latest.TagName, self)` —— **含 checksums.txt SHA256 校验**
- `RestartRunningAgents` ≤10s best-effort；非 Linux 返回的「需要手动重启」是预期降级，**只记 note，不得让 hub 重启失败**。
- **绝不原地写二进制**（否则同机 agent 吃 ETXTBSY/SIGBUS）。
- 纯重启路径**不触碰 agent 进程**。

### 7. 状态机（D4）
6 阶段：`idle|checking|downloading|installing|restarting|failed`。**只存内存**，`sync.Mutex` 保护。
- 并发再 POST upgrade → `*web.HubError{409, hub-update-running}`
- `downloading|installing` 期间 restart → 同样 409
- `failed` 的 `note` 必须区分两条分支：① 下载/校验失败（新二进制**未**写入，可重试）② 替换成功但 Exec 失败（note：「新版本已装好，但中心没有重启；请在服务器上重新执行 homer serve」）

### 8. dev 拒绝（D7）
`web.Version == "dev"` 或 `!upgrade.IsParseableVersion(web.Version)` → `CanUpdate:false` + note；POST upgrade → `*web.HubError{409, hub-dev-version}`。`HOMER_NO_VERSION_CHECK=1` → 同样 `CanUpdate:false`。**不提供 force。**

### 9. BootID
每进程随机 8 hex（`crypto/rand`），进程启动时生成一次，`HubStatus.BootID` 返回它。

### 10. HubStatus 不发网络请求
只读内存。`latest` 由后台 10 分钟 TTL 刷新（首次调用异步刷一次即可，不要为此建复杂调度器）。

## 实现要点

- 可复用范式：`internal/agentd/reexec_unix.go:10-19`、`internal/agentd/handlers.go:354-400`（in-flight 等待 + 去重）、`internal/upgrade/download.go:83 InstallArchive`、`internal/upgrade/upgrade.go:54 FetchLatest` / `:100 IsNewer`、`internal/upgrade/tar.go:40 SelfPath`、`internal/cli/commands/agentrestart.go`（`RestartRunningAgents`）。
- `internal/cli` 与 `internal/cli/commands` 是**两个包**，注意 import cycle（`commands` 不 import `cli`）。
- 日志用标准库 `log`（项目无 slog）；用户可见文案全中文。
- **可测试性**：把 `exec`、`sha256`、时钟、`INVOCATION_ID` 读取都做成可注入的字段/函数变量，单测才能覆盖回滚与失败分支（参考 `internal/cli/commands/agentrestart.go:15-22` 的 `restarter` 结构体注入范式）。

## 单测必须覆盖

状态机转换、并发 409、dev 拒绝、bootID 稳定性、回滚判定四条件（尤其 sha256 相同但 attempt==1 时不回滚）、Exec 失败两条分支。

## 验收（必须全绿，自己跑并把输出贴进 verdict）

```sh
cd /root/code/homer-cli
test -z "$(gofmt -l . | grep -v vendor)"
go build ./...
go vet ./internal/cli/...
go test ./internal/cli/... -count=1
```

已知基线红（与本任务无关，**不要去修**）：`internal/gitx TestEnsureLocalIdentityWritesOnlyThisRepo`（宿主 git 自造身份 `Super User <root@host.host.zzj.cool>`）。

## 交付

自报 verdict：改了哪些文件、**逐条 D 编号**说明如何落实、测试结果、以及任何你认为契约有矛盾的地方（**矛盾就明确说，不要自行解释绕过**）。
