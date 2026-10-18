# 冻结契约：Hub 自更新 + 控制台一键重启

> 2026-10-18。基于 4 路并行侦察 + 2 个 advisor 独立裁决（含相互修订）。
> 本文件是**冻结契约**：派工、实现、审查都以此为准。偏离即视为契约矛盾，需先修订本文件。

## 0. 背景与现状结论

| 问题 | 结论 | 证据 |
|---|---|---|
| 更新走 GitHub Release 吗？ | **是，但不是唯一通道。** 双通道，Hub 优先 | 见 §0.1 |
| 我们自己的 Hub 更新做了吗？ | **做了，但只做了 agent 侧。** Hub 自身没有任何自更新/自重启能力 | 见 §0.2 |
| 控台点一下 Hub 就重启？ | **不存在。** 无端点、无按钮、无 re-exec 原语 | 见 §0.2 |

### 0.1 现有升级通道（`homer upgrade`）

```
homer upgrade
├── 有机器凭证（agent.json 的 agentSecret 或 keys/hub-token）
│   → HUB 通道（主路）：GET {hub}/dl/homer?goos=&goarch=  Bearer
│      校验：X-Homer-Platform 头 + payload magic + CPU arch + ≥1MiB
│      ⚠ 无 checksum 比对（sha256 仅计算用于展示）
│      原子替换：写 self+".upgrade" → os.Rename
│      重启：RestartRunningAgents（仅 linux；darwin 直接报错 exit 1）
│      ⚠ 已接入机器不回退 GitHub（回退会"成功降级"）
└── 无凭证（纯 GitHub 安装机）
    → GITHUB 通道：api.github.com/zzjcool/homer/releases/latest
      下载 homer_<os>_<arch>.tar.gz + checksums.txt
      ✓ SHA256 校验（verifyChecksum）
      → tar 解包 → 写 target+".homer-new" → os.Rename
      → RestartRunningAgents
```

即：**agent 机器更新走我们自己的 Hub（`/dl/homer`），Hub 上的二进制由运维用 goreleaser 产物或源码构建替换。** 没有独立"Hub 更新服务"。

### 0.2 Hub 侧缺口（本次要补的）

| # | 缺口 | 证据 |
|---|---|---|
| G1 | Hub 无任何自重启原语（全仓唯一 `syscall.Exec` 在 agent 侧） | `internal/agentd/reexec_unix.go:18`；`internal/hub/`、`internal/web/` 无 exec |
| G2 | 无 hub 自更新端点（无 `/api/hub*`、无 `/api/version`） | `internal/web/handlers.go:99-222` 路由表 |
| G3 | 控制台无 Hub 运维入口（「更新程序」只针对 agent 机器） | `internal/web/static/index.html:2011-2017` |
| G4 | Hub 自身升级 = 纯手工 4 步运维 | `docs/ops/release-discipline.md:36-43` |
| G5 | 优雅停机能力已有但 web 层无法触发 | `internal/cli/hub.go:163-171`；`ServeOptions` 无 hook |

---

## 1. API 契约（D1–D8）

### D1 端点形状：三个端点，一个资源命名空间

| 方法 | 路径 | 语义 | 成功码 |
|---|---|---|---|
| GET | `/api/hub` | Hub 自身状态（版本/bootId/更新阶段/进度） | 200 |
| POST | `/api/hub/upgrade` | 检查→下载→校验→替换→重启 | **202** |
| POST | `/api/hub/restart` | 仅重启 | **202** |

- **否掉** `mode` 合并参数：两动作的响应契约根本不同（restart = 瞬时受理；upgrade = 5 分钟异步任务 + 结束时触发 restart）。合并会让同一 handler 产生阶梯状特例。
- **否掉** `/api/hub/actions`：URL 不再表达语义，与全仓「一条 path 一个 case」的手写 switch 风格冲突。
- 未来扩展（回滚、更新日志）走**同级动词 + 同一 GET**：`POST /api/hub/rollback`，`GET /api/hub` 多返回字段。不许加第三个 GET。

