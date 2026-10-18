# 缺陷狩猎 · 角度 B：网页 UI 交互状态机 / 静默失败 / 假进度

仓库 /root/code/homer HEAD=010521a。真实浏览器（browserless chromium, CDP）操作控制台。
hub http://hub:7760 · 账号 qa-admin-password · 专属 box homer-qa-box-b（hub 状态与其他 hunter 共享，
下列结论均不依赖全局唯一性）。

## 角度
负责：真实浏览器里的**用户可感知交互缺陷**——点了没反应、状态机卡死/串台、错误静默、
进度假象、轮询造成状态丢失。
不做：后端 API 语义、纯静态代码审计（scout 已做）、已由 e2e 断言过的设计意图（空中心 OK disabled、
登录 2s 退避、离线机器按钮 disabled 等）。

## 基建
- `timeout 60 curl http://127.0.0.1:17760/api/health` → 200 `{"ok":true,...}` **PASS**
- `timeout 60 curl http://127.0.0.1:3000/json/version` → Chrome/156 根 ws **PASS**
- harness：`.qa-hunt/lib2.mjs`（CDP 连接、注入 prompt/confirm stub + window error 收集）、
  `.qa-hunt/t*.mjs` 各场景脚本。**未改动被测源码一行**。
- 登录门 → workspace 隐藏类翻转，浏览器实测 **PASS**（`#workspace` 非 hidden、5 张机器卡片渲染）。

## Candidates

### C1 「接入新机器」的复制按钮在非安全上下文完全失效：抛 TypeError，无任何 toast，也不显示降级提示
- 疑似位置: `internal/web/static/index.html:4784-4793`（`copyJoin`）、调用点 `:4795`
- **detector**: `typeof navigator.clipboard` 在页面上下文返回 `"undefined"`（`window.isSecureContext === false`，
  origin `http://hub:7760`）。点击 `#btn-copy` 后判据为
  `document.querySelector('#btn-copy').textContent === '复制'`（未变成「已复制 ✓」）
  **且** `document.querySelector('#copy-fallback').classList.contains('hidden') === true`（降级提示仍隐藏）
  **且** `window.__err` 出现 `"Uncaught TypeError: Cannot read properties of undefined (reading 'writeText')"`。
  - 已确认该符号在目标组件外不出现：`grep -n "copy-fallback" internal/web/static/index.html` → 仅 4 处
    （HTML 定义 :914、openJoin 重置、copyJoin 的 .catch、.catch 内部），`#copy-fallback` 只由 copyJoin 的
    catch 分支控制 → 用它判「降级提示是否出现」无歧义。
- 前置状态探测: 点击前先 `waitFor(#join-cmd.value.indexOf('hr_')>=0)`，实测值
  `curl -fsSL http://hub:7760/install.sh | sh -s -- --token hr_be18f7f03b...`——确认对话框已打开、命令已取到。
- 证据: `.qa-hunt/shots-B/C1-copy-join-broken.png`、`.qa-hunt/shots-B/13-copyjoin-broken.png`
- 复现步骤:
  1. `cd /root/code/homer && timeout 60 node .qa-hunt/t16.mjs`
  2. 或浏览器打开 `http://hub:7760/`（非 https）→ 登录 → 点右上「接入命令」→ 点命令框右侧「复制」。
- 实机输出: `C1 detector navigator.clipboard: undefined isSecureContext: false origin: http://hub:7760` /
  `C1 __err: ["Uncaught TypeError: Cannot read properties of undefined (reading 'writeText')"] btnText: 复制 fallbackHidden: true`
- 下游后果: 用户以为「复制了」去粘贴 —— 剪贴板里是空的（或上一次的内容）。同页另外两处复制
  （`copyCommand` :2164、`copyAgentId` :2190）**都有 `navigator.clipboard &&` 守卫**并回退到 window.prompt，
  唯独 copyJoin 没有 → 三处实现不一致（scout §3.1 第 14 条怀疑点，**证实**）。
- 为什么用户真会撞上：README 明写 QA/内网场景就是 `http://` 访问（非安全上下文），
  接入新机器的第一个动作就是复制命令。
