# 冻结面侦察 · UI/网页控制台

仓库 /root/code/homer，HEAD=010521a。纯只读侦察。所有行号基于 HEAD 磁盘内容。

环境（已实测）：curl http://127.0.0.1:17760/api/health → 200 {"ok":true,"status":"healthy","version":"dev"}；
/api/auth/status → {"configured":true,"ok":true,"setupCodeRequired":false}；
/api/agents（Bearer qa-hub-token）→ 1 台机器 e363efdcf085（drift.error=未找到 homer 配置，tools=[pi 读不出版本]）。

---

## 0. 一页速览

- 整个控制台是一个文件：internal/web/static/index.html（5275 行，含内联 style 与全部 JS）。
  没有构建产物、没有外链 JS/CSS（internal/web/static.go 只有 embed 声明，8 行）。
- 没有路由：无 hash 路由 / History API。页面只有「门（gate）↔ 工作区（workspace）」两态，
  区块用 class="hidden" 显隐，不是 SPA 路由。
- 服务端路由在 internal/web/handlers.go:32-230 的手写 switch（非 http.ServeMux）。
- /api/health 与 /api/auth/* 免鉴权（handlers.go:48-58）；其余 /api/* 走 requireAuth（handlers.go:83）。

---

## 1. 区块 / 元素 → 触发函数 → 后端端点 对照表

### 1.1 静态骨架（index.html:760-960）

| 元素 | 位置 | 显示/隐藏条件 |
|---|---|---|
| header.topbar | 780-788 | 常驻；4 个按钮由 gate/workspace 切换隐藏（1075 / 1085） |
| #btn-plugins 插件 | 784 | → openPluginManager() 3712 → dlg-plugins；数据来自 app.plugins（refresh 拉的 /api/plugins） |
| #btn-join-top 接入命令 | 785 | → openJoin() 4769 → GET /api/auth/join；loadAgents() 在 code==="agents-disabled" 时隐藏它（2259） |
| #btn-refresh 刷新 | 786 | → refresh() 5088（无 loading 态、无 disabled 防抖，可连点并发） |
| #btn-logout 退出 | 787 | → POST /api/auth/logout，catch 全吞（5174），直接 showGate("login") |
| #theme-select 外观 | 768-773 | localStorage homer-console-theme（5191-5215），纯前端，无端点 |
| #gate-setup | 791 | routeUnauthed() 在 !gate.configured 时显示（5228-5243） |
| #gate-login | 804 | 已配置但未认证 / 401 / status 不可达（"无法连接服务："+e.message，5236） |
| #workspace（main） | 821 | 登录成功后 showWorkspace() 1082 |
| #hero | 824 | renderHero() 1568；状态机见 §2 |
| #alert-slot | 827 | showAlert() 4055 / clearAlert() 4081；页面级 inline banner（role=alert + 关闭 X） |
| #sec-fleet-map | 828 | renderFleetMap() 1300-1311：!c.published \|\| !c.machines.length → hidden |
| #sec-adapters | 834 | renderAdaptersOverview() 1410-1421：同上条件 |
| #sec-keys | 839 | 常驻不隐藏（无任何 hidden 逻辑）；details 折叠，body=#key-body |
| #sec-agents | 852 | 常驻；#agent-list 由 renderAgents() 1854 填充 |
| footer #refreshed-at | 863 | stampRefresh(failed) 5082-5086 |
| #toast | 868 | toast() 1053，2.6s 自动消失 |
| #busy | 869-871 | setBusy() 1057 / clearBusy() 1061 |

### 1.2 模态框（dialog）

| dialog id | 位置 | 打开者 | 端点 |
|---|---|---|---|
| #dlg-confirm（收取/下发/裁决/密钥/停用 共用一个） | 874 | openScopeDialog() 3039、confirmSSHKey() 3120、confirmAgentAdapterOverride() 4602 | /api/sync/choices、/api/sync?direction=collect、/api/agents/<id>/pull?confirm=true、/api/resolve/record、/api/resolve/clear、/api/agents/<id>/adapters/<aid>、/api/agents/<id>/ssh-key |
| #dlg-view（查看 / 存储内容 / 主机详情） | 893 | openMachineView() 4641、openStorageView() 4085、openHostDetail() 1828 | POST /api/agents/<id>/status、GET /api/agents/<id>/adapters、GET /api/storage、GET /api/storage/file |
| #dlg-key（密钥表单） | 900 | openKeyForm() 5000、openMachineKeys() 5049、encryptCredential() 3012、encryptDestination() 4312 | GET/POST /api/keys、/api/keys/encrypt、/unlock、/password、POST /api/keys/op(browse)、POST /api/agents/<id>/keys |
| #dlg-join（接入命令） | 911 | openJoin() 4769 | GET /api/auth/join；轮询 GET /api/auth/join/status?code= |
| #dlg-plugins（插件管理） | 924 | openPluginManager() 3712 | /api/plugins/install、/api/plugins/uninstall、/api/sync?direction=dispatch&confirm=true、/api/agents/<id>/ssh-key |
| #dlg-plugin-action | 953 | openPluginAction() 3840 | POST /api/agents/<id>/ssh-key |

### 1.3 机器卡片动作（#agent-list，renderAgents 1854-2055）

| 按钮 | 位置 | 条件 | 函数 → 端点 |
|---|---|---|---|
| .aid 复制 agentId | 1891-1909 | a.agentId 存在 | copyAgentId() 2171 → 无端点（clipboard） |
| 查看 | 1931 | disabled = a.stale | openMachineView() → POST /api/agents/<id>/status |
| 处理冲突 | 1953 | d.conflicts>0 && !a.stale（渐进披露 #49） | confirmResolve() 3145 → openScopeDialog(direction:"resolve") |
| 收取 | 1966 | d.push>0 && !a.stale | confirmCollect() 3100 → /api/sync/choices?direction=collect&stream=1 |
| 下发 | 1976 | (d.pull>0 \|\| waitingFirst) && !a.stale | confirmDispatch() 3110 |
| 更多▸详情 | 1988 | — | openHostDetail() 1828（纯本地，复用缓存 host，无请求） |
| 更多▸密钥 | 1996 | disabled = a.stale | openMachineKeys() 5049 |
| 更多▸更新程序 | 2003 | a.outdated 才渲染；disabled=a.stale | upgradeAgent() 2070 → POST /api/agents/<id>/upgrade |
| 更多▸登录公钥 | 2021 | disabled=a.stale | confirmSSHKey() 3120 → POST /api/agents/<id>/ssh-key |
| 更多▸吊销 | 2027 | window.confirm | POST /api/agents/revoke |
| 更多▸移除 | 2042 | window.confirm | POST /api/agents/remove |
| ＋接入新机器（空态+页脚） | 1869 / 2046 | — | openJoin() |
| 全部更新 | 2050 | outdated && !stale 非空 | upgradeAgents() 2075 |
| 全部升级应用 | 2059 | outdatedTools().length | upgradeBehindTools() 2204 → POST /api/agents/<id>/tool-upgrade |
| 应用 chip 升级 | 2121-2129 | t.outdated && t.upgradable | upgradeTool() 2196 |

### 1.4 配置差异 / 日志 / 任务 / 设置

- 配置差异：无独立 section。抽屉式内嵌：
  - makeScopeDiffControls() 4388 → POST /api/agents/<id>/diff?adapter=&category=（收取/下发对话框内）
  - toggleConflictDiff() 4466 → 同上 + &path=（冲突行对照）
  - toggleStoragePreview() 4327 → GET /api/storage/file?adapter=&path=
  - renderUnifiedDiff() 4376；CONFLICT_MAX_LINES 截断（4535-4536）
- 日志：无日志区块。footer 只有 #refreshed-at 时间戳。
- 任务：无任务队列 UI。长任务只有 #busy 遮罩 + beginDispatchWait() 对话框内计时器。
- 设置：无设置页/区块。仅 #theme-select 外观（localStorage）。

---

## 2. 前端状态机

### 2.1 启动 / 门

boot() 5255
 ├─ loadHeaderVersion() 5244 → GET /api/health（无需鉴权；失败静默，#ver 留空）
 ├─ routeUnauthed() 5225 → GET /api/auth/status
 │    ├─ !configured → showGate("setup") → return "setup"（不再 refresh）
 │    ├─ fetch 抛错 → showGate("login","无法连接服务："+e.message) → return "error"
 │    └─ configured → showGate("login", loginNote) → return "ok"
 └─ ok → await refresh()

关键：没有 "authed" 标志位。认证状态完全靠刷新时是否 401 反推（5107-5111）。
showGate() 1071 隐藏 workspace + 4 个 header 按钮；showWorkspace() 1082 反之。

### 2.2 轮询 / 定时器清单（没有 WebSocket，没有 EventSource）

| 位置 | 周期 | 拉什么 | 停止条件 |
|---|---|---|---|
| 5271 setInterval | 30s | refresh() → 并发 Promise.all([/api/console, /api/agents, /api/plugins])（5091-5093） | 永不清除；!document.hidden && workspaceVisible() 才真跑 |
| 5272 visibilitychange | 事件 | 同上 | — |
| 4755 joinWatch = setInterval | 2s | GET /api/auth/join/status?code=<hr_> | #dlg-join 关闭 / st.active / stopJoinWatch()（4750） |
| 3511 app.dispatchWaitTimer | 1s | 无网络请求，只刷新"正在下发… N 秒"文案 | stopDispatchWait() 3471 |
| 4980 input debounce | 180ms | keyApi(browse) → /api/keys/op 或 /api/agents/<id>/keys | 每次输入重置 |
| 4965 | 400ms | browse 失败后一次性重试（attempt=0→1） | 重试 1 次后显示错误 |
| 1055 toast | 2.6s | — | — |
| 1479 flash-target | 1.4s | — | — |
| 2181 复制成功回显 | 1.2s | — | — |
| 4789 复制按钮文案 | 2s | — | — |

唯一的"实时流"：loadCollectChoicesStream() 2899 用 fetch + response.body.getReader() 读
Accept: application/x-ndjson（GET /api/sync/choices?direction=collect&agent=<id>&stream=1），
由 splitNDJSONLines() 1040 解析 + handleCollectStreamEvent() 2857 分派。
不是 WebSocket/SSE，是 ndjson over fetch（代理缓冲会退化成"一次性到达"——
ws_collect_ui_test.go 正好测了 buffered vs incremental 两条路径）。

### 2.3 refresh 内部流水线（5088-5116）

refresh(): Promise.all([console, agents, plugins])
  → showWorkspace()
  → app.console / app.agents / app.plugins 赋值
  → renderHero() → renderFleetMap() → renderAdaptersOverview() → renderPlugins()
  → loadAgentsRender(app.agents,false) → renderAgents()
  → loadKeys()（单独再发一次 GET /api/keys）
  → stampRefresh(false)

注意 Promise.all 里没有 /api/keys，keys 是 refresh 尾部 loadKeys() 额外串行拉（5103），
所以一次 refresh 实际是 4 个请求（3 并发 + 1 串行）。

---

## 3. 错误处理路径

统一入口 api() 1018-1038：
- fetch 抛错（网络断开/DNS/CORS）→ reject 原始 TypeError，未包装 status/code。
- 非 2xx → 解析 data.error.{message,code,details} 造 Error；另有"写操作失败信封"分支
  （ok:false + status + errors，HTTP 409/422）1029-1034。
- res.json() 解析失败 → data = {}（1023），错误退化成 "请求失败（<status>）"。

| 场景 | 表现 | 出处 |
|---|---|---|
| 401 | refresh() → routeUnauthed("会话已过期，请重新登录") → 回 login 门 | 5107-5110 |
| 403 | 后端不产生 403（requireAuth 只写 401，internal/web/auth.go:76）；前端也无 403 分支 → 走通用错误 | 待验证 |
| 网络断开（fetch reject） | refresh() catch → showWorkspace() + renderHeroError(e.message) + stampRefresh(true)（footer "刷新失败，下次自动重试"） | 5111-5116 |
| 非 401 的 refresh 失败 | hero 变红 + "状态加载失败" + 重试按钮 | 5121-5133 |
| /api/agents 单独失败 | loadAgents() catch → renderAgentsError()（2242，带重试按钮）；401 直接 return 交给 refresh | 2251-2265 |
| /api/keys 失败 | #key-body 写"密钥加载失败：…"；401 静默 return | 4811-4821 |
| 收取流中断 | 对话框内三行错误 + OK/ALT 双 disabled | 2947-2956 |
| 下发失败 | 冲突 → 关对话框 + presentWriteFailure banner；其他 → failDispatchInDialog() 留在 #confirm-err | 3401-3416 |
| 插件操作失败 | #plugin-error / #plugin-action-error（setPluginError 3642） | 3770 等 |
| 机器 status 查询失败 | #view-body "查询失败（机器可能刚离线）：…" | 4738-4741 |
| 存储读取失败 | #view-body "读取存储失败：…" | 4107-4109 |
| 差异读取失败 | 抽屉内 "读取失败：" + 重试按钮 | 4455-4471 |
| 存储文件预览失败 | box.remove() + toast("读取失败："+e.message) | 4345-4348 |

### 3.1 没有错误提示的分支（静默失败）—— 猎人重点

1. watchJoinCode() 4755-4766：catch (_) {} 完全吞掉。接入码状态轮询失败/永久重试无提示。
2. boot() 的 loadHeaderVersion() 5252：catch (_) {} → #ver 空白。
3. logout 5174：try { await api(...) } catch (_) {} → 登出请求失败也照样回登录门（本地状态与服务端可能不一致）。
4. openMachineView() 的 /api/agents/<id>/adapters 4651-4654：catch (_) 只置 overrideViewFailed，
   然后仅加一行小字"暂时无法读取这台机器的适配器停用状态。"——不是错误提示，是静默降级。
5. submitKeyForm() 成功后 if (!spec.next) toast(...) 5045：若 spec.next 存在，无任何成功提示。
6. refresh() 内部 try { await refresh(); } catch (_) {} 出现在 14 处（2091,2231,3350,3380,3409,3590,
   3968,3982,3988,3994,4010,4021,4024,4040）—— 同步成功后的 refresh 失败被静默吞掉，UI 停在旧数据。
7. pathField().refresh() 4963-4965：第 1 次 browse 失败静默 400ms 重试，只有第 2 次失败才显示。
8. renderPlugins() 3665-3673：签名未变直接 return；若 app.plugins 为 null，两个列表都显示
   "插件列表加载中…"（3659）且可能永不更新。
9. loadKeys() 401 分支 4817：return 无提示，靠 refresh 兜底——单独调 loadKeys() 则完全无痕。
10. copyCommand() 2162-2170：navigator.clipboard.writeText(text).then(ok, ask) ——
    resolve 和 reject 都调 ask()（window.prompt）。即复制成功也会弹 prompt。疑似笔误（待验证）。
11. #btn-confirm-ok 早退分支（3197 !action、3209 recordSelectedCount===0、3240 !adapters.length）
    → 空 return，无 toast，用户点了没反应。
12. openScopeDialog 的 .catch(e => {...}) 3094-3098 只写 #confirm-desc，没有 toast，
    对话框底部无红色强调。
13. submitPluginAction() 3914-3916 / 3901-3902：非 ssh-key 插件 / 未选机器 → 只写 #plugin-action-error。
14. copyJoin() 4786-4793：navigator.clipboard.writeText 没有 null 检查（copyAgentId 有），
    非安全上下文会抛 TypeError 未捕获。待验证（与 copyCommand/copyAgentId 三处实现不一致）。

---

## 4. 乐观更新 vs 真实状态（点了只改 DOM，等轮询才反映）

判定标准：动作完成后是否 await refresh()。

### 4.1 有 await refresh()（真实状态，但仍是"先 UI 后拉"）

| 函数 | 位置 | 说明 |
|---|---|---|
| revoke / remove | 2019-2036 | toast 先行，然后 refresh |
| upgradeAgents() | 2091 | 循环逐个 upgrade，最后 refresh |
| upgradeTools() | 2231 | 同上 |
| executeCollect() | 3380 | |
| executeDispatchOne() | 3590 | |
| executeSSHKey() | 3625 | |
| installPlugin/uninstallPlugin/dispatchPluginToAll | 3729, 3747, 3776 | |
| submitThirdPartyPlugin | 3934 | |
| clearResolution | 3968 | |
| dlg-join close | 4797 | 关闭即 refresh |

### 4.2 纯乐观 / 只改 DOM，不等后端确认（猎人重点）

1. app.recordChoiceSelections / app.configPolicySelections（2606-2610, 2698-2700）：
   radio change 只写前端对象，updateScopeConfirm() 重算按钮可用性。未发请求。
2. 类 checkbox（2629-2631）：只 updateScopeConfirm() + alignKeyring()。
3. alignKeyring() 2349-2364：勾选适配器后自动勾/取消 keyring checkbox —— 纯 DOM 推演，
   与后端"密钥能否同行"的判定独立计算（blockedByKeys() 2365）。两套逻辑可能不一致。
4. renderCollectChoicesPreservingChecks() / upsertCollectChoice()（2804-2849）：
   ndjson 事件到达时前端合并旧勾选状态；preserveCollectChoiceChecks 2844 纯本地。
5. #btn-confirm-ok 的 dispatch-done 分支（3191-3194）：只 close()，不 refresh。
6. beginDispatchWait() 3479-3513：setInterval 1s 刷新"已用时"文案 —— 纯前端计时，
   与后端实际进度无关。后端 hang 时计时会一直涨。
7. finishDispatchInDialog() 3531-3557：把后端返回的 paths/problems/resolutions
   直接渲染成"已下发到 X，并已写回：…"，无 toast。
8. theme 切换 5202-5214：纯 localStorage（设计如此）。
9. localStorage "homer-github-user" 3131, 3623, 3848, 3888：写成功/失败都 catch (_) {}。
10. app.openDiffs（1015）：注释说"展开的 diff 抽屉，轮询不重置"，但全文 grep 只有这 1 处声明，
    没有任何读写——死字段。diff 抽屉实际保留是碰巧（不在 renderAgents 重建范围）。
11. copyAgentId 成功回显 2174-2184：改 aidEl.textContent 为"已复制"，1.2s 后还原 —— 乐观 DOM 篡改。
12. #btn-plugin-custom-toggle 5180-5186：纯显隐 form。
13. pluginListSignature 3668：JSON.stringify 比对，顺序敏感；后端顺序变化会导致无意义重渲染（或漏渲染）。

---

## 5. 前端对后端字段的隐式假设（XSS / 崩溃 / NaN 高发点）

总体结论：全文 0 处 innerHTML / outerHTML / insertAdjacentHTML / document.write（grep 确认）。
所有文本都走 el(tag,cls,text) → textContent（1046-1051）。所以用户可控内容注入 DOM 不是 XSS 面。
真正风险是 NaN / 崩溃 / 无判空直取。

| 行号 | 代码 | 风险 |
|---|---|---|
| 1693-1701 | fmtBytes(n)：Number(n) + isFinite 守 + toFixed | 已守卫，OK |
| 1721-1723 | fmtLoad(load)：(Math.round(Number(v)*100)/100).toFixed(2) —— load.one/five/fifteen 未判空；undefined → NaN → "NaN" | NaN 高发（调用点 1786 有 if(host.load) 守，hostSummary 1808 无 load 分支） |
| 1752-1754 | memText(mem)：Math.round(mem.used/mem.total*100) —— mem.used 未判空，undefined/total → NaN | 调用点 1769,1771 守了 mem.total 但不守 mem.used |
| 1776-1778 | disk.used / disk.total（有守）；disk.mount 直拼 "  " | mount 缺失 → UI 显示 "undefined" |
| 1810-1815 | hostSummary：Math.round(host.cpu.usage) 有 != null 守 | OK |
| 1260-1262 | truncateHost(name,max)：name \|\| "—" | OK |
| 3684 | plugin.machineCount：Number.isFinite ? Math.max(0,Math.floor) : 0 | 有守（好例子） |
| 3681 | renderPluginRow：plugin.id \|\| ""、plugin.name \|\| plugin.id \|\| "未命名插件" | 有守 |
| 3904-3906 | JSON.parse(plugin-manifest) + typeof/Array.isArray 检查 | 有守 |
| 4356 / 4535-4536 | conflictFilePath / CONFLICT_MAX_LINES slice | 有守 |
| 4219-4221 | file.path.slice(0,slash) —— slash 守卫需复查 | 待验证 |
| 4848-4860 | renderKeys：view.keys \|\| []（4812, 5043）有守；key.files \|\| []（4860）有守；但 file.destination + note 直拼 | destination 缺失 → "undefined" |
| 5003 | openKeyForm：titles[action] —— action 非法 → undefined 标题 | 低（调用点固定 4 种） |
| 5016-5018 | openKeyForm encrypt 分支 key.name \|\| key.id —— key 为 null 时崩溃（TypeError） | 中（encryptDestination 4315 用 app.keys[0]，有 length 守） |
| 1599/1620/1646/1663 | "存储 第 " + c.generation + " 代" —— c.generation 未判空 | 低（!published 分支提前 return） |
| 1457-1459 | isWaitingFirstDispatch(a)：d.error.indexOf("homer")>=0 && indexOf("init")>=0 —— 字符串匹配判定新机器 | 脆弱：错误信息文案一变，"新机器 · 等待下发"失效 |
| 1881 / 1175 | (d.push\|\|0)+(d.pull\|\|0)+(d.conflicts\|\|0) | 有守 |
| 1497 | share = fleetMaxDrift>0 ? n/fleetMaxDrift : 1 | OK |
| 1319-1338 | renderFleetMap：m.hostname \|\| m.agentId \|\| "—" | 有守 |
| 2933 | JSON.parse(line) 有 try/catch → 抛"机器返回了无法解析的收取结果" | 有守 |
| 1023 | res.json() 有 catch | 有守 |

最值得猎的 4 处：fmtLoad 1722（NaN）、memText 1753（mem.used 未守）、
disk.mount 1778 / file.destination 4860（"undefined" 字面量泄漏到 UI）、
isWaitingFirstDispatch 1457（字符串匹配判定新机器）。

---

## 6. e2e 已断言过的行为（别把设计意图当缺陷）

| 行为 | 测试文件:行 |
|---|---|
| setup 密码过短 → #err-setup 含"至少" | console_docker_test.go:96 |
| 错误密码 → #err-login 含"密码" 且 #gate-login 不隐藏 | console_docker_test.go:105-108 |
| 登录失败退避 ~2s（下次尝试被拒） | console_docker_test.go:109-111（源码 internal/web/password.go verifyPassword） |
| logout → workspace 隐藏 | console_docker_test.go:114-117 |
| hr_ 码一次性：复用已兑换码不再加机器 | console_docker_test.go:119-143 |
| 新机器卡片显示「新机器 · 等待下发」 | console_docker_test.go:132 |
| 查看不得出现 502 / "查询失败" | console_docker_test.go:147-152 |
| 中心为空时下发：#confirm-desc 含"中心还没有任何内容"，OK 保持 disabled（不是 toast 报错） | console_docker_test.go:157-163 |
| 空中心下发不得写 homer.json | console_docker_test.go:165-167 |
| 收取成功 toast 含"已从 X … 收取" | console_docker_test.go:170-172 |
| 下发成功 toast 含"已下发到 X" | console_docker_test.go:190 |
| 漂移 badge 滞后于下发（可达 30s+）—— 测试用 waitGone(...,75s) 容忍 | console_docker_test.go:193-194 |
| 接入命令不得含 --listen / --connect | console_docker_test.go:200-202；ws_collect_ui_test.go:342 |
| 登录公钥写入：已有 key 保留 + "# BEGIN homer github <user>" 恰好 1 次 | console_docker_test.go:375-380 |
| 不存在的 GitHub 用户 → outcome 含"不存在" 且 authorized_keys 不被改动 | console_docker_test.go:367-371 |
| 离线机器：.agent-card.stale + 查看 disabled；重启后恢复 | console_docker_test.go:270-292 |
| 离线时下发：desc 含"离线"且 OK 保持 disabled（不是 toast 报错） | console_docker_test.go:398-403 |
| 吊销/移除 toast 分别是"已吊销" / "已移除" | console_docker_test.go:390, 408 |
| 冲突裁决：radio data-resolution-adapter，OK=「记录决定」，ALT=「记录并立即执行」 | console_docker_test.go:335-345；staged_resolution_ui_test.go:147-158 |
| 记录决定（OK）不得产生 push/pull | staged_resolution_ui_test.go:172 |
| 下发对话框 #confirm-resolutions 必须列出已记录决定 | staged_resolution_ui_test.go:234-241 |
| 收集对话框流式：#confirm-note 含"已读取 N/M"（增量）/ 一次性全量，两条路径终态必须一致 | ws_collect_ui_test.go:225-291 |
| 收集流只调 1 次 inspect（stub.calls()==1） | ws_collect_ui_test.go:288-290 |
| join 对话框：恰好 1 个 readonly input，无 listen 控件，命令含 hr_ | ws_collect_ui_test.go:323-350 |
| 适配器停用：GET /api/agents/<id>/adapters 按 role 过滤，keyring(carrier) 永不出现 | agent_overrides_ui_test.go:80-84（注释要求 mock 与真实实现同形状） |
| 绑定密钥时下发：OK 文案变「下一步」→ 解锁步 → 错误口令留在对话框 | dispatch_unlock_ui_test.go:88-146 |
| 密钥带不上时该适配器必须 disabled 且给出理由；无绑定密钥的仍可下发 | dispatch_unlock_ui_test.go:294-341 |
| 以中心为准需口令，以机器为准不需要 | dispatch_unlock_ui_test.go:403-478 |
| 密钥 click 流：create → encrypt → unlock → 明文写回；页面不得出现密钥明文 | keyring_ui_test.go:62-111 |
| 插件：安装/卸载/下发到全部/ssh-key 动作；卸载 confirm 必须含"卸载守卫"文案 | plugins_ui_test.go:102-212 |
| 第三方 manifest 非法 JSON → #plugin-error 可见 | plugins_ui_test.go:224-243 |
| 应用升级：落后标红 + hover title 说明；不可升级的不给按钮；失败不得改卡片版本号 | tool_upgrade_ui_test.go:286-390 |
| 升级 confirm 必须提及涉及数量 | tool_upgrade_ui_test.go:326, 390 |
| 详情对话框在升级进行中必须关闭（不遮挡进度/结果） | tool_upgrade_ui_test.go:361 |

---

## 7. 后端端点枚举（前端实际会打的）

/api/health · /api/auth/{status,setup,login,logout,join,join/status} · /api/console ·
/api/agents · /api/agents/revoke · /api/agents/remove ·
/api/agents/<id>/{status,adapters,adapters/<aid>,diff,pull,push,upgrade,tool-upgrade,ssh-key,keys} ·
/api/sync/choices · /api/sync · /api/resolve/record · /api/resolve/clear · /api/resolve ·
/api/plugins · /api/plugins/{install,uninstall} · /api/keys · /api/keys/{encrypt,unlock,password,op} ·
/api/storage · /api/storage/file

服务端路由 switch：internal/web/handlers.go:100-230；agent 子路由 handlers.go:432-480
（len(parts)!=2 → 404；/api/agents/<id>/adapters/<aid> 仅 POST）。
错误信封：internal/web/errors.go:17-45（code/message/details）；
status 映射 errors.go:71-99（agent-unreachable→502, agent-offline→503, agent-outdated→409,
agent-timeout→504, agents-disabled→501）。

---

## 8. 待验证问题

1. copyCommand() 2162-2170 的 .then(ok, ask) 是否确为笔误（复制成功也弹 window.prompt）——
   需实机点一次"复制安装命令"确认。
2. copyJoin() 4786 navigator.clipboard.writeText 无 null 检查，非安全上下文是否真抛错。
3. app.openDiffs（1015）确认为死字段（全文仅 1 处引用）。
4. file.path.slice(0,slash) 4219-4221 是否有 slash 守卫。
5. 后端是否真不产生 403（auth.go:76 只写 401；需确认有无其他路径）。
6. #keys-fold-hint（843）没有任何 JS 写它——hint 文案永久是初始中文，确认是否为缺陷。
7. renderHeroSkeleton() 1683 与 loadAgents() 2251 定义了但 grep 仅命中定义处（+2247 retry 引用），
   确认是否为死代码。
8. 本轮未实机操作浏览器（未连 CDP），所有 UI 行为结论均来自源码静态阅读。

---

## 9. 给猎人的最短路径提示

- 最密集的交互面在 #dlg-confirm（一个 dialog 承载 5 种 action kind，靠 app.pendingAction.kind
  与 app.scopeStep 二维状态机分流：3197-3256）。状态机交叉分支最可能有洞。
- refresh() 是 30s 一次的全量重建（renderAgents 会 clear(list) 重建所有卡片），
  对话框外的 DOM 在轮询时被整块替换——正在展开的 details、滚动位置、焦点都会丢。
- 所有"进度"都是假的（busy 遮罩 + 1s 计时器），没有中断/取消按钮
  （cancel 在 beginDispatchWait 里被 disabled=true，3489）。下发中无法取消。
- #btn-refresh 无防抖：连点会产生并发 refresh，响应乱序可能导致 UI 显示较旧的数据。