### D2 web 层解耦：`ServeOptions.HubOps` 接口字段

- **否掉** 对 `AgentsSource` 做 type assertion：hub 生命周期与「机器/同步」语义无关，会让所有 `AgentsSource` 实现（含测试桩）隐式背上 hub 职责，且"谁没实现"只能在调用点暴露。
- **否掉** `web` import `cli`：`cli` 已 import `web`（`internal/cli/hub.go` 组装 `ServeOptions`），反向即编译期循环依赖。
- 与既有 `Plugins *pluginruntime.State` / `Overrides *AgentOverrides` 的「nil 保持旧行为」先例一致。
- `HubOps == nil` → 三个端点全部 **501 `hub-disabled`**「这台 hub 不能自己更新」。**不许**用 404 或 503。

### D3 接口签名（冻结）

```go
// HubOperations is the hub's own lifecycle: self-update from releases and
// in-place restart. Nil keeps embedded servers compatible.
type HubOperations interface {
    HubStatus(ctx context.Context) HubStatus
    StartHubUpdate(ctx context.Context) (HubUpdateAccepted, error)
    RestartHub(ctx context.Context) (HubRestartAccepted, error)
}

type HubStatus struct {
    Version    string `json:"version"`
    BootID     string `json:"bootId"`
    Phase      string `json:"phase"`           // idle|checking|downloading|installing|restarting|failed
    CanUpdate  bool   `json:"canUpdate"`
    Latest     string `json:"latest,omitempty"`
    Downloaded int64  `json:"downloaded"`
    Total      int64  `json:"total"`
    Note       string `json:"note,omitempty"`
}

type HubUpdateAccepted  struct { Phase string `json:"phase"`; BootID string `json:"bootId"` }
type HubRestartAccepted struct { RestartID string `json:"restartId"`; BootID string `json:"bootId"`; Busy bool `json:"busy"` }

// HubError carries the HTTP mapping. cli constructs it (cli already imports web).
type HubError struct { Status int; Code, Message string; Details []string }
func (e *HubError) Error() string
```

- `*HubError` 之外的一切 error → 走既有 `writeErrorValue`，不得新造信封。

### D4 状态机：6 阶段，**只存内存**

| phase | 进入条件 | 前端文案要点 |
|---|---|---|
| `idle` | 初始 / 任务结束 | 空闲 / 「已是最新版本 vX」 |
| `checking` | POST 受理后立即 | 正在检查最新版本… |
| `downloading` | FetchLatest 返回且 IsNewer | 正在下载（downloaded/total） |
| `installing` | 下载+校验完成，落盘替换 | 正在安装… |
| `restarting` | 替换完成，即将 Exec | 正在重启中心… |
| `failed` | 任一步出错 | 失败原因（note），可重试；**必须区分**「未写入可重试」与「已写入未重启」 |

- **不落文件**：进程一重启进度按定义归零；唯一值得跨重载保留的信息（"新二进制已装好但没重启"）被「更新成功后自动重启」消除。
- `HubStatus` 读路径**不发网络请求**；`latest` 由后台 10 分钟 TTL 缓存刷新。

### D5 升级链顺序（不可调）

```
POST /api/hub/upgrade
→ 202 {phase:"checking"}
→ 后台任务：
   FetchLatest + IsNewer（不是新版本 → 409 hub-up-to-date 不成立，改走 idle+note）
   → downloading（下载 tar.gz + checksums.txt + SHA256 校验）
   → installing（temp + rename 替换二进制，先备份 <self>.prev）
   → RestartRunningAgents(≤10s, best-effort，非 Linux 只记 note)
   → restarting → Exec
```