- 我无法排除的干扰: 无（纯同步 TypeError，与 CDP 时序无关；`isSecureContext=false` 是浏览器规则不是 harness 产物）。
- 建议修复: 与另两处对齐——`if (!navigator.clipboard) { input.focus(); input.select(); $("copy-fallback").classList.remove("hidden"); return; }`。

### C2 下发「取消」是假的：取消按钮 disabled，真正的退出通道 Escape 会把在途请求变成孤儿，之后回调写进**另一个**对话框
- 疑似位置: `internal/web/static/index.html:3479-3493`（`beginDispatchWait` 里 `cancel.disabled = true`）、
  `:3531-3559`（`finishDispatchInDialog` 无条件写 `app.pendingAction = {kind:"dispatch-done"}` 与 `#confirm-desc`）、
  `dlg-confirm` close 事件 `:3167-3178`（只清 pendingAction，不取消在途 fetch）
- **detector**（三态判据，全部用可见 DOM / JS 值）:
  1. 进入下发中：`#btn-confirm-ok`.textContent 匹配 `/^正在下发… \d+ 秒$/` **且** `#btn-confirm-cancel`.disabled === true
     **且** `#btn-confirm-back`.disabled === true（进得去出不来）。
  2. 假取消：对 `#btn-confirm-cancel` 用 `Input.dispatchMouseEvent` 派发**真实鼠标点击**后，
     `#dlg-confirm.open === true` 且 `#btn-confirm-ok`.textContent 仍匹配 `/^正在下发… \d+ 秒$/` → 取消无效。
  3. 串台：按真实 Escape 关掉对话框（`#dlg-confirm.open === false`，`app.pendingAction === null`），
     随后打开**另一台机器**的「处理冲突」对话框（`#confirm-title` 为 `处理冲突 · b6675d68ddf7（b6675d68ddf7-7920）`），
     再让后端请求返回 → 判据 `document.querySelector('#confirm-desc').textContent === '已下发到「54d0c3373428」。'`
     且 `#confirm-title` 仍是**处理冲突**标题，且 `app.pendingAction.kind === "dispatch-done"`。
  - 已确认 `#confirm-desc` 的「已下发到「X」。」文案只由 `finishDispatchInDialog`（:3553-3556）产出：
    `grep -n '已下发到' internal/web/static/index.html` → 仅 :3554-3555 与 toast 那处；用
    `#confirm-title` 保持「处理冲突」而 `#confirm-desc` 变成「已下发到…」两条不一致文案判定串台，无歧义。
- 前置状态探测: 进入测试前先 `waitFor(#confirm-choices.children.length>0)` 与
  `app.pendingAction.kind === "dispatch-one"`；实测 `{"open":true,"ok":"正在下发… 1 秒","cancel":true,"back":true}`
  ——确认处于「下发中」状态才操作。
- 证据: `.qa-hunt/shots-B/C6-hung-dispatch.png`（下发中，取消灰）、
  `.qa-hunt/shots-B/C6-cancel-click-noop.png`（点了取消没反应）、
  `.qa-hunt/shots-B/C7-second-dialog-before.png`（另一个对话框正常）、
  `.qa-hunt/shots-B/C7-second-dialog-AFTER-orphan.png`（**截图可见标题「处理冲突 · b6675d68ddf7（…7920）」，正文却是「已下发到「54d0c3373428」。」**，按钮变成「完成」）
- 复现步骤:
  1. `cd /root/code/homer && timeout 120 node .qa-hunt/t20.mjs`
  2. 或浏览器：任一机器点「下发」→ 勾一个适配器 → 「确认下发」（后端慢时）→ 观察「取消」是灰的
     → 按 Esc 关掉 → 立刻对另一台机器点「处理冲突」→ 等原请求返回，看它的正文被改成「已下发到「上一台」」。
- 下游后果: 用户明确按了取消/关掉了窗口，操作仍然执行（无撤销），并且把结果文案写进了**不相干的对话框**；
  若此时用户以为看的是裁决窗口而点「完成」，会关掉一个已被污染的状态机。
