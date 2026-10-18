# Worker-CLI 交付报告：Hub 自更新 + 自重启的 CLI 侧

分支 `pi-subagent/worker-0-7df4acce` · commit `4681a65` · MR https://github.com/zzjcool/homer/pull/138

## 一、改动文件清单（严格在任务卡所有权内）

| 动作 | 路径 | 说明 |
|---|---|---|
| 新建 | `internal/cli/hubops.go` | `HubOperations` 实现、6 阶段状态机、升级链、draining/exec 顺序、D12 回退链、D13 回滚、bootId、argv 规范化、监听退避 |
| 新建 | `internal/cli/hubreexec_unix.go` | `//go:build linux \|\| darwin`：`hubReexec` = `syscall.Exec`，`hubReexecSupported()=true` |
| 新建 | `internal/cli/hubreexec_other.go` | `//go:build !linux && !darwin`：`hubReexec` 返回 error，`hubReexecSupported()=false` |
| 新建 | `internal/cli/hubops_test.go` | 21 个测试（含表驱动子用例），覆盖任务卡列出的全部必测项 |
| 改动 | `internal/cli/hub.go` | **仅** runServe 装配段：装配 `web.ServeOptions.HubOps`、`resumeBoot`/`setBoundAddr`/`markReady` 调用、serve 主体提成 `buildStack` 闭包 + serve loop（D12 无 supervisor 恢复用） |

未触碰（已用 `git status` 确认）：`internal/web/**`、`internal/upgrade/**`、`internal/cli/commands/**`、`docs/**`。

## 二、逐条 D 编号落实

### D4 状态机（6 阶段，只存内存）
`hubPhaseIdle/Checking/Downloading/Installing/Restarting/Failed` 六个常量，`hubOps.mu sync.Mutex` 保护全部字段，无任何文件持久化。
- 并发二次 POST upgrade：`acceptUpdate` 在锁内检查 `running || phase ∉ {idle, failed}` → `*web.HubError{409, hub-update-running}`。
- `downloading|installing` 期间 restart：`RestartHub` 锁内同样判定 → 409。
- `failed.note` 两条分支由**不同代码路径**产出，不可混淆：
  - 下载/校验/安装失败 → `... + hubNoteNotWritten`（「当前程序没有被替换，可以直接重试」）
  - 替换成功但 Exec 失败 → `hubNoteInstalledNotRestarted`（「新版本已装好，但中心没有重启；请在服务器上重新执行 homer serve」）
- 测试：`TestHubUpdateStateMachineRejectsConcurrentUpdate`、`TestHubRestartRejectedWhileUpdateRuns`、`TestHubUpdateDownloadFailureIsRetryable`、`TestHubUpdateExecFailureReportsInstalledNotRestarted`、`TestHubRejectedRestartDoesNotPublishDraining`。

### D5 升级链（顺序不可调）
`runUpdate`：`fetchLatest` → `upgrade.IsNewer(target, web.Version)` → `installArchive(target, self+".hub-new")` → `restartAgentsBestEffort()`（goroutine + `hubAgentRestartWait=10s` select）→ `phase=restarting` → `drainAndExec()`。
- 不是新版本 → `phase=idle` + `note="已是最新版本 <tag>"`（测试 `TestHubUpdateAlreadyLatestIsIdle`）。
- 非 Linux 的「需要手动重新启动 agent」只拼进 note，**不**让更新失败（测试 `TestHubUpdateAgentRestartDegradationDoesNotFail`）。
- `InstallArchive` 目标是 `self + ".hub-new"`，**绝不**直接写 `self`；最终由 `replaceSelf` 两步 rename 落位（测试 `TestHubUpdateReplacesViaBackup` 断言 install target 以 `.hub-new` 结尾）。
- 纯重启不碰 agent：测试 `TestHubPureRestartDoesNotTouchAgents` 断言 `agentRestarts == 0`。

### D6 版本通道
`upgrade.NewClient(os.Getenv("HOMER_INSTALL_BASE_URL")).FetchLatest()`、`upgrade.NewDownloader(同 baseURL).InstallArchive(tag, staged)`（内含 checksums.txt SHA256 校验）。没用 `/dl/homer`。