- 并发再 POST → **409 `hub-update-running`**。
- `downloading|installing` 期间 POST restart 同样 **409 `hub-update-running`**（禁止在替换二进制中途 Exec）。
- **绝不原地写二进制**（否则同机 agent 吃 ETXTBSY/SIGBUS）。
- 纯重启**不触碰 agent 进程**（旧 inode 的 agent 与"远端未升级的 agent"等价，fleet 混跑是既有常态约束）。
- 升级路径**必须**重启本机 agent，否则它一直跑旧 inode（这正是 `release-discipline.md:42` 记「记得 restart homer-agent」的原因）。

### D6 版本通道 = GitHub Releases

```go
client := upgrade.NewClient(os.Getenv("HOMER_INSTALL_BASE_URL"))
latest, err := client.FetchLatest()
if !upgrade.IsNewer(latest.TagName, currentVersion) { /* 已最新 */ }
downloader := upgrade.NewDownloader(os.Getenv("HOMER_INSTALL_BASE_URL"))
err = downloader.InstallArchive(latest.TagName, self)  // 含 checksums.txt SHA256 校验
```

- **否掉** `/dl/homer`：它服务的正是**当前进程自己的二进制**，自更新 = `cp self self`，永远"成功"、永远无版本变化；且该端点对 agent secret 开放，把 hub 生命周期绑进 fleet 分发包的鉴权面。校验也丢了。
- 复用 GitHub 分支即得 `HOMER_INSTALL_BASE_URL` 镜像/离线 `file://` 支持与唯一版本判据。

### D7 dev 构建必须拒绝

- `web.Version == "dev"` 或 `!upgrade.IsParseableVersion(web.Version)`：
  - `GET /api/hub` → `canUpdate:false` + note「这是源码构建的 hub，不能自动更新」
  - `POST /api/hub/upgrade` → **409 `hub-dev-version`**「这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启」
  - **不提供 force**
- 理由：`hubVersionIsNewer`（`internal/web/handlers.go:1158-1175`）已冻结「dev hub 比任何 release 都新」，`handlers_version_test.go` 守着。若允许 dev hub 自更新，一次点击会把源码构建**降级**成 release 二进制。
- `HOMER_NO_VERSION_CHECK=1` → 同样 `canUpdate:false`。
- `!linux && !darwin` → restart 端点 **501 `hub-restart-unsupported`**。

### D8 错误码冻结

| 码 | HTTP | 文案 |
|---|---|---|
| `hub-disabled` | 501 | 这台 hub 不能自己更新 |
| `hub-restart-unsupported` | 501 | 此平台不支持自重启，请手动重启 homer serve |
| `hub-dev-version` | 409 | 这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启 |
| `hub-up-to-date` | 409 | 已是最新版本 |
| `hub-update-running` | 409 | 更新正在进行，完成后会自动重启 |
| `hub-update-failed` | 502 | details = err.Error() |
| `restarting` | 503 | 中心正在重启，请稍后重试（**独立于** `write-busy`） |

- `/api/hub` **禁止**加入 `principalAllowedForEndpoint` 机器 secret 白名单（默认拒绝是唯一防线）。

---

## 2. 重启机制契约（D9–D14）

### D9 主路 = `syscall.Exec` 原地 re-exec

```go
syscall.Exec(exe, normalizedArgs, os.Environ())   // linux || darwin
```

- 理由：`homer-serve` unit **不在仓库**、`Restart=` 未知。把重启正确性押在未验证的 `Restart=` 上，等于让「重启」在生产随机成功。Exec 保留 PID，`Type=simple` / `Restart=always` / `Restart=no` 三种配置下行为一致。
- 仓库已有同构原语与成熟范式：`internal/agentd/reexec_unix.go:10-19` + `internal/agentd/handlers.go:354-400`。
- **永不调用 `systemctl` 重启自身**（会与 supervisor 打架）。
- `!linux && !darwin` → `reexec_other.go` 返回错误，端点 501，**绝不 exit**。

### D10 re-exec argv 规范化

- 把 `--addr` 规范化为**已绑定地址**（缺失则追加）；其余参数与 `os.Environ()` 原样透传。
- 重绑端口需退避重试（100ms × 20 次）——exec 关掉监听 fd 后重占端口存在竞态。