- 时序前提: 需要后端 `POST /api/agents/<id>/pull?confirm=true` 慢于用户按 Esc 的瞬间。
  本机实测用 harness 挂住 fetch 复现；真实环境里机器离线超时（后端 agent-timeout 504）/网络抖动即满足。
- 我无法排除的干扰: 「在途请求变孤儿」这一半依赖我把 /pull 挂住（harness 造的慢后端）。
  但**「取消按钮 disabled + Escape 是唯一出口」这一半是纯静态必然**（:3489 `cancel.disabled = true`
  且 `stopDispatchWait` 只在 close/结束后复位），不依赖 harness。串台那半是挂起后的必然结果。
- 建议修复: close 事件里记录 `app.scopeStreamController`，把在途 pull 用 AbortController 取消，
  或在 `finishDispatchInDialog` 开头加 `if (!app.pendingAction || app.pendingAction.kind !== "dispatch-one") return;`
  （用它自己的 action 令牌校验，避免把结果写进别人的对话框）。

### C3 30s 自动刷新重建机器卡片：展开的「更多」菜单被强制收起、焦点被抢走（真实轮询复现，非手动 refresh）
- 疑似位置: `internal/web/static/index.html:1854`（`renderAgents` 开头 `clear(list)` 全量重建）、
  `:5271`（`setInterval(..., 30000)`）
- **detector**: 操作前 `document.querySelector('.agent-card details.agent-more').open === true`
  **且** `document.activeElement.textContent.trim() === '详情'`；
  等 footer `#refreshed-at` 文本变化（= 真实 30s 轮询真的跑过）后，
  判据 `document.querySelector('.agent-card details.agent-more').open === false`
  **且** `document.activeElement.tagName === 'BODY'`。
  - 已确认该符号在目标组件外不出现：`grep -n 'details.agent-more' internal/web/static/index.html` → 仅
    `makeAgentMoreMenu`（:1527 创建）、:1540 兄弟菜单互斥、:1548 全局 pointerdown 关闭；
    `.agent-card details.agent-more` 只在机器卡片内出现，且我选定第一张卡片的菜单，
    兄弟菜单互斥逻辑不可能关掉它自己 → 「open 由 true 变 false」只能来自 DOM 被重建。
  - 额外判据 `document.elementFromPoint(oldX, oldY)` 返回 `BUTTON.small-btn|查看`：轮询后在旧坐标
    点上原来的「吊销」，命中的是**另一个按钮**（证明卡片整体位移/重建，不只是收起）。
- 前置状态探测: 点击前 `window.__snap = {open:true, focused:'详情'}`；轮询前记录
  `#refreshed-at` 原文本（`06:19:02 已刷新`），轮询后变 `06:19:31 已刷新` 才判定轮询已发生。
- 证据: `.qa-hunt/shots-B/R1-menu-open-before-poll.png`（菜单开着、「详情」聚焦）、
  `.qa-hunt/shots-B/R2-menu-closed-after-REAL-poll.png`（轮询后菜单收起）、
  `.qa-hunt/shots-B/22-menu-open.png` / `23-menu-closed-after-poll.png`（同现象，手动 refresh 版）
- 复现步骤:
  1. `cd /root/code/homer && timeout 150 node .qa-hunt/t24.mjs`（等真实 30s 轮询，脚本会自动等到 footer 变化）
  2. 或浏览器：点开任一机器卡片的「更多」（不要动别的，等最多 30 秒）→ 菜单自己关上；
     点开「更多」后用 Tab 聚焦「详情」→ 30s 后焦点被丢回 body，键盘用户失去位置。
- 实机输出: `open menu snapshot: {"open":true,"focused":"详情"}` / `poll fired at i=19: 06:19:31 已刷新` /
  `after REAL poll -> menu open: false | activeElement: BODY|homer`
  （提示：`app.openDiffs` 注释声称「轮询不重置」，但 **diff 抽屉的保留是巧合**——它不在 renderAgents
  重建范围内；机器卡片上的菜单没有这个运气。）