### D7 dev 拒绝
`versionUpdatable()`：`web.Version` 去空格后非空、非 `"dev"`、且 `upgrade.IsParseableVersion` 为真。
- 否则 `HubStatus.CanUpdate=false` + note（「这是源码构建的 hub，不能自动更新」）。
- POST upgrade → `*web.HubError{409, hub-dev-version, "这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启"}`。
- `HOMER_NO_VERSION_CHECK=1` → `CanUpdate=false` + `hubNoteNoCheck`，POST 同样 409。
- 无 force 开关。测试：`TestHubDevVersionCannotUpdate`（`dev`/`""`/`not-a-version` 三例）、`TestHubVersionCheckDisabledBlocksUpdate`。
- 非 linux/darwin restart → `{501, hub-restart-unsupported}` 且**绝不 exit**：`TestHubRestartUnsupportedPlatform` 断言 `exitCode` channel 为空、无 exec 调用。

### D8 错误码
全部经 `hubError(status, code, message) = &web.HubError{...}`。`hub-disabled`(501) 按契约由 web 层 `HubOps == nil` 分支产生，本包不重复实现。

### D9/D14 re-exec 原语
- `hubreexec_unix.go`：`hubReexec(exe, argv, env)` = `syscall.Exec`。语义：成功永不返回。
- `hubreexec_other.go`：返回 error，`hubReexecSupported()=false` → RestartHub 在 `drainAndExec` 之前就 501 返回，永不 exit。
- D14 横幅：`hubBootMarker.PID` 在启动时写入；下次启动比较当前 `os.Getpid()`——相同 → 「已原地重启（PID 未变），端口和会话保持原样。」，不同 → 「已由 supervisor 重新拉起（上一个进程 PID N）。」**真实进程已验证**（见第五节）。跨平台交叉编译已验证 linux/darwin/freebsd 可编译（windows 因既有 `internal/gitx`、`internal/manifest` 用 `syscall.Setpgid` 而失败，master 上同样如此，非本次引入）。

### D10 argv 规范化 + 重绑退避
`normalizeReexecArgs(args, boundAddr)`：
- 无 `--addr` → 追加 `--addr <boundAddr>`；
- `--addr <v>` → 替换值（并跳过旧值）；
- `--addr=<v>` → 替换；
- `--` 之后原样保留，但 `--addr` 若尚未出现则插在 `--` **之前**；
- 空 argv → `["homer","serve","--addr",addr]`；
- 不改写入参切片（`os.Args`）。
测试 `TestNormalizeReexecArgsReplacesAddr` 5 个子用例。
`listenHubWith` 只对 EADDRINUSE（`errors.Is(syscall.EADDRINUSE)` 或字符串兜底）重试，`hubListenRetryCount=20 × hubListenRetryDelay=100ms`；其他错误（权限等）立即返回。测试 `TestListenHubRetriesAddressInUse`。

### D11 Exec 前顺序（不可调）
`RestartHub` / `runUpdate` 的执行序：
1. `setPhase(hubPhaseRestarting, "")` —— **最先**把 draining 发布出去；
2. `drainAndExec()`：
   - `sleep(hubRestartGrace=200ms)` 宽限窗（注释里写明「phase 是 web 层唯一可见的 draining 依据」这条契约）；
   - `agentShutdown(ctx, 3s)`（close 1001「hub restarting」）；
   - `httpShutdown(ctx, 2s)`（超时即弃）；
   - `tryWriteLock` 2s 探针（拿不到也继续；**不**在持锁状态 Exec —— 探针内部自行释放）；
   - `exec(self, normalizedArgs, environ)`。
- 不因 busy 拒绝重启：`writeBusy` 只是探测并把结果作为 `Busy` 放进 202 body（测试 `TestHubRestartBusyProbeNeverRefuses`）。
- 顺序有专门测试：`TestHubRestartPublishesDrainingPhase` 在 exec 的**那一刻**采样 `HubStatus.Phase`（必须为 `restarting`）并断言 `agentShutdowns==1 && httpShutdowns==1`（两者都发生在 exec 之前）。