### D11 Exec 前顺序固定

```
置 draining → agentHub.Shutdown(≤3s)（发 close 1001 "hub restarting"）
→ httpServer.Shutdown(≤2s，超时即弃，不等待 12min pull）
→ 2s 探针尝试拿 writeMutex（拿不到也继续）
→ syscall.Exec
```

- **禁止**在持有 `writeMutex` 时 Exec。
- **不拒绝重启**：pull 可占锁达 12 分钟，任何"忙就拒绝"规则等于让按钮随机失效，比中断一次同步糟得多。中断是安全的：generation 发布走 staging + rename 再翻 HEAD（`internal/gens/gens.go:99-113`），最多留 staging 残目录。
- 2s 探针拿不到锁 → 202 body 标 `busy:true`，前端如实提示「有同步被中断」。

### D12 Exec 失败回退链

| 情形 | 行为 |
|---|---|
| `syscall.Exec` 返回错误 且 `INVOCATION_ID` 为空（无 supervisor） | **保持服务不退出**，日志报 errno + 目标路径，`draining` 复位，返回错误 |
| `syscall.Exec` 返回错误 且 `INVOCATION_ID` 非空（有 supervisor） | 优雅停机 + `exit(1)`，交 supervisor 拉起 |
| Exec 成功 | 永不返回 |

- `exit` 会让 hub **彻底消失**，比"重启失败但仍可用"坏一个数量级 → 默认不退。
- `INVOCATION_ID` 是 systemd 对服务进程设置的廉价存在性探针，用它决定"失败后要不要交 supervisor"不需要读 unit 文件。

### D13 防崩溃循环：单份 `.prev` + 启动标记

- 替换前先把现二进制 rename 成 `<self>.prev`；失败即放弃本次替换。
- 标记文件 `<home>/keys/.hub-boot`：`{sha256, startedAt, attempt}`，启动时立即写；ready（监听成功）即删除。
- 回滚**仅当**：标记存在 && 距 startedAt < 90s && 标记 sha256 == 当前二进制 sha256 && attempt==2（第二次同字节短命启动）。
- 回滚 = `.prev` rename 覆盖 + 一次 Exec 进回滚版本，`/api/hub` 表面化 `rolledBack`。
- 回滚后**禁止**再触发第二次。不做多代历史、不做版本 pin。
- sha256 比对是关键：否则 `systemctl restart` 连点两次会误判并回滚一个**好**版本。

### D14 无 supervisor 场景一致

- `syscall.Exec` 是内核原语，不依赖 init：nohup / tmux / docker exec / 容器 PID 1 / macOS 终端行为全部一致（PID 不变、容器不重建）。
- 容器里 hub 常是 PID 1 或 shell 子进程，exec 后 PID 不变 ⇒ 容器不退出、`restart` 策略根本不会被触发（`e2e/docker-compose.yml:26` 恰好没配）——这是优点，不是缺陷。
- 启动/重启横幅必须区分「原地重启（PID 未变）」与「由 supervisor 重启」。

---

## 3. 前端契约（D15–D19）

### D15 入口 = header 文本按钮 + 独立 dialog

- `internal/web/static/index.html` header 新增 `<button class="text" id="btn-hub" type="button">Hub</button>`，紧跟 `#btn-plugins`。
- 打开 `<dialog id="dlg-hub">`。
- **否掉** footer「hub · 本机」可点：footer 是状态说明，小屏被压到页面末尾（`.statusline` 分段隐藏），「重启生产 hub」的入口不能藏在要滚到底的位置。
- **否掉** 复用插件 dialog：插件 dialog 承载「可同步的适配器/密钥载体/机器动作」，混入 hub 运维会让它的语义说明变成谎话，且进度条与危险按钮会被埋进滚动区。

### D16 header 显隐必须两处同步

`showGate()` 与 `showWorkspace()` 各自逐个 add/remove `hidden`，**新按钮不加就出现在登录页**。