- 下游后果: 用户正在挑「更多」里的动作（吊销/移除/登录公钥）时菜单凭空消失，必须重点；
  展开过程中若恰逢轮询，点击落到重建后的**另一个按钮**上（我实测旧坐标命中「查看」而不是「吊销」）。
- 我无法排除的干扰: `details.agent-more` 也可能被全局 pointerdown 关闭 —— 但本测试全程只发
  键盘/无指针事件，且等待期间没有鼠标动作，故排除。
- 建议修复: renderAgents 前记录 `details.agent-more[open]` 的 `data-agent-id` 与 `document.activeElement`
  的稳定标识（如 `data-action`），重建后恢复 open 与焦点；或按 agentId 做增量 diff 而非 `clear(list)`。

### C4 「退出」请求失败时照样跳回登录门：用户以为已登出，服务端会话仍然有效
- 疑似位置: `internal/web/static/index.html:5173-5178`
  （`try { await api("/api/auth/logout", {method:"POST"}) } catch (_) {}` 后无条件 `showGate("login")`）
- **detector**: 拦截 `/api/auth/logout` 使其 reject（`window.fetch` 打桩计数 `window.__logoutCalls`），
  点击 `#btn-logout` 后判据
  `document.querySelector('#workspace').classList.contains('hidden') === true`
  **且** `!document.querySelector('#gate-login').classList.contains('hidden') === true`
  **且** `document.querySelector('#err-login').textContent === ""`（无任何错误提示）
  **且** `fetch('/api/console',{credentials:'same-origin'}).then(r=>r.status) === 200`（服务端会话仍有效）。
  - 已确认该符号在目标组件外不出现：`#err-login` 仅由登录表单逻辑写（`grep -n 'err-login'` 命中登录分支），
    用它判「是否提示了错误」无歧义。
- 前置状态探测: 操作前确认处于已登录态（`#workspace` 非 hidden、`#btn-logout` 可见可点）。
- 证据: `.qa-hunt/shots-B/C4-logout-fake.png`（已回到登录门、无错误）、`.qa-hunt/shots-B/14-logout-failed.png`
- 复现步骤:
  1. `cd /root/code/homer && timeout 120 node .qa-hunt/t16.mjs`
  2. 或浏览器：断网（DevTools offline）→ 点右上「退出」→ 页面回到登录门，没有任何错误；
     恢复网络后**刷新页面即直接回到已登录工作区**（Cookie 未被清除）。
- 实机输出: `C4 logoutCalls: 1 workspaceHidden: true gateLoginShown: true errText: ""` /
  `C4 server session /api/console status: 200`
- 下游后果: 用户以为自己退出了，把机器留在共享/公共环境就离开 —— 别人刷新页面（或按后退）就能继续操作。
  这与「退出」按钮的安全语义直接冲突。
- 我无法排除的干扰: 我用 fetch 打桩制造失败。真实触发条件是网络不可达/hub 重启瞬间。属于必要条件，
  但「失败后 UI 与服务端状态不一致」是代码路径的必然（catch 全吞 + 无条件 showGate）。
- 建议修复: `catch (e) { toast("退出失败：" + e.message); return; }` 或提示「服务端未确认退出，请重试」。

### C5 `#btn-refresh` 无防抖无反馈：连点产生 3 个并发 `/api/agents`，无 loading 态、无 disabled、无 aria-busy
- 疑似位置: `internal/web/static/index.html:5181`（`$("btn-refresh").addEventListener("click", refresh)`）、
  `:5088-5098`（refresh 无任何 loading 门）
- **detector**: 点击后立即（t+0.3s）判据
  `document.querySelector('#busy').classList.contains('hidden') === true`
  **且** `document.querySelector('#btn-refresh').disabled === false`
  **且** `document.querySelector('#btn-refresh').getAttribute('aria-busy') === null`
  **且** `document.querySelector('#refreshed-at').textContent` 未变化（仍是旧时间戳）；
  并发判据：把 `/api/agents` 延迟 3s，连点 3 次后 `window.__inflight === 3`。
  - 已确认该符号在目标组件外不出现：`#busy` 只由 `setBusy/clearBusy`（:1057,1061）与
    `finishConfirmButton` 操作，而 refresh 路径**不调用**它们（`grep -n 'setBusy'` 命中处不含 refresh）
    → 用 `#busy` 判「刷新有没有 loading 反馈」成立。
