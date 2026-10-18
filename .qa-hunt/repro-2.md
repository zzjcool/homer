# 复现判定 · 第 2 组（角度 B 全部 + 角度 C 的 C4）

仓库 /root/code/homer HEAD=010521a · hub http://127.0.0.1:17760 · box=homer-qa-box-b · 判定人 qa-repro（独立 harness `.qa-hunt/repro2-lib.mjs`，全部脚本自写于 `.qa-hunt/r2-*.mjs`，截图 `.qa-hunt/shots-repro2/`）。
**判定纪律**：所有判据与 hunter 不同源——CDP 网络层计数/阻断（非 window.fetch 打桩）、元素身份标记、页面内焦点采样、剪贴板降级路径观测、Go 层纯函数探针。全程未改被测源码（`zz_c6probe_test.go` 为临时 Go 测试文件，跑完即删，git 干净）。

---

## B-C1 copyJoin 在非安全上下文完全失效 → **true**（高，维持）
- **我的独立判据**：①对照实验——同一页面同一环境下，有守卫的 copyCommand/copyAgentId 必须走降级（window.prompt 弹出/文案变化），无守卫的 copyJoin 若也走降级则必须弹 prompt 且显示 #copy-fallback；②真实鼠标 Input.dispatchMouseEvent 点击 #btn-copy（非 .click() 捷径）+ window.addEventListener('error') 独立错误通道（不用 hunter 的 window.__err 注入方式）。
- **实机**：`r2-c1.mjs`/`r2-c1b.mjs`/`r2-c1c.mjs`，3 轮（2 轮程序调用 + 1 轮真实鼠标）全部一致：
  - copyJoin → `THROW:TypeError: Cannot read properties of undefined (reading 'writeText')`；btn 保持「复制」；`#copy-fallback` 仍 hidden；prompt 未弹；`window.onerror` 捕获同型错误（独立通道）
  - 对照 copyAgentId → prompt「复制失败，请手动复制完整 agentId」+ toast（降级正常）
  - 对照 copyCommand → prompt「复制这条命令…」（降级正常，不抛）
  - 前置：`isSecureContext=false`、`typeof navigator.clipboard === 'undefined'`（hub:7760 是 README 明示的 http 访问形态）
- **与猎人差异**：不依赖注入的错误收集器做主判据；用「同页有守卫两处不抛、无守卫一处抛」的差分证明，并补真实鼠标轮。
- **截图**：C1-round3-realclick.png、C1-B-copyagentid.png
- **严重度**：高（内网 http 是 README 主推用法，接入第一动作即复制；用户以为复制成功实际剪贴板为空——execCommand 对照轮已证降级路径完全没走到）。

## B-C2 假取消 + 孤儿回调写穿对话框 → **true**（高，维持；比猎人更强：真实慢后端）
- **我的独立判据**：①静态事实链：`beginDispatchWait` :3489 `cancel.disabled=true`，`btn-confirm-cancel` click 处理器只 `close("cancel")`（disabled 时点不动），`dlg-confirm` close :3167 只 stopDispatchWait/abort **collect** 流，不 abort `/pull`；②真实慢后端：`kill -STOP` box-b 真实 agent 进程（pid 1005，不是打桩 fetch），pull budget 12min（proto.go:63）；③真实鼠标点取消 + 真实 Esc + MutationObserver 记录 #confirm-desc/#view-title。
- **实机**（`r2-c2final3.mjs`）：
  1. agent 在线时打开「下发到 b6675d68ddf7-8e7b」对话框，勾 pi（choices 就位后 STOP，规避 choices 503 agent-offline）
  2. `kill -STOP 1005` → 点确认 → `[WAIT] {"open":true,"okText":"正在下发… 3 秒","cancelDisabled":true,"backHidden":true,"step":"wait"}` ——截图 C2f-b-waiting-REAL.png 可见「取消」灰色不可点
  3. 真实鼠标点「取消」→ `{"open":true,"okText":"正在下发… 4 秒"}` 完全无响应（假取消证实）
  4. Esc → 对话框关闭、`app.pendingAction=null`，**但 /pull fetch 仍在途**
  5. 打开另一台机器（54d0c3373428-bc77）的「查看」对话框（`#dlg-view` open，标题「机器 54d0c3373428…」）
  6. `kill -CONT 1005` → 孤儿 fetch 返回 → MutationObserver 记录 `#confirm-desc=已下发到「b6675d68ddf7（b6675d68ddf7-8e7b）」。`，且 `app.pendingAction={kind:"dispatch-done"}` ——写下发结果时用户开着的是**另一台机器**的对话框（截图 C2f-d-second-dialog-open.png / C2f-e-after-orphan.png）