### D17 listener 位置约束

所有新 listener **只能**追加在 `$("btn-plugins").addEventListener(...)` 之后。
**禁止**插在 `$("btn-logout")` 与 `$("btn-refresh")` 之间 —— `ui_issues_test.go` 用字符串切片切 logout 函数体，插入会污染切片。

### D18 交互范式照抄 `upgradeAgents()`

```
window.confirm → setBusy → POST → clearBusy → 轮询 health（不是 await refresh()）→ toast / showAlert
```

确认框文案（冻结）：

- 更新（已知 latest）：「把中心从 v1.4.0 更新到 v1.5.0？下载完成后中心会重启，控制台大约 10 秒不可用。」
- 更新（latest 未知）：「更新到最新版本？会先检查再下载；完成后中心会重启，控制台大约 10 秒不可用。」
- 仅重启：「重新启动中心？控制台大约 10 秒不可用，正在进行的同步会被中断。」
- **不要求管理员密码二次确认**：与 `upgradeAgent` / 吊销 / 移除的既有危险动作范式一致；cookie 已是 admin，在无 CSRF token 的现状下加密码框只是多一个可绕过的仪式。
- 必须警告「控制台不可用」，**不**写「机器离线」（agent 会自动重连，用户会误以为机器掉了）。

### D19 重启成功判据 = `bootId` 变化

- `/api/health` 新增 `bootId`（每进程随机 8 hex）与 `startedAt`。
- 判据严格为 `res.ok && data.bootId && data.bootId !== oldBootId`。**不看 version**：纯重启后 version 不变，且 dev hub 恒"最新"。
- 轮询 500ms，上限 60s，每次单独 `try/catch`（服务停机时 `api()` 会 reject）。
- `app.restarting = true` 期间 `refresh()` 直接 return，避免 30s 主轮询把 hero 打成「状态加载失败」。
- 成功确认后**必须**重新调 `loadHeaderVersion()`（现在只在 `boot()` 调一次），否则 header 版本永远显示旧值。

失败文案（冻结）：

- 超时标题「中心没有在预期时间内回来」；明细「已经等待 60 秒，健康检查仍然失败。」；提示「请在服务器上确认 homer 进程还在、查看它的日志；必要时重新执行 `homer serve`。」；动作「重新加载页面」。
  - **不许**写 `systemctl --user status homer-serve` —— 该 unit 名不在仓库内，写死会误导。
- 更新失败标题「中心没有更新成功」；明细为 `note`；按两条分支给提示（未写入→「可以直接重试」；已写入未重启→「新版本已经装好，重启中心就会生效」）。

### D20 文案冻结测试必须同步

- `internal/web/copy_test.go` 的 banned 列表含「推送」「拉取」「仓库」「commit」「bare」「处理改动」等；新文案不得触碰。
- required 列表需加入新冻结串（「Hub」「更新并重启」「仅重启」「重启中心」「中心已重启」「中心没有在预期时间内回来」等）。

---

## 4. 文件归属（强制，禁止越界）

| 归属 | 文件 | 说明 |
|---|---|---|
| **契约（已落库）** | `docs/plan/2026-10-18-hub-self-update-contract.md`、`internal/web/hubops.go` | 接口与类型定义，先于实现提交 |
| **Worker-CLI** | `internal/cli/hubops.go`（新建）、`internal/cli/hubreexec_unix.go`（新建）、`internal/cli/hubreexec_other.go`（新建）、`internal/cli/hub.go`（仅装配段）、`internal/cli/hubops_test.go`（新建） | HubOperations 实现、reexec、状态机、回滚 |
| **Worker-Web** | `internal/web/handlers.go`（三个 case + health + draining）、`internal/web/server.go`（ServeOptions 字段）、`internal/web/hubops_routes_test.go`（新建） | 路由、鉴权、错误映射 |
| **Worker-Front** | `internal/web/static/index.html`、`internal/web/copy_test.go`、`internal/web/ui_issues_test.go` | 按钮、dialog、轮询、文案冻结 |
| **主 agent** | 合并、集成测试、`docs/ops/release-discipline.md` 更新、`docs/api-reference.md` 补端点 | 三处缝隙由主 agent 亲自补 |