- 前置状态探测: 记录刷新前 `#refreshed-at` 文本（`06:13:14 已刷新`），确认点击后有变化（`06:13:17 已刷新`）
  → 证明 refresh 确实执行了，只是**执行期间没有任何可见反馈**。
- 证据: `.qa-hunt/shots-B/C5-refresh-no-feedback.png`
- 复现步骤: `cd /root/code/homer && timeout 90 node .qa-hunt/t18.mjs`
  （或浏览器：DevTools 设 Slow 3G → 连点「刷新」3 次 → 页面无任何变化，Network 面板 3 个 /api/agents 并发）
- 实机输出: `t+0.3s busyVisible: false btnDisabled: false btnText: 刷新 ariaBusy: null refreshed-at: 06:13:14 已刷新` /
  `C5 concurrent /api/agents in flight: 3`
- 下游后果: 用户点「刷新」后页面数秒不动（无转圈、无按钮态），会误判「卡住了」并连点，
  造成并发请求；响应乱序时后到的旧响应覆盖新状态（scout §9 已指出该风险，此处证实并发确实产生）。
- 我无法排除的干扰: 「响应乱序导致显示旧数据」我没能构造出确定性复现（需要构造两个响应体对调，
  属 harness 可控但对真实用户不可达），所以我只报**已被实测的**「无反馈 + 并发」这半段。
- 建议修复: refresh 入口加 `if (app.refreshing) return;` 与按钮 disabled/aria-busy，或直接 `debounce`。

### C6 机器卡片把 `NaN%` 渲染给用户（内存百分比），来源是后端允许的 `used > total` 归一化后仍 NaN
- 疑似位置: `internal/web/static/index.html:1752-1754`（`memText`）与 `:1793`（`hostSummary` 的内存分支）；
  数据侧 `internal/hub/host.go:150-159`（`normalizeMemory` 只做 `used=min(used,total)`，total 保留 0 以外的值）
- **detector**: 在页面上下文用真实渲染函数喂「服务端允许的字段组合」（`total` 有值而 `used` 缺失），判据
  `document.querySelector('#agent-list').textContent.includes('NaN') === true`；
  同时 `hostSummary(a) === "CPU 8% · 内存 NaN% · 磁盘 50%"`。
  - 已确认该符号在目标组件外不出现：`grep -rn 'NaN%' internal/web/static/index.html` 无字面量命中
    （`NaN` 只在 typeof 守卫里出现），故 `#agent-list` 文本含 `NaN` 只能是拼接产物，不会与其它文案冲突。
- 前置状态探测: 先 dump 真实 `/api/agents`（Bearer qa-hub-token）确认线上字段形状：
  `"memory":{"total":16639692800,"used":5758283776}`、`"load":{"one":0.42,"five":0.89,"fifteen":0.78}`、
  `"disks":[{"mount":"/","total":...,"used":...}]` —— 基线正常；再注入缺字段的变体观察渲染。
- 证据: `.qa-hunt/shots-B/21-nan-card.png`、`.qa-hunt/shots-B/12-nan.png`
- 复现步骤:
  1. `cd /root/code/homer && timeout 120 node .qa-hunt/t13.mjs`
  2. 最小纯函数复现（浏览器控制台）：
     `hostSummary({host:{cpu:{usage:8},memory:{total:100},disks:[{used:5,total:10}]}})` → `"CPU 8% · 内存 NaN% · 磁盘 50%"`
- 实机输出: `card text: 54d0c3373428 ... CPU 8% · 内存 NaN% · 磁盘 50% ...` /
  `C6 contains NaN: true undefined: false`
- 下游后果: 机器卡片摘要直接显示 `内存 NaN%`，用户看到的是「系统信息坏了」而非「这项没读到」。
  `MemTotal` 有值而 `MemAvailable/MemFree` 都读不到时（受限容器、cgroup 环境）就会出现
  `used = total - (0+0+0)`；`normalizeMemory` 只在 `Total == 0` 时返回 nil，挡不住 total 有效但 used 缺失的输入。