- **与猎人差异**：慢后端不是 harness 挂 fetch，而是 `kill -STOP` 真实 agent 进程；取消无效用真实鼠标事件；两个 half 都不依赖 JS 打桩。hunter 提到的行号 :3489/:3553 全部核实无误。
- **kill 恢复声明**：实验结束已 `kill -CONT 1005`，box-b agent 进程现处 S（运行）态。
- **严重度**：高（用户明确取消/Esc 逃逸后操作仍会执行，且结果写进不相干对话框的状态机）。

## B-C3 30s 轮询重建卡片：菜单收起 + 焦点丢失 → **true**（中，维持）
- **我的独立判据**：①**元素身份**而非 .open 布尔：给展开的 `details.agent-more` 打 `__r2mark` 标记，标记存活=元素没被重建（open=false 也可能来自全局 pointerdown 关闭，标记消失只能来自 `clear(list)` 重建，:1854）；②焦点用**页面内 setInterval 500ms 采样**记录 activeElement 序列，CDP 全程不发焦点相关调用（Runtime.evaluate 只读 localStorage/样本数组）——排除 hunter 自认无法排除的 harness 伪影；③footer #refreshed-at 变化作为「真实轮询已发生」的门。
- **实机**（`r2-c3.mjs`/`r2-c3b.mjs`，2 实验 + 1 对照）：
  - exp-1：轮询前 `{ae:BUTTON|详情, mark:R2MARK, open:true}` → footer 变化（06:48:51→06:49:08）后 `{ae:BODY, mark:GONE, open:false}`，页面侧捕到 `focusout` 事件
  - control-2：8 秒无轮询窗口（footer 不变）内 16 个采样全部 `{mark:R2MARK, open:true, ae:BUTTON}` ——标记存活、焦点稳定
  - exp-2：轮询后（06:49:40→06:49:57）同样 `mark:GONE, ae:BODY`
- **与猎人差异**：身份标记 + 页面内采样 vs .open 布尔 + CDP 直读 activeElement；并补了无轮询对照证明不是时间自然衰减。
- **截图**：C3-a-menu-open-before-poll.png、C3-b-after-real-poll.png
- **严重度**：中（30s 必然撞上正在展开菜单/键盘定位的用户）。

## B-C4 退出失败照样回登录门、服务端会话仍 200 → **true**（中，维持）
- **我的独立判据**：①用 **CDP `Network.setBlockedURLs`** 在网络层阻断 `*auth/logout*`（不碰 window.fetch，排除打桩伪影）；②服务端会话真值用**页面 same-origin fetch `/api/console` 的状态码**（凭据在浏览器 cookie jar，不由前端任何 state 决定），解封后读。
- **实机**（`r2-c45.mjs`）：基线 `/api/console`=200 → 阻断 logout → 点 #btn-logout → `{workspaceHidden:true, gateLoginShown:true, errLogin:"", toast:"已登录"(残留)}` ——无任何错误提示（截图 C4-logout-blocked-no-error.png）→ 解封后 `fetch('/api/console',{credentials:'same-origin'})` **仍 200**（会话未被服务端终止，截图 C4-server-session-still-200.png）。
- **与猎人差异**：网络层阻断 + 解封后同源探活，代替 fetch 打桩；结论一致且更贴近真实「hub 瞬断」形态。
- **静态核对**：:5174-5177 `catch(_){}` 后无条件 `showGate("login")`；服务端 logout 处理器（password.go:369）本会 clear cookie——被阻断时 cookie 未清，与 200 实测互洽。
- **严重度**：中（登出的安全语义被静默破坏；刷新即回到已登录工作区）。