### D12 Exec 失败回退链
`execFailure(err, note)`：
- `invocationID() != ""` → `setPhase(failed, note+"（已交给 supervisor 重新拉起）："+err)` + `stopAndExit()`（agent/http 各 1s 收尾）→ `exit(1)`。**不发** in-place 恢复信号。
- `invocationID() == ""`：
  - 更新路径（note 非空）→ `phase=failed` + 「新版本已装好…」note，draining（= phase==restarting）自动解除；
  - 纯重启（note 空）→ `phase=idle` + 「中心没有重启：<err>。可以直接重试。」。
  - 两种情况都向 `execFailed`（buffer 1）发信号，`runServe` 的 serve loop 收到后**重新 bind + 重建 stack** 继续服务（因为 `agentHub.Shutdown` 会 latch `closing`、`httpServer.Shutdown` 会永久关掉 listener，栈不可复活——这条写进了 `buildStack` 与 `serveStack` 的注释）。这正是 D12「保持服务不退出」的硬要求。
- 测试：`TestHubExecFailureWithoutSupervisorKeepsServing`（断言不 exit、phase 回 idle、note 含 errno、两次 drain 各 1 次）、`TestHubExecFailureWithSupervisorExits`（断言 `exit(1)`、且**不**发恢复信号）。
- 永不调用 systemctl：全文件无 `systemctl` 字样。

### D13 防崩溃循环
- `replaceSelf`：先 `os.Remove(<self>.prev)`（单份，无历史）→ `os.Rename(self, .prev)`（失败即放弃）→ `os.Rename(staged, self)`；第二步失败则把 `.prev` 改回去再报错。
- 标记 `<home>/keys/.hub-boot`，JSON `{sha256, startedAt, attempt, pid, rolledBack}`。写入点：`runServe` 在 `net.Listen` **之前**调 `resumeBoot()` → `beginBoot` 立即写；`markReady()` 在 bind 成功后**立刻**删除。
- 回滚判定全四条件（`beginBoot`）：
  - `hadMarker`
  - `now.Sub(previous.StartedAt) < hubBootRollbackWindow(90s)`
  - `strings.EqualFold(previous.SHA256, selfSHA)`（sha256 相同）
  - `previous.Attempt == 2`
  - 另外 `previous.RolledBack` 为真时只上报「本次是回滚后的启动」，**永不**回滚第二次。
- 计数语义：`attempt` 在 sha256 相同且 <90s 时递增（`previous.Attempt+1`）；`attempt==1` 只记录不回滚，下一次（`attempt==2`）才回滚——即「第二次同字节短命启动」才触发，与契约措辞一致。
- 回滚动作：sha256 先算（rename 后 `.prev` 就不叫这个名字了）→ `Rename(.prev, self)` → `RolledBack=true` → 写新标记（`RolledBack:true`、`Attempt:1`，防二次）→ 一次 Exec（argv 同样规范化）→ `HubStatus.RolledBack=true`。
- 回滚自己失败（找不到 `.prev` / rename 失败）→ 只 `log.Printf`，不拖垮 hub。
- 测试 `TestHubRollbackRequiresAllFourConditions` 覆盖 6 个子用例：无标记 / sha256 不同 / 同 sha 但 attempt==1 / 同 sha attempt==2 在窗内（→回滚，校验 `.prev` 内容已覆盖 self、标记变 `rolledBack+attempt1`、argv 含 `--addr 127.0.0.1:7760`）/ 同 sha attempt==2 但超 90s / 已 `rolledBack` 过一次。另有 `TestHubMarkReadyRemovesBootMarker`。

### D19 bootId
`newHubOps` 里 `randomHexString(4)` = `crypto/rand` 8 hex，进程内不变。测试 `TestHubBootIDIsStable`：长度 8、5 次读一致、两个实例不同。

### HubStatus 不联网
`HubStatus` 只读锁内字段；`latest` 过期（`hubLatestTTL=10min`）时置 `refreshing` 标记后 `go o.refreshLatest()`。失败也更新 `latestAt`（避免每次轮询都打网络）。测试 `TestHubStatusDoesNotBlockOnNetwork`：让 `fetchLatest` 阻塞在 channel 上，断言 `HubStatus` 2s 内返回且 `Latest==""`，放行后缓存被填上。