- 我无法排除的干扰: 我用 JS 注入构造了 `memory:{total:100}`（缺 used）。**线上 5 台机器当前都同时有
  total/used**（见前置探测），所以我**未能证明真实 agent 会产出这种组合** —— 这一条按「未证实可触发」
  标注，交给 repro 判断是否够格。前端本身的缺陷（缺 `mem.used` 守卫，而 `memText`/`hostSummary`
  对 `total` 有守卫却在 `used` 上裸算，且同一份 `hostSummary` 对 `cpu.usage` 有 `!= null` 守卫）是确定的。
- 建议修复: `hostSummary` 内存分支加 `host.memory.used != null`；`memText` 用 `fmtBytes`（它已有 isFinite 守卫）。

### C7 「接入新机器」的轮询失败被完全吞掉（`catch (_) {}`）：网络断了也没有任何提示，用户一直等一个不会自动关闭的窗口
- 疑似位置: `internal/web/static/index.html:4752-4766`（`watchJoinCode` 内 `catch (_) {}`）
- **detector**: 拦截 `/api/auth/join/status` 使其 reject 并计数 `window.__joinCalls`，
  打开接入对话框后等 7 秒，判据
  `window.__joinCalls >= 3`（轮询确实在跑）**且** `document.querySelector('#dlg-join').open === true`
  **且** `document.querySelector('#dlg-join').textContent` 不含任何错误词（无「失败」「错误」「重试」）
  **且** `document.querySelector('#toast').textContent === '已登录'`（是上一次登录的残留 toast，没有新提示）
  **且** `window.__err.length === 0`（连未捕获异常都没有 —— 因为是主动吞掉的）。
  - 已确认该符号在目标组件外不出现：`#dlg-join` 是唯一 join 对话框（`grep -n 'dlg-join'` 命中 openJoin/
    watchJoinCode/close 三处，均为同一元素），用它的文本判「有没有错误提示」无歧义；
    toast 文案先记录基线值再比较，避免把残留 toast 误读成新提示。
- 前置状态探测: 轮询启动前先 `waitFor(#join-cmd.value.indexOf('hr_')>=0)`，实测
  `curl -fsSL http://hub:7760/install.sh | sh -s -- --token hr_...`，确认已进入「等待接入」状态。
- 证据: `.qa-hunt/shots-B/19-join-poll-silent.png`
- 复现步骤: `cd /root/code/homer && timeout 120 node .qa-hunt/t11.mjs`
  （或浏览器：打开「接入命令」→ DevTools 设 Offline，或把 hub 停掉 → 窗口永远显示「机器用这个接入码
  连上之后，窗口会自己关上」，没有任何「连不上服务器」的提示）
- 实机输出: `J1 join/status calls in 7s: 3` / `J1 dlg open: true` /
  `J1 any error text in dialog: 接入新机器 ... 复制 ... 机器用这个接入码连上之后，窗口会自己关上。 完成`（无错误词）
- 下游后果: 用户以为「还在等机器连上」，实际是前端根本问不到服务器。窗口文案明确承诺「会自己关上」
  （:4791 注释同义），与事实相反。
- 我无法排除的干扰: 我用 fetch 打桩制造失败。真实触发是 hub 重启/断网。属必要条件，但 `catch (_) {}`
  是代码里的确定路径。
- 建议修复: 连续 N 次失败后在对话框内显示「读取接入状态失败，正在重试…」并保留轮询（区分「等机器」与「问不到服务器」）。

## 证伪/未构成候选（已核对，写给下游省时间）
- **scout 怀疑点 1「copyCommand 复制成功也弹 prompt」= 误读**。`grep -n 'navigator.clipboard.writeText'` →
  :2165 是 `.then(() => toast("已复制，…"), ask)`，**第二个参数是 rejection 处理**，不是成功回调。
  实机验证：`copyCommand('curl x')` 在非安全上下文走 else 分支弹 prompt（正确）；`copyAgentId` 走
  `.then(flashOk, fail)` 并 toast「复制失败，请手动复制」（正确）。两者行为符合意图，**不报**。