**共享文件不可并发**：`internal/web/hubops.go` 由主 agent 一次写完，worker 只读。

---

## 5. 验收命令（冻结）

**每个 worker 的 verify（CI 同款子集）**

```sh
gofmt -l . | grep -v vendor        # 必须空输出
go build ./...
go vet ./internal/web/... ./internal/cli/...
go test ./internal/web/... ./internal/cli/... -count=1
```

**已知基线红（不是本次引入，不要试图修）**

- `internal/gitx TestEnsureLocalIdentityWritesOnlyThisRepo` —— 宿主 git 自造身份 `Super User <root@host.host.zzj.cool>`
- `tests/integration TestRevokedSecretKicked` / `TestFiftyConcurrentTasks` —— 偶发（单独 `-count=3` 可过）

**完整验收（主 agent 合并前）**

```sh
test -z "$(gofmt -l . | grep -v vendor)" && go build ./... && go vet ./... \
  && go test $(go list ./... | grep -v '/tests/e2e') -count=1 -timeout 20m \
  && sh -n install.sh
```

---

## 6. 关键取舍记录（为什么不做）

| 不做 | 理由 |
|---|---|
| 用 `/dl/homer` 给 hub 自更新 | 服务的是自己，`cp self self`；且对 agent secret 开放，把 hub 生命周期绑进 fleet 分发包鉴权面；丢 checksum 校验 |
| 同步阻塞等 Exec 完成 | 终点是进程自杀，响应必然被 reset，用户永远看到"失败" |
| 忙就拒绝重启 | pull 可占锁 12 分钟，按钮会随机失效；中断是安全的（staging+rename） |
| 调 systemctl 重启自己 | 与 supervisor 打架 |
| 多代版本历史 / 版本 pin / 健康探测端点 | supervisor 的 `StartLimitBurst` 已限制爆炸半径；`<self>.prev` + 单标记文件足够兜住"新二进制启动即崩" |
| 状态落文件 | 进程重启后进度按定义归零，落文件只留陈旧状态待清理 |
| dev hub 允许自更新 / force | 会把源码构建降级成 release 二进制，且与 `hubVersionIsNewer` 的冻结语义冲突 |
| 重启要管理员密码二次确认 | cookie 已是 admin，无 CSRF token 现状下加密码框不增加安全边界，只是仪式 |
| 重启时顺手重启本机 agent | 旧 inode agent 与"远端未升级 agent"等价，fleet 混跑是既有常态；纯重启不新增风险模型 |
| footer 做入口 / 复用插件 dialog | 见 D15 |
| 文案写死 `systemctl --user status homer-serve` | 该 unit 不在仓库内，写死会误导（这是 advisor-0 被 advisor-1 推翻的一条） |

---

## 7. 未验证项（实现时必须诚实处理）

1. **生产 `homer-serve` unit 的 `Restart=` 与 `Type=notify`** —— unit 不在仓库，仅有 `release-discipline.md:36-43` 的手工 SOP。D12 的 `INVOCATION_ID` 分支因此是"探测式"而非"已知式"。
2. **pull 全程是否持有 `writeMutex` 且在锁内单点发布** —— 只核到 `gens` 的 staging+rename+HEAD 翻转是原子的（`internal/gens/gens.go:99-113`），未逐行确认持锁边界。若实现时发现增量发布点，D11 的"不拒绝重启"需改为"busy 时二次确认"。
3. **`HOMER_INSTALL_BASE_URL` 为空 → 真实 GitHub** 在受限网络下能否 5 分钟内完成 —— e2e 只有 install 脚本侧的 stub 分发。
4. **手机 320–375px 宽度下 header 再加一个按钮是否溢出** —— `.topbar` 小屏规则仅 `flex-wrap`，未实测。