## B-C5 #btn-refresh 无防抖无反馈 → **true**（低，维持；仅已证实的半段）
- **我的独立判据**：**CDP Network.requestWillBeSent 事件计数**真实 /api/agents 请求数（不经页面任何 JS 钩子）；反馈用 t+0.3s 的 #busy/#btn-refresh.disabled/aria-busy 快照；实验/对照各 2 轮，其中 1 轮用真实鼠标连点（排除 .click() 伪影）。
- **实机**（`r2-c5.mjs`）：control-1 单点→1 个请求；exp-1 连点 3 次→**3 个请求**，t+0.3s `{busyHidden:true, btnDisabled:false, ariaBusy:null}`；control-2 单点→1；exp-2 真实鼠标连点 3 次→**3 个请求**。截图 C5-a/C5-b。
- **与猎人差异**：请求计数从页面 JS 计数改为协议层事件，无需打桩延迟即可证并发（猎人靠 3s 延迟桩才数到 3）。
- **严重度**：低（「响应乱序显示旧数据」仍未证实，不并入）；无反馈+可无限连点是体验缺陷而非正确性缺陷。

## B-C6 机器卡片 NaN%（mem.used 缺失）→ **downgraded（理论风险，当前真实不可达）**
- **我的独立判据**（不用 JS 注入，攻「真实 agent 能否产出 used 缺失」）：
  1. **agent 采集层**（hoststat.go:162-174）：`parseMeminfo` 只有 `MemTotal>0` 才产出 mem，`used = total - avail`（avail 缺失回退 MemFree+Buffers+Cached，avail>total 被钳到 total）→ `used` 恒为非负整数，不存在「缺字段」。单测 TestParseMeminfo 实跑 PASS。
  2. **传输/规范化层**（hub.HostMemory host.go:37-38 `Total/Used uint64` **无 omitempty**；normalizeHost 只装非指针字段，经 webHost JSON 往返）→ Go 临时测试实测：`HostMemory{Total:100}` 序列化为 `{"total":100,"used":0}`，`{"total":100}` 反序列化 used=0——**used 键永不缺失**，NaN 的前提（`undefined/total`）在真实链路不可达。
  3. `normalizeMemory`（host.go:150-159）nil/Total==0 → nil（前端整段不渲染）；used>total 被钳制，不产生负值/NaN。
  4. 线上 5 台机器 /api/agents 全部同时带 total+used（基线正常）。
- **结论**：hunter 的 NaN 复现是 **JS 注入伪影**（喂了真实链路产不出的 `{total:100}` 缺 used 形状）。前端 `hostSummary:1813`/`memText:1752` 对 `used` 无守卫属实的防御性缺口，但触发条件在当前 agent 代码下不可达 → 按 **downgraded** 记为理论风险（若未来第三方/旧版 agent 直报 `/api/snapshot` 缺 used 键，Go 零值也会补成 0，仍不 NaN）。
- **证据**：`go test ./internal/agentd -run TestParseMeminfo` ok；临时探针输出（已删文件）：`{"total":100,"used":0}` / `normalizeMemory({Total:100,Used:250}) → Used=100`。
- **严重度**：降级为「加固建议」：hostSummary 内存分支补 `used != null` 是一行防御，但不构成用户可遇缺陷。

## B-C7 接入码轮询 catch(_){} 断网无提示 → **true**（低→中，上调）
- **我的独立判据**：CDP `Network.setBlockedURLs` 阻断 `*join/status*`（非 fetch 桩）+ 对照轮（不阻断）测同一对话框的基线行为；判「无提示」用对话框全文错误词扫描（失败/错误/重试/无法/连不上/断开）。
- **实机**（`r2-c7b.mjs`，2 轮 + 对照）：对照轮 7s 内 7 次 join/status 全部成功（正常等待，静默合理）；实验轮阻断后 6 次请求全部被 CDP 拒掉，`#dlg-join` 仍 open，全文 `"机器用这个接入码连上之后，窗口会自己关上。"` ——**0 个错误词**，toast 是登录残留「已登录」（截图 C7-b-join-poll-silent.png）。静态 :4764 `catch (_) {}` 核实。
- **与猎人差异**：网络层阻断 vs fetch 桩；补对照轮证明「静默」不是等待态的固有设计（等待态的静默发生在请求成功时；断网时同样静默才是缺陷）。
- **严重度**：低→**中**：对话框文案明确承诺「窗口会自己关上」，断网时承诺变谎言且用户无从区分「等机器」vs「问不到服务器」；hub 重启（qa 环境常态）即可触发。