### 其余
- 日志：标准库 `log`，用户可见文案全中文。
- 可测试性：`hubOps` 上 `now/sha256/exec/environ/invocationID/reexecSupported/sleep/exit/randomHex/fetchLatest/installArchive/restartAgents/agentShutdown/httpShutdown/tryWriteLock` 全为可注入字段，生产默认在 `newHubOps` 里装配。

## 三、新增测试名（21 个）

1. `TestHubUpdateStateMachineRejectsConcurrentUpdate`
2. `TestHubRestartRejectedWhileUpdateRuns`（3 子例：checking/downloading/installing）
3. `TestHubDevVersionCannotUpdate`（3 子例：dev/""/not-a-version）
4. `TestHubVersionCheckDisabledBlocksUpdate`
5. `TestHubBootIDIsStable`
6. `TestNormalizeReexecArgsReplacesAddr`（5 子例）
7. `TestHubRollbackRequiresAllFourConditions`（6 子例）
8. `TestHubMarkReadyRemovesBootMarker`
9. `TestHubExecFailureWithoutSupervisorKeepsServing`
10. `TestHubExecFailureWithSupervisorExits`
11. `TestHubUpdateExecFailureReportsInstalledNotRestarted`
12. `TestHubUpdateDownloadFailureIsRetryable`
13. `TestHubUpdateAlreadyLatestIsIdle`
14. `TestHubUpdateAgentRestartDegradationDoesNotFail`
15. `TestHubPureRestartDoesNotTouchAgents`
16. `TestHubRestartUnsupportedPlatform`
17. `TestHubRestartPublishesDrainingPhase`
18. `TestHubRejectedRestartDoesNotPublishDraining`
19. `TestHubRestartBusyProbeNeverRefuses`
20. `TestHubStatusDoesNotBlockOnNetwork`
21. `TestListenHubRetriesAddressInUse` / `TestHubUpdateReplacesViaBackup`

## 四、验证输出（原样）

```
### gofmt -l . | grep -v vendor
(empty output)

### go build ./...
OK (no output)

### go vet ./internal/cli/...
OK (no output)

### go test ./internal/cli/... -count=1
ok      github.com/zzjcool/homer/internal/cli           0.387s
ok      github.com/zzjcool/homer/internal/cli/commands  3.013s
```

附加（超出要求，用于自证无竞态）：`go test ./internal/cli/... -count=1 -race` → 两个包均 `ok`（1.600s / 5.470s）。

已知基线红 `internal/gitx TestEnsureLocalIdentityWritesOnlyThisRepo` 未触碰、未修改。

## 五、真实二进制端到端 smoke（超出单测，验证真实 exec 路径）

用 `go build -o /tmp/homer-smoke ./cmd/homer` 后：

1. **正常启动**：`homer serve --addr 127.0.0.1:17761` → `keys/` 下无 `.hub-boot`（ready 已删除）；`/api/health` 返回 `{"ok":true,"status":"healthy","version":"dev",...}`；`/api/hub` 无鉴权 → 401（默认拒绝，符合 D1/D8）。
2. **崩溃循环回滚**：造 `keys/.hub-boot = {sha256: <当前二进制>, startedAt: now, attempt: 2, pid: 999999}` + 存在 `homer.prev` → 启动输出：
   ```
   homer serve: 新版本启动未成功，正在回滚到上一版程序（.../homer）
   homer serve: 已原地重启（PID 未变），端口和会话保持原样。
   homer serve: 上一版程序启动失败，已回滚到原来的版本。
   homer serve: http://127.0.0.1:17762（Ctrl+C 停止）
   ```
   → 回滚发生、exec 成功、PID 未变（D14 横幅正确）、回滚后的进程读到 `RolledBack` 标记上报回滚、ready 后标记被删、**未二次回滚**。
3. **attempt==1 反例**：同 sha 但 `attempt: 1` → 输出 `已由 supervisor 重新拉起`（PID 不同），**无**回滚，验证 sha256 相同但 attempt==1 时不回滚。

## 六、契约矛盾点 / 需要主 agent 裁决的地方

### 1) D11 的「2s 写锁探针」在本 worker 的文件所有权内无法真正实现（**需要 Web worker 补一个 seam**）

`internal/web` 的写锁是**包级私有** `var writeMutex = make(chan struct{}, 1)`（`internal/web/server.go:259`），`lockWrite`/`unlockWrite` 也未导出。任务卡禁止我改 `internal/web/**`，因此 `cli` 包**没有任何合法途径**在 2s 内试拿这把锁。