- **`#btn-confirm-ok` 全不选时点是静默 no-op，但按钮是 disabled 的**：实测
  `await ev("document.querySelector('#btn-confirm-ok').disabled") === true`，真点无效。
  早退 `return`（:3209）只是防御性代码，用户碰不到 → **不报**（与 e2e 断言的「OK 保持 disabled」一致）。
- **`#keys-fold-hint`（:843）的确无任何 JS 写入**：`grep -c 'keys-fold-hint'` = 1（仅定义处），
  文案永久是「已收下的密钥 · 点开管理」。但该文案**语义上始终成立**（密钥区就是「已收下的密钥」），
  不构成用户可感知错误 → **不报**，仅登记为「静态未使用元素」。
- **`openMachineView` 的 adapters 请求失败只降级一行小字**（scout §3.1-4）：实机用
  `Network.setBlockedURLs(['*adapters*'])` 复现，`#view-body` 末尾出现「暂时无法读取这台机器的适配器停用状态。」，
  抽屉其余内容完整、无崩溃、无 NaN。这是**有意的降级设计**（注释明说兼容旧 hub），
  虽不显眼但用户能读到 → 按现有证据**不报**（若要报需先证明「停用按钮消失且无任何解释」会让用户误操作，
  我这里 override buttons 数 0 但降级文案在，故证据不足）。

## 未覆盖
- 「响应乱序导致 UI 显示较旧数据」：需要让两个 `/api/console` 响应体对调才能确定复现，
  属 harness 可控但真实用户不可达，未构造。
- `#dlg-confirm` 五 kind 的**全部交叉分支**（收取/下发/裁决/密钥/停用 × choose/unlock/wait/done）：
  实测覆盖了 dispatch-one×choose/wait/done、agent-adapter-override×choose、resolve-record×choose
  与 resolve 对话框的勾选联动；**未覆盖** ssh-key kind、collect×unlock、resolve×unlock（需绑定密钥的机器，
  本环境无已加密密钥可用）、以及 openScopeDialog 的 `.catch` 分支（需 `/api/sync/choices` 失败）。
- 密钥相关 UI（`#dlg-key` 表单、加密/解锁口令步）未实机走通——hub 上 `/api/keys` 返回
  `{"ok":true,"status":"listed"}` 无已存密钥，需要先在机器上造密钥，超出时间预算。
- 应用升级/插件安装的进度假象（`upgradeTools` 的 `setBusy` 文案、「正在升级 …（可能要几分钟）」）：
  本环境 pi stub 永远 0 版本、无可安装官方插件，未触发。
- 未跑 Go e2e（`tests/e2e/*_ui_test.go`）：需要容器网络与 chromedp 构建，时间预算内改跑 CDP 实机。

## 环境
- 仓库：/root/code/homer HEAD `010521a`
- hub：http://127.0.0.1:17760（浏览器侧 http://hub:7760）· token `qa-hub-token` · 密码 `qa-admin-password`
- 浏览器：browserless chromium `Chrome/156.0.8078.4`，CDP `ws://127.0.0.1:3000`
  （必须先 `Target.attachToTarget(flatten:true)` 拿 sessionId）
- 视口：`Emulation.setDeviceMetricsOverride 1280x900`（截图为 1280x900 或默认）
- node v24.13.1；harness：`.qa-hunt/lib2.mjs` + `.qa-hunt/t2*.mjs`/`t1*.mjs`（每个脚本
  `setTimeout(...process.exit(9), 50_000)` 自保护）；所有 bash 命令 `timeout 60` 起
- 截图目录：`.qa-hunt/shots-B/`（34 张）
- 未改动被测源码（`git status` 仅新增 `.qa-hunt/` 下产物）
- 注意：hub 状态与其他 hunter 共享，测试期间机器数在 1→5 之间变化、多台出现过 stale/conflict；
  上述结论均已避免依赖全局机器数量或唯一性。