## C-C4 控制台红色告警栏显示内部 token `partial` → **true（root-cause-elsewhere）**（中，维持）
- **我的独立判据**：全真实链路（不打桩、不改源码）：真浏览器点「插件 → pi → 下发到全部机器」，JS confirm 用 CDP `Page.javascriptDialogOpening` 真实接受（比 stub 更贴近用户）；后端侧用 curl 独立取证返回信封。
- **实机**（`r2-c4plugin6.mjs`）：点击后 2s `#plugin-error` visible，`textContent === "partial"`，`role=alert` 红色告警栏（截图 CC4-b-partial-visible.png 可见）。curl 同时刻 `POST /api/sync?direction=dispatch&confirm=true` → HTTP 422 `{"ok":false,"status":"partial","errors":null,"agents":[2 台失败]}` ——信封里 errors=null，`partial` 是裸状态码。
- **Root cause 修正（与猎人不同）**：猎人指 index.html:3786（`result.status` 分支）。但该响应是 **HTTP 422**，`api()`（:1024）对非 2xx 会 **throw**，根本到不了 3786 的 `if (result.ok)` 分支；真实路径是 api() 的写操作失败信封兜底（:1029-1034）：`e.message = errors.join || data.status`（errors 为 null → `"partial"`）→ 抛出 → `catch (error) → pluginErrorMessage(error)`（:3790）→ setPluginError("partial")。即根因在 **api() 的信封合成把 `data.status` 当人话文案**（:1033），插件分发 handler 只是受害者。修复应在 :1033 或后端 sync.go:262 给 partial 附人话 errors。
- **严重度**：中（英文内部 token 出现在面向用户的红色告警，无任何解释/下一步指引）。

---

## 汇总表

| 编号 | 猎人判 | 我的判定 | 我的判据要点 | 关键证据 |
|---|---|---|---|---|
| B-C1 | 高 | **true（高）** | 同页三处复制差分 + onerror 独立通道 + 真实鼠标 | 3 轮稳定；copyCommand/copyAgentId 走降级，copyJoin 抛 |
| B-C2 | 高 | **true（高）** | kill -STOP 真实慢后端 + CDP 层全真实事件 | 假取消/孤儿回调/写穿 desc 全部实机复现 |
| B-C3 | 中 | **true（中）** | 元素身份标记 + 页面内焦点采样 | 2 实验 1 对照；对照期标记存活 |
| B-C4 | 中 | **true（中）** | CDP 网络层阻断 + 解封后同源探活 | gate 无错误提示；服务端会话仍 200 |
| B-C5 | 低 | **true（低）** | CDP 协议层请求计数 | 1→1、3→3（含真实鼠标轮）；无任何反馈 |
| B-C6 | 中(未证实) | **downgraded** | Go 采集/序列化层探针（uint64 无 omitempty） | used 恒在且为 0；NaN 前提前端不可达 |
| B-C7 | 低 | **true（中，上调）** | CDP 阻断 + 对照轮区分「等待静默」 | 6 次请求全拒、0 错误词、承诺文案相反 |
| C-C4 | 中 | **true（root-cause-elsewhere）** | 全真实 UI + curl 双侧取证 | `#plugin-error="partial"`；根因在 api():1033 信封合成 |

## 环境恢复声明
- box-b agent 进程（pid 1005）已 `kill -CONT`，处 S 运行态；实验中另注册的 b6675d68ddf7-8e7b/6dd5 为共享 hub 留下的正常状态，未做破坏性操作。
- 仓库未改一行源码（`git status` 仅 `.qa-hunt/` 产物）；Go 探针文件跑完即删。
- hub 状态与他人共享，全程未断言全局唯一性。