我的处理（不阻塞交付）：
- 设计成可选 seam `hubOps.tryWriteLock func(context.Context) bool`，生产路径**默认为 nil**；
- `nil` 时 `Busy=false`，restart 照常进行（这正是 D11「不拒绝重启」的语义，所以**功能上无缺陷**，只是 `Busy` 永远报 false）；
- 一旦 Web worker 在 `ServeOptions` 上暴露一个 `TryWriteLock func(ctx) bool`（或导出等价的 `web.TryLockWrite`），CLI 侧只需在 `runServe` 里加一行 `hubOps.tryWriteLock = ...` 即可点亮 `Busy`，无需改动其它逻辑。

**建议**：请主 agent 在合并 Web worker 的 `server.go` 改动时，顺手加一个导出的探针（例如 `func TryWriteLock(ctx context.Context) bool`，内部 `select writeMutex <- struct{}{} / ctx.Done()` 并在成功时立刻 `<-writeMutex`）。这不是契约矛盾，而是**跨 worker 的共享面缺口**，属于主 agent 契约里标注的「合并阶段亲自补的缝隙」。

同理，**draining（D11）** 的可见性：我按任务卡第 3 条实现决策，只把 `phase=restarting` 作为唯一信号并留 200ms 宽限窗；`internal/web` 侧需要 Web worker 读 `HubOps.HubStatus(ctx).Phase` 来返回 503 `restarting`。若 Web worker 没实现，功能退化为「重启期间写请求被 drain 中断」——仍是 D11 允许的行为（「中断是安全的」），只是不给用户 503 提示。这一点已写进 `drainAndExec` 的代码注释。

### 2) `internal/web/hubops.go` 的 `hubPhase*` / `hubCode*` 常量是小写未导出

我在 `internal/cli/hubops.go` 里**字面镜像**了这 12 个字符串常量并加了注释说明「改一处必须改两处，drift 会静默关掉 draining」。这是接口冻结带来的必然重复：`cli` 无法引用 web 包内未导出标识符。若主 agent 希望消除重复，可以把它们导出（`web.HubPhaseRestarting` 等）——但这会改动共享面 `internal/web/hubops.go`，超出我的所有权，故未做。

### 3) 阶段判定 D4 与阶段 ⑤ 的一个措辞细节（已在实现中定稿，供 review 确认）

契约 D13 写「attempt==2（第二次同字节短命启动）」。我把计数定义为：第一次短命启动写 `attempt=1` 且**不回滚**（此时无法区分「偶发一次」与「开始循环」），第二次（`attempt=2`）才回滚。另一种读法是「第一次短命启动就写 attempt=1，第二次启动读到 attempt=1 → 立刻回滚」。我选了前者的理由：D13 原文是「第二次同字节短命启动」，且单次启动失败可能是偶发（如端口暂被占），立刻回滚会把一个正常版本换掉。若 review 认为应符合后一种读法，改动点只有 `beginBoot` 里的 `rollbackRequired = previous.Attempt == 2` 一行，测试表也需相应调整。

### 4) D5 里 `/api/hub/upgrade` 的 502 `hub-update-failed`

契约 D8 保留该码，但 D5 的升级是**全异步**（202 后后台跑），所以「下载失败」这类错误发生时代码已无法回给调用方。我只在**同步阶段**（`acceptUpdate` 里 `self` 路径解析失败，即 `os.Executable()` 不可用）用 502 `hub-update-failed`；其余一律走 `failed` phase + note。这与契约意图一致（note 驱动前端文案），但严格说 D8 的 502 在异步链上永远不可达——记录于此供 review。

## 七、未决问题

1. 上述 §六.1 的 `tryWriteLock` seam 需要 Web worker / 主 agent 补上，否则 `Busy` 恒为 false。
2. `internal/cli` 在 `GOOS=windows` 无法编译，但原因在既有依赖包 `internal/gitx`、`internal/manifest`（`syscall.Setpgid` 等），master 上同样失败，**不是本次引入**。已用独立探针包确认 `syscall.EADDRINUSE` 在 windows 存在，故我的 `hubAddrInUse` 本身可移植。
