# 冻结面侦察 · hub 后端 / agent 协议 / 契约

仓库 /root/code/homer，HEAD 010521a。全部证据带 文件:行号。
运行时事实来自 127.0.0.1:17760（qa-hub-token / hr_ 码实机 GET 验证，见 2.4 / 9）。

---

## 0. 路由总览（实现真源）

路由不是 http.ServeMux，是 internal/web/handlers.go:24 serveHTTP 的手写 switch。
server.go:272 把 server.serveHTTP 装成唯一 handler。

分发顺序（顺序即鉴权边界）：

| 次序 | 条件 | 位置 |
|---|---|---|
| 1 | path == "/" -> serveIndex | handlers.go:32 |
| 2 | /static/ 前缀 -> serveStatic | handlers.go:40 |
| 3 | /api/health -> handleHealth（无 requireAuth）| handlers.go:48-55 |
| 4 | /api/auth/ 前缀 -> handleAuthAPI（无 requireAuth）| handlers.go:56-59 |
| 5 | /install.sh -> serveInstallScript（无 requireAuth）| handlers.go:64-71 |
| 6 | /dl/homer, /dl/homer.gz -> 先 requireAuth 再 serveDownload | handlers.go:72-82 |
| 7 | /api/ 前缀 或 /api -> requireAuth（门）| handlers.go:83-87 |
| 8 | /agent/v1/stream -> handleAgentEndpoint（绕过第 7 步）| handlers.go:88-91 |
| 9 | /agent/ 前缀 -> handleAgentEndpoint | handlers.go:92-97 |
| 10 | 其余 -> 大 switch（业务路由）| handlers.go:99-223 |

---

## 1. API 契约清单：文档 vs 实现

### 1.1 docs/api-reference.md 声明的（已实现，逐条对上）

| 文档 | 实现位置 | 备注 |
|---|---|---|
| GET /api/plugins | handlers.go:173 -> plugins.go:81 | |
| POST /api/plugins/install | handlers.go:173 -> plugins.go:155-218 | |
| POST /api/plugins/uninstall | handlers.go:173 -> plugins.go:240-293 | |
| POST /api/sync?direction=dispatch | handlers.go:136 -> sync.go:87,216 | |
| GET /api/agents/<id>/adapters | handlers.go:219 -> 432 -> agent_overrides.go:208 | |
| POST /api/agents/<id>/adapters/<adapterID> | handlers.go:438 -> agent_overrides.go:266 | |

### 1.2 文档有 / 实现缺失或状态码不符（猎人候选）

1. POST /api/plugins/install 的 404 分支文档未写。api-reference.md:141 只列了
   422（schema 版本不支持）、422（第三方 role）、409（ID 冲突）。
   实测 {"id":"nope"} -> **404 plugin-not-found**（plugins.go:192）。
   422 分支 plugins.go:226、409 分支 plugins.go:224 与文档一致。
2. Agents 未启用时 501（api-reference.md:215）：实机永不触发。
   判据是 s.opts.Agents == nil（handlers.go:433-436），
   而 homer serve 恒注入 Agents（cli/hub.go:95）。只在嵌入式模式出现，文档未说明。
3. GET 与 POST 对幽灵机器语义不一致。api-reference.md:229-230 把
   agent-not-found 404 写在 POST .../adapters/<id> 条目下。
   POST 侧确实有幽灵防御（agent_overrides.go:273-285）；
   但 GET /api/agents/nope/adapters 实测 200（空 adapters 列表），
   agent_overrides.go:208 只校验 agentID 非空，没有"机器必须存在"检查。

### 1.3 实现有、文档无（api-reference.md 完全没提的端点）

docs/api-reference.md 自称"插件管理与适配器下发"，只覆盖 6 个端点。
以下全部未在 api-reference.md 中出现（README.md:143 只做了一句跳转）：

GET /api/health(handlers.go:48,248)、GET /api/console(100 -> sync.go:816)、
GET /api/status(106,257)、POST /api/push(112,323)、POST /api/pull(118,345)、
GET /api/diff(124,282)、GET /api/sync/choices(130)、
POST /api/sync collect/to-others/from-center(136 -> sync.go:115,269,304)、
POST|GET /api/snapshot(143-153 -> sync.go:597,653)、
POST|GET /api/resolve/record(154 -> resolve_record.go:97)、
POST /api/resolve/clear(160 -> resolve_record.go:202)、POST /api/resolve(166)、
GET|POST /api/keys 与 /api/keys/{encrypt,unlock,password,op}(175 -> keys.go:23-66)、
GET /api/config(177,374)、GET /api/agents(183,405)、
POST /api/agents/revoke(189,1006)、GET /api/storage(197 -> sync.go:872)、
GET /api/storage/file(205 -> sync.go:1058)、POST /api/agents/remove(211,1033)、
POST /api/agents/<id>/{status,diff,push,pull,ssh-key,keys,upgrade,tool-upgrade}(451-511)、
GET /api/auth/{status,setup,login,logout,join,join/status}(56 -> password.go:353)、
GET /dl/homer 与 /dl/homer.gz(72 -> 929)、GET /install.sh(64 -> 907)、
GET /static/index.html(40 -> 237)、GET|POST /agent/v1/stream 与 /agent/*(88-97 -> hub/agenthub.go:106)。

### 1.4 参数 / 状态码不一致（实现内部）

- writeMethodNotAllowed（handlers.go:872）对所有路由都发 Allow: GET, POST，
  即使该路由只接受 GET（如 /api/console）。误导性 header。
- 未确认（confirm 缺失）一律 409 + aborted（sync.go:129,224; resolve.go:50,85;
  resolve_record.go:109,209）。api-reference.md:196 只说返回 aborted，没说状态码。
- reportHTTPStatus（handlers.go:717）：ok->200；aborted 或含配置错误->409；否则 422。
  冲突未解决等普通业务失败也是 422，语义区分度低。

---

## 2. 鉴权矩阵

### 2.1 四条通道（authorized()，判定顺序）internal/web/auth.go:37-74

1. session cookie — s.auth.validSession()（auth.go:41），HMAC 签名，password 登录签发。
2. Bearer = hub token — provided == s.opts.Token（auth.go:55）。
3. Bearer = 未兑换的 hr_ 码 — s.opts.Enrollment.ValidCode(provided)（auth.go:62）。
   注释明确：只 CHECK，不 burn（auth.go:58-61）。
4. Bearer = 已接入机器的 per-agent secret — s.opts.AgentEndpointAuthorized(r)
   （auth.go:68）-> 实为 hub.Authenticator.Authorized（cli/hub.go:98 -> hub/auth.go:52-55），
   只允许 PrincipalSecret / PrincipalHubToken。

关键矛盾：hub/auth.go:52-55 明确把 PrincipalEnrollCode 排除在 /api、/dl 数据面之外，
但 web/auth.go:62 的通道 3 是独立路径，不受 hub/auth.go 约束。
homer serve 把同一个 Enrollment 同时塞给 web（cli/hub.go:97）和 hub（cli/hub.go:79-80）。

### 2.2 矩阵（行 = 路由前缀，列 = 凭证）

| 路由 | cookie | hub token | 未兑换 hr_ 码 | per-agent secret | 无凭证 |
|---|---|---|---|---|---|
| /api/health | 200 | 200 | 200 | 200 | **200 公开**（handlers.go:48 在门之前）|
| /api/auth/status | 200 | 200 | 200 | 200 | **200 公开**（handlers.go:56；password.go:355）|
| /api/auth/setup POST | ok | ok | ok | ok | **公开**（password.go:365，受 setupPeerAllowed 限制，见 2.5）|
| /api/auth/login POST | ok | ok | ok | ok | **公开**（password.go:367）|
| /api/auth/logout POST | ok | ok | ok | ok | **公开**（password.go:369）|
| /api/auth/join GET | ok | ok | **ok** | **ok** | 401（password.go:376 内部再查 authorized）|
| /api/auth/join/status GET | ok | ok | **ok** | **ok** | 401（password.go:387）|
| 其余 /api/* | ok | ok | **ok** | **ok** | 401（handlers.go:83-87）|
| /dl/homer、/dl/homer.gz | ok | ok | ok | ok | 401（handlers.go:72-82）|
| /install.sh | 200 | 200 | 200 | 200 | **200 公开**（handlers.go:64；注释 60-63 明确）|
| / 与 /static/index.html | 200 | 200 | 200 | 200 | **200 公开**（handlers.go:32,40）|
| /agent/v1/stream | n/a | 端点自查 | 端点自查 | 端点自查 | **401**（hub/agenthub.go:135-140）|
| /agent/v1/{enroll,register,poll,report} | 410 | **410** | **410** | **410** | **410**（hub/agenthub.go:121-123，完全无鉴权）|
| /agent/* 其他未知 | 404 | **404** | **404** | **404** | **404**（hub/agenthub.go:125-128）|

注：/agent/* 在 handlers.go:92-97 被提前 return，永远走不到 handlers.go:83 的门。
注释（handlers.go:93-94）说明是有意的：不能让 web 门盖住 410/404。

### 2.3 是否漏鉴权

- /api/health 公开（handlers.go:48），返回
  {"ok","status","version","homerHome"}（handlers.go:249-254）。
  **实测泄漏 hub 绝对路径 /root/.homer**。低危信息泄漏。
- /api/auth/*、/install.sh、/ 公开是设计意图（password.go:350-352；
  handlers.go:60-63；installscript.go:11-13）。
- **未发现 /api/* 业务端点漏鉴权**：handlers.go:83 的前缀门覆盖 /api 与 /api/ 全部。

### 2.4 hr_ 码超出预期的可访问范围（实机验证，最强候选）

用 GET /api/auth/join（password.go:372）铸 hr_9a6d...（TTL 24h，password.go:505），
带 Authorization: Bearer hr_9a6d... 实测：

    /api/agents      -> 200   完整机器清单 + drift + host(os/arch/MAC/IP/CPU) + tools
    /api/console     -> 200   完整控制台数据
    /api/status      -> 200
    /api/keys        -> 200   密钥环操作面
    /api/plugins     -> 200
    /api/config      -> 409   （无配置，但已过鉴权门）
    /api/snapshot    -> 409   （无快照，但已过鉴权门）
    /dl/homer.gz     -> 200   二进制下载（设计意图）
    /api/agents/remove POST -> 404（已过门，仅目标不存在）

结论（可报）：一个"一次性接入码"在未兑换期间等价于完整的 hub token——
能读 /api/agents、/api/console，能操作 /api/keys，能删/吊销机器。

internal/hub/auth.go:49-55 的注释写得很清楚：
  "The one-time enrollment code can download bootstrap assets and open a stream,
   but it is not accepted for /api or /dl data access after this initial admission check."

即 hub 包的设计意图是拒绝 hr_ 码访问 /api 数据面，
但 internal/web/auth.go:62 的独立通道 3 绕过了它。
-> hub/auth.go 的注释与 web/auth.go 的行为互相矛盾。

放大项：
- TTL 24 小时（password.go:505 Mint(24 * time.Hour)）。
- ValidCode 不 burn（enrollment.go:89-97），窗口内可无限次使用。
- 码可自我增殖：持有 hr_ 码调 GET /api/auth/join -> 200 并铸出全新 hr_ 码
  （实测返回 hr_a022c1ce...）。一个码 -> 无限个码。
- 已兑换/已失效的码仍可用于 /api：实测用前面铸过的码 GET /api/keys -> 200
  （burn 只发生在 stream hello 的 Redeem 路径，enrollment.go:103-121，
  从未走该路径的码始终 ValidCode 为真）。
- 伪造码 hr_000... -> 401（对照，说明确实是 ValidCode 在起作用）。

### 2.5 setup 的额外边界

setupPeerAllowed(r)（password.go:330-348）：peer IP 为 loopback/private/link-local
即 trusted（password.go:346-348）。trusted peer 裸设管理员密码（password.go:434-441）；
公网 peer 必须给一次性 6 位 setup code（password.go:420-433）。
code 在所有廉价校验之后才 burn（password.go:415-419 注释 + auth_test.go:598）。
GET /api/auth/status 会预先告诉浏览器该 peer 是否需要 code（password.go:360-364）。

---

## 3. agent 生命周期状态机

### 3.1 状态与转移

    [MINT]  code = "hr_" + 48hex，TTL 24h
            enrollment.go:73-83 (Mint) / password.go:505 (TTL=24h)
              |
              v
    [DOWNLOAD]  GET /dl/homer.gz  Bearer hr_xxx
            handlers.go:72-82 -> auth.go:62（只 check 不 burn）
              |
              v
    [DIAL WS]  GET /agent/v1/stream  Sec-WebSocket-Protocol 必须匹配
            hub/agenthub.go:113-120 -> 141-145(subprotocol) -> 146-150(Hijacker)
                                    -> 151-154(hub 关闭中 -> 503 hub-shutting-down)
            -> hub/auth.go:30 Authenticate（三选一：token / secret / code）
              |
              v
    [HELLO]  第一帧必须是 TReq + MethodHello + 非空 ID
            agenthub.go:296 HelloTimeout -> ClosePolicy "hello timeout"
            agenthub.go:178 proto 不符 -> CodeUnsupported + CloseProtocol
            agenthub.go:183 secret 的 AgentID != hello.AgentID -> agent-id-mismatch + CloseRevoked
            agenthub.go:191 secret 在 hello 期间被 revoke -> CodeUnauthorized + CloseRevoked
              |
              +- PrincipalSecret     -> 不发新 secret（welcome.AgentSecret 为空）
              +- PrincipalEnrollCode -> Redeem (agenthub.go:206)
                                      -> BindAgent (agenthub.go:214, bind before welcome)
                                      -> welcome.AgentSecret = 新 secret (agenthub.go:220)
              |
              v
    [WELCOME]  WelcomeResult{Proto,HubVersion,InstanceID,AgentSecret,Ping*,MaxFrame}
            proto.go:39-47 / agenthub.go:218-224
              |
              v
    [ATTACH]  Registry.Attach(info, session) -> 返回 prev session
            registry.go:73-114
            prev != nil -> closeSession(prev, CloseSuperseded, same agent ID connected elsewhere)
            agenthub.go:258-261
              |
              v
    [ONLINE]  session.Run()；每收到一帧 Touch（刷新 LastSeen）
            registry.go:282-291 (Touch)
            heartbeat(hb) -> agenthub.go:378-404 更新 Version/Drift/Host/Tools
              |
              v (断线)
    [DISCONNECT]  <-runDone ; <-sessionConn.closeDone
            agenthub.go:271-278
            Registry.Detach(agentID, session)（只有 session 仍是当前那个才清）registry.go:118-127
              |
              v
    [RECONNECT]  agentd/client.go:273-313 按 close code 决定退避

### 3.2 超时常量（全部带 file:line）

| 常量 | 值 | 位置 |
|---|---|---|
| AgentStaleAfter | **90s** | hub/task.go:74 |
| PullWait | 12min | hub/task.go:77 |
| UpgradeWait | 3min | hub/task.go:78 |
| ToolUpgradeWait | 7min | hub/task.go:79 |
| CallBudgetDefault | 60s | hub/proto.go:58 |
| ReqDeadlineSlack | 5s | hub/proto.go:59 |
| CallBudget(pull/upgrade/tool) | 12/3/7 min（与 task.go 重复定义两套）| hub/proto.go:62-72 |
| HelloTimeout（默认） | 10s | hub/agenthub.go:61 |
| stream.DefaultPingInterval | 25s | stream/session.go:15 |
| stream.DefaultPingTimeout | **75s** | stream/session.go:16 |
| stream.DefaultWriteTimeout | 15s | stream/session.go:17 |
| stream.DefaultMaxFrame | 8MiB | stream/session.go:14 |
| agentCallConcurrency | **64** / 机器 | hub/dispatcher.go:19 |
| maxConcurrentAgentPulls | **8**（fanout）| web/sync.go:357 |
| snapshotBodyLimit | 32MiB | web/sync.go:595 |
| keyBodyLimit | 64KiB | web/keys.go:21 |
| readSyncScope body limit | 1MiB | web/scope.go:69 |
| readJSONBody limit | 64KiB | web/password.go:515 |
| agentd TaskTimeout 默认 | 50s | agentd/agentd.go:107 |
| agentd Backoff | base 1s / max 1min / x2 / jitter .5 | agentd/agentd.go:110 |
| agentd DriftInterval / DriftTimeout | 30s / 20s | agentd/agentd.go:225,228 |
| agentd HTTP client timeout | 120s | agentd/executor.go:773 |
| agentd hello budget | 10s | agentd/client.go:219,221 |
| releaseCacheTTL | 10min | web/releaseproxy.go:26 |
| hub HTTP WriteTimeout | 13min | cli/hub.go:142 |
| hub ReadHeaderTimeout | 10s | cli/hub.go:138 |
| 优雅关闭 drain | 10s | cli/hub.go:164 |

### 3.3 断线后多久标记离线

两套独立判据，且数值不一致：

1. agentIsStale（registry.go:341-343）：
   session == nil || sessionDone(session) || now-LastSeen >= AgentStaleAfter(90s)。
   - 干净断线：Detach 立刻把 session 置 nil -> **立即 stale**（registry.go:124-126）。
   - 半开连接（TCP 未断、对端消失）：session 仍非 nil 且未 done，
     要等 ping timeout **75s**（session.go:16）后 session 才 done；
     同时 LastSeen 停止刷新，**90s**（task.go:74）后 stale。
     -> 半开场景最坏 ~90s（90 > 75，所以 90s 是上限）。
2. Registry.Session()（registry.go:130-141）**只查 sessionDone，不查 LastSeen**。
   -> 存在 75s ~ 90s 的窗口：UI（Get/List 走 agentIsStale）已显示离线，
   Session() 仍返回活 session。dispatcher 靠 requireOnline 的
   !ok || info.Stale 双重判据（dispatcher.go:385-389）兜住。

### 3.4 重复 Redeem 同一 hr_ 码

Redeem（enrollment.go:103-121）：码存在就**先 delete**（enrollment.go:106-108），再判过期。

- 第二次 Redeem 同一码 -> !ok -> error 接入码无效或已使用（enrollment.go:111）。
- **已兑换码在 web 鉴权通道仍有效**：ValidCode 不 burn（enrollment.go:89-97），
  实测 -> **200**（见 2.4）。即"一次性"语义只在 stream hello 路径成立，
  在 /api 与 /dl 路径完全不成立。
- 过期码：ValidCode 判 time.Now().Before(expiresAt)（enrollment.go:96），
  过期后 web 通道也失效（但 map 里的条目要等重启或一次 Redeem 才清）。
- 码仅存内存（enrollment.go:30-31 注释 One-time codes stay in memory）
  -> hub 重启后所有未兑换的 hr_ 码全部失效（已绑定 secret 落盘，不受影响）。

### 3.5 同机器名 / 同 agent id 重复接入

- **AgentID 相同** -> Registry.Attach 走 current, ok := r.agents[info.AgentID]
  分支（registry.go:91）：旧 session 作为 prev 返回，hub 用
  CloseSuperseded + same agent ID connected elsewhere 关掉（agenthub.go:260）。
  Host/Tools/Drift/Version 缺失时**沿用旧值**（registry.go:93-104）。
  ReportedAdapters 保留（registry.go:109 注释：观测状态跨重连存活）。
  **caps 不继承**（registry.go:105-108 注释：新 hello 重新协商）。
- **Hostname 相同但 AgentID 不同** -> 两条独立记录，**无去重**。
  列表按 AgentID 排序（registry.go:321-323），UI 会出现两个同名机器。
- **agent id 生成**：实测为 hostname-<8hex>（如 e363efdcf085-7b6d），
  hostname + 随机后缀 -> 同一 hostname 也会产生不同 agent id。

### 3.6 agent 重启后身份是否保持

- **secret 侧保持**：BindAgent 把 sha256(secret) 写 keys/agent-secrets.json
  （enrollment.go:126-134, 231-271；0600）。hub 重启后 OpenEnrollment 重载
  （enrollment.go:53-70），VerifySecret 仍认（enrollment_test.go:100-114）。
- **agent 侧保持**：agent.json 的 agentSecret（cli/hub.go:224-226,233）。
- **registry 侧丢失**：registry 纯内存，hub 重启后机器列表**空**，
  要等 agent 重连才回来（LastSeen/Host/Drift/Tools 全丢）。
- **hr_ 码 vs agent.json 优先级**（cli/hub.go:286-306 resolveAgentCredential）：
  install.sh 把 hr_ 码写进 keys/hub-token 且不清理（cli/hub.go:280-282 注释）。
  explicit --token hr_ > keys/hub-token 的 hr_（仅当 agent.json 无 secret 时）
  > persisted secret。agent.json 已有 secret 时忽略文件里的过期 hr_ 码（正确，有注释）。

### 3.7 注销（两条路径）

| 操作 | 端点 | 行为 |
|---|---|---|
| revoke | POST /api/agents/revoke（handlers.go:1006）| 只删 secret 绑定，**保留 registry 行**（handlers.go:1003-1005 注释）。revokeHook 立即 Kick（agenthub.go:89-93）。未绑定过 -> 409 not-enrolled（handlers.go:1023）|
| remove | POST /api/agents/remove（handlers.go:1033）| **先 Revoke 再 RemoveAgent**（handlers.go:1053-1059，顺序有注释：先杀凭证再删行）。RemoveAgent 内部 Hub.Kick(CloseRemoved)（dispatcher.go:511-519）。不存在 -> 404（handlers.go:1057）|

revoke 后 agentID 保持可重新接入（enrollment.go:152-154 注释）。

---

## 4. 下发 / 任务链路

### 4.1 链路

    POST /api/agents/<id>/pull   (handlers.go:478-485, 573)
      +- global writeMutex（handlers.go:483；package-global，server.go:242-245）
      +- readSyncScope (scope.go:56)
      +- 停用 adapter 检查 (handlers.go:580-595)
      +- requireCenterAdapters (handlers.go:597)
      +- refuseBoundKeyGap / dispatchUnlockErrors (handlers.go:611-624)
      +- Dispatcher.AgentPull (dispatcher.go:128)
      |    +- writeAgent -> callTask -> call (dispatcher.go:263,312,319)
      |       +- budget = CallBudget(kind)               (dispatcher.go:313, proto.go:62)
      |       +- requireOnline #1                        (dispatcher.go:332)
      |       +- acquireAgentCall：per-agent 信号量 64    (dispatcher.go:336, 404-427)
      |       +- requireOnline #2 <- 拿到信号量后再查一次 (dispatcher.go:341)
      |       +- hasCapability(caps, method)             (dispatcher.go:345, 451)
      |       +- session.Done() 检查                     (dispatcher.go:353)
      |       +- remoteBudget = remaining - 5s           (dispatcher.go:364)
      |       +- session.Call                            (dispatcher.go:373)
      |    +- noteWriteOutcome (dispatcher.go:268,272-285) -> Registry.NoteWriteOutcome
      +- 后置 unlock (handlers.go:630-636)：pullWarrantsUnlock -> unlockDispatched
         -> 失败则 422 unlock-failed（handlers.go:633）

    POST /api/sync?direction=dispatch   (sync.go:216)
      +- **不持 writeMutex**（只有 collect/to-others/from-center 走 sync.go:103-104）
      +- fanoutPullOnlineAgents (sync.go:436-511)：8 并发 (sync.go:357,449)

### 4.2 超时 / 重试 / 并发 / 队列上限

| 项 | 值 | 位置 |
|---|---|---|
| 端到端 budget（默认） | 60s | proto.go:58, dispatcher.go:327 |
| pull | 12min | proto.go:64-65 |
| upgrade | 3min | proto.go:66-67 |
| tool-upgrade | 7min | proto.go:68-69 |
| 留给 agent 的 remote budget | budget - 5s | dispatcher.go:364 |
| **per-agent 并发** | **64**（channel 容量）| dispatcher.go:19, 414 |
| fanout 并发 | 8 | sync.go:357,449 |
| **重试** | **无**。hub 侧零重试（dispatcher.go 全文无 retry）| — |
| agent 侧退避 | base 1s / max 1min / x2 / jitter .5 | agentd/agentd.go:110 |
| agent 固定重试延迟 | 401/410/协议不符/revoked/removed -> 5min；superseded -> 30s | agentd/client.go:279-313 |
| **离线队列** | **不存在**。语义是"跳过"（sync.go:462-465 info.Stale -> Skipped）| — |

"离线队列"只在测试里以"排队等信号量"的形态出现：
dispatcher_offline_queue_test.go:15 断言第 65 个调用在信号量上排队、
期间 session detach、拿到 token 后第二次 liveness 检查返回 agent-offline 503。
那是**并发排队**，不是离线任务队列。

### 4.3 任务卡在中间态的可能路径（猎人重点）

1. **writeMutex 是 package-global 且无超时**（server.go:242-245 注释）。
   /api/agents/<id>/{push,pull,ssh-key} 持有它（handlers.go:475,483,491），
   /api/push、/api/pull（handlers.go:329,351）、/api/keys/*（keys.go:79-80）
   共享同一把锁。一个 12min 的 pull 会**阻塞所有其他机器的写操作**，
   **无 try-lock、无超时、无排队上限** -> 请求无限挂起。
2. **generationMutex 与 writeMutex 分离是有意的**（sync.go:583-587 注释，
   防 hub->agent->hub 死锁）。但 fanout（/api/sync?direction=dispatch）**不持 writeMutex**，
   而单机器 pull 持 -> 两者可并发修改 generation。
   POST /api/snapshot 的 CAS（sync.go:611-621）在 generationMutex 下保护，
   但 fanout 的 pull 不经过 CAS。**是否产生混合世代，待验证。**
3. **HTTP WriteTimeout 13min（cli/hub.go:142）vs pull budget 12min**：
   注释（cli/hub.go:140-141）说 agent stream 被 hijack 后 deadline 清除，
   但 POST /api/agents/<id>/pull **不 hijack** -> 12min budget + 排队时间
   可能撞上 13min WriteTimeout，客户端拿到空响应/连接重置，
   **而 hub/agent 侧任务其实跑完了** -> 文件已写但控制台显示失败。
4. **后置 unlock 失败**（handlers.go:630-636）：内容**已下发**，
   但 unlock 失败 -> **422 unlock-failed**。机器上是"写了但解不开"的半完成态。
   详情由 pullUnlockFailureDetails（handlers.go:777-810）塞进 details。
5. **NoteWriteOutcome 只在成功路径调用**（dispatcher.go:268）。
   callTask 返回 err 时 registry 的 drift **不更新** -> UI drift 计数停留在旧值（陈旧 UI）。
6. **信号量 acquire 失败的释放**：acquire 返回 err 后 call 立刻 return
   （dispatcher.go:337），defer release 在 339 行（之后）-> 不会误 release。
   已核对，**不是 bug**。
7. **refuseBoundKeyGap 返回 true 时已写过响应**（keys.go:197）-> 提前 return，无双写。

---

## 5. 错误码与消息

### 5.1 错误码 -> HTTP 状态（两套映射，互不一致）

**A. agentStatusForCode（web/errors.go:77-94）— 只有 6 个码：**

| code | status | 位置 |
|---|---|---|
| agent-not-found | 404 | errors.go:79-80 |
| agent-unreachable | 502 | errors.go:81-82 |
| agent-offline | 503 | errors.go:83-84 |
| agent-outdated | **409** | errors.go:85-86 |
| agent-timeout | 504 | errors.go:87-88 |
| agents-disabled | **501** | errors.go:89-90 |
| 其他 -> 0 -> **500** | | errors.go:62-64 |

**B. handler 里直接写死（不经 agentStatusForCode）：**

| code | status | 位置 |
|---|---|---|
| unauthorized | 401 | auth.go:81 |
| not-found | 404 | handlers.go:222 |
| method-not-allowed | 405 | handlers.go:874 |
| not-initialized | **409** | handlers.go:849 |
| invalid-config | **409** | handlers.go:387,392,852 |
| bad-request | 400 / **422** / **409** | handlers.go:326 / 602 / 621 |
| no-snapshot | **409** | sync.go:232,657 |
| generation-conflict | 409 | sync.go:617 |
| platform-unknown | 409 | handlers.go:956 |
| unlock-required | **422** | handlers.go:616 |
| agents-disabled | **501 或 422**（同一 code 两种状态）| handlers.go:618-619 |
| unlock-failed | 422 | handlers.go:633 |
| upgrade-failed | 422 | handlers.go:670 |
| override-invalid | 400 / **422** | agent_overrides.go:210 / 287 |
| overrides-disabled | **503** | agent_overrides.go:291 |
| plugin-not-found | **404** | plugins.go:192 |
| plugin-installed | 409 | plugins.go:200,224 |
| not-enrolled | 409 | handlers.go:1023 |
| adapter-disabled-for-agent | 400 | handlers.go:581,590 |
| ssh-key | 400/404/502（**按错误消息子串匹配决定**）| handlers.go:688-693 |
| release-fetch | 502 | releaseproxy.go:54 |
| plugins-disabled | 503 | plugins.go:608 |
| enroll-disabled | 501 | handlers.go:1008 |

errorMessage（errors.go:96-127）只有 **14 个码**有中文文案，
其余走 default: return code（errors.go:125）。

### 5.2 后端返回明确错误但前端无提示（候选）

1. **fanout 的每台机器错误被压成一句话，原始错误串丢弃**（sync.go:470,486,488,498）。
   常量在 sync.go:65-76：msgAdapterListRead / msgAgentTimeout(75) /
   msgAgentUnreachable(74) / msgAgentNotApplied(73)。
   sync.go:482-489 里 result.Error = name + msgAgentUnreachable，**不带 err.Error()**。
   -> 后端知道具体原因（如 adapter 未装、exec 失败），前端只看到"没有连上，这次没能更新它。"
   这是最实的候选。
2. **skipped 机器只有 Reason，HTTP 仍是 200**（sync.go:259-264，
   allOK 只统计 !OK && !Skipped）。
   -> 全部机器都因停用被 skip 时，响应是 200 ok:true status:synced，
   前端显示"同步成功"，实际一台都没同步。
3. writeError 的 details 常为 nil -> []（errors.go:35-37），
   例如 not-enrolled（handlers.go:1023）前端只拿到 message。
4. **关于 errorMessage default 回吐 code 串**：理论受害路径是 writeErrorValue
   （errors.go:45-53）。但 writeErrorValue 只在 agent 链路调用
   （handlers.go:520,531,568,627,650,706），其 code 全来自 mapDispatcherError
   （dispatcher.go:460-490），只产出 bad-request / agent-timeout /
   agent-unreachable / agent-offline / agent-not-found / agent-outdated
   ——**全部在 errorMessage 里有中文文案**。-> **此项不是缺陷，别误报。**

### 5.3 后端吞掉错误返回 200 / 成功态

1. **writeRemoteWriteReport（handlers.go:812-825）**：agent 返回
   ok:true 但 status:conflicts-remain -> reportHTTPStatus(true, ...) -> **200**。
   -> **有冲突未解决，HTTP 200**。前端必须自己读 status 字段。
2. **GET /api/status 对"未初始化"返回 200**（handlers.go:263-271），
   错误串塞进 report.Errors。注释（handlers.go:260-262）说明是有意的
   （避免 409->502 让控制台打不开状态抽屉）。-> **设计意图，别报。**
3. **GET /api/agents/nope/adapters -> 200 空列表**（见 1.2 第 3 条）。
4. **syncCollectFromMachine 的 no-drift 分支**（sync.go:193-203）：
   payload.Status == no-drift 时即使未前进 generation 也保留 synced（sync.go:197-199）。
5. **GET /api/storage/file 对二进制/AppleDouble 返回 200
   {"binary":true,"content":""}**（sync.go:1078,1088）。是降级还是吞错，看前端用法，待验证。
6. **noteWriteOutcome 解析失败静默 return**（dispatcher.go:281-283）。
7. **心跳畸形静默丢弃 + 节流日志**（agenthub.go:384-387）。
8. **wsconn.Accept 失败只记日志、不写 HTTP 响应**（agenthub.go:157-161）
   -> 连接被关闭，客户端可能看到空响应而非明确 4xx/5xx。

---

## 6. 密钥 / 敏感数据路径

| 端点 | 返回什么 | 鉴权要求 | 风险 |
|---|---|---|---|
| GET /api/auth/join（password.go:372）| **hub token 明文**（无 Enrollment 时，password.go:502-503）或**新铸 hr_ 码**（password.go:505-509）| authorized()（password.go:376）| **hr_ 码可自我增殖**（2.4）|
| GET /api/auth/join/status?code=hr_x（password.go:386）| active: bool | authorized() | 可**探测任意已知 hr_ 码是否有效** |
| **/agent/v1/stream hello 响应** | **WelcomeResult.AgentSecret 明文** | 三通道任一（hub/auth.go:135）| **hr_ 码 -> 换出永久 per-agent secret**（设计如此）。hr_ 码泄露 = 永久凭证泄露 |
| GET /api/keys（keys.go:28）| keyring list 元数据，**私钥不出** | 门（handlers.go:83）| 实测 {"ok":true,"status":"listed"}，无私钥 |
| POST /api/keys/{encrypt,unlock,password,op}（keys.go:34-62）| 操作结果 | 门 | **body 携带 password**（keyring.Command），明文过 HTTP（除非 TLS）|
| POST /api/agents/<id>/keys（keys.go:100）| 转发到 agent | 门 | 同上；注释 keys.go:215-217 说明 password 不回传 |
| GET /api/snapshot（sync.go:653）| **中心 store 全量明文**（所有机器配置文件内容）| 门（含 hr_ 码）| store 里若有 token 则全量泄露；有 secretscan 闸门（README.md:501）|
| POST /api/snapshot（sync.go:597）| {ok, generation} | 门 | 32MiB 上限（sync.go:595）|
| GET /dl/homer、/dl/homer.gz（handlers.go:929）| **hub 自身可执行文件本体** | 门（含 hr_ 码，设计意图）| 二进制可被反编译；支持 Range（handlers.go:995,1000）|
| GET /api/health（handlers.go:248）| version + **homerHome 绝对路径** | **无鉴权** | 路径泄漏（实测 /root/.homer）|
| GET /api/storage/file（sync.go:1058）| store 内单文件内容 | 门 | **traversal 防护**：filepath.Clean("/"+path) + .. 检查（sync.go:1072-1076）。但 **adapter 参数未做同类校验**——filepath.Join(head.StoreDir, adapterID, ...)（sync.go:1081）。实测 adapter=.. 返回"服务器还没有存储内容"（因无 generation），**未实机验证有 generation 时** |
| GET /api/agents（handlers.go:405）| 机器 host 快照：**MAC、IP、CPU 型号、内核、磁盘** | 门（含 hr_ 码）| **hr_ 码可读取整队机器网络拓扑**（实测 200，含 mac:6e:cd:ec:b2:86:f5、addrs:172.21.0.4/16）|
| GET /api/console（sync.go:816）| 全量控制台数据 | 门（含 hr_ 码）| 同上 |
| POST /api/agents/revoke、/api/agents/remove | — | 门（含 hr_ 码）| **hr_ 码可逐台吊销/删除机器**（实测 remove 已过门，仅目标不存在返回 404）|
| keys/agent-secrets.json | **sha256 哈希**，0600 | 文件权限 | 只存哈希（enrollment.go:132），明文永不落盘（enrollment_test.go:119-121 断言）|
| keys/hub-token | hub token 明文 | 0600（hubtoken.go:120）| — |
| /install.sh | 脚本 | **公开** | 不含 token（installscript.go:11-13）|

**潜在越权可读点排序（最实 -> 最弱）**：
1. **hr_ 一次性码 = 全权 hub token**（2.4），能自我增殖、24h、不 burn、可删机器。
2. /api/health 无鉴权泄漏 homerHome 绝对路径。
3. /api/storage/file 的 adapter 参数缺 traversal 校验（需先有 generation 才能验证）。
4. /agent/v1/{enroll,register,poll,report} 无鉴权返回 410（无害，但是无鉴权面）。

---

## 7. 单测断言为设计意图的行为（避免误报）

### internal/hub/*_test.go

| 行为 | 测试:行 |
|---|---|
| hello 必须 burn 掉 hr_ 码 | agenthub_test.go:210-212 |
| 用 secret 重连**不再**收到新 secret | agenthub_test.go:471-473 |
| 用 hub token hello **不**发 per-agent secret | agenthub_test.go:477-479 |
| secret 与 hello.agentId 不符 -> agent-id-mismatch 且不注册伪身份 | agenthub_test.go:495-500 |
| revoke 立即踢掉在线 agent，registry 转 stale | agenthub_test.go:504-523 |
| 同 agentID 重连顶替旧 session | agenthub_test.go:526 (TestAgentHubSupersedesPreviousSession) |
| 欢迎帧带 hub 协商的 ping/maxFrame | agenthub_test.go:201-206 |
| 心跳紧接 welcome（registry 可见前）不丢 | agenthub_test.go:220-226 |
| 缺/错 bearer -> 401 unauthorized | agenthub_test.go:247-248 |
| 缺 subprotocol -> 400 unsupported-version | agenthub_test.go:282-284 |
| 4 个 legacy 端点 -> 410（**任何方法**）| agenthub_test.go:290-316 |
| ResponseWriter 不实现 Hijacker -> 500 | agenthub_test.go:317 |
| hello 超时 -> 关闭 | agenthub_test.go:348 |
| 首帧非 hello -> 关闭 | agenthub_test.go:354 |
| hello 缺 ID -> CloseProtocol | agenthub_test.go:381 |
| proto 不符 -> CloseProtocol | agenthub_test.go:405 |
| 重绑同一 agentID **替换**旧 secret | enrollment_test.go:91-98 |
| 绑定跨 hub 重启存活 + 明文不落盘 + 0600 | enrollment_test.go:100-136 |
| revoke 跨重启存活 | enrollment_test.go:129-135 |
| 32 并发 Bind/Verify/Revoke 无竞态 | enrollment_test.go:138-160 |
| session 关闭错误 -> agent-offline 503，且**保留 close reason** | dispatcher_test.go:256,276-278 |
| deadline/cancel -> agent-timeout 504 | dispatcher_test.go:257-258 |
| ExecFailed/Internal/UnknownMethod/Overloaded/unsupported 全折成 agent-unreachable **502** | dispatcher_test.go:261-267 |
| 远程错误消息必须透传 | dispatcher_test.go:279-281 |
| per-agent 并发上限 64 | dispatcher_test.go:286 |
| **排队等信号量期间 session detach -> 第二次 liveness 检查返回 agent-offline 503**（不是发到死 session）| dispatcher_offline_queue_test.go:79-96 |
| RemoveAgent 会 Kick（CloseRemoved）| dispatcher_test.go:420 |
| 未上报 method 的 agent -> agent-outdated **409** | dispatcher_test.go:530 |
| **离线判定优先于 outdated**（先 503，不是 409）| dispatcher_test.go:543 |
| call budget：pull 12m / upgrade 3m / tool 7m / 默认 60s | task_test.go:12 |
| stale 由 AgentStaleAfter 现算（Get/List）| registry_test.go:74 |
| Attach 保留旧 Host/Tools/Drift/Version（缺失则沿用）| registry_test.go:22 ; host_test.go:47 |
| **caps 不跨重连继承** | registry.go:105-108 注释 |
| Tools nil vs [] 语义不同（nil=还没上报，[]=装了 0 个）| tools_test.go:20,63 ; registry_test.go:237 |
| host 字段裁剪（OS 32 / Distro 80 / Mount 128 / MAC 32 / addr 80）| host.go:71-134 |
| tool 文本裁剪 64 | hub/tools.go:10,30-40 |
| TaskOptions 零值省略 resolution 字段 | task_zero_test.go:11 |
| AgentDrift 零值省略 resolutions | registry_zero_test.go:10 |

### internal/web/*_test.go

| 行为 | 测试:行 |
|---|---|
| **闸门矩阵：hr_live（未兑换码）对 /api/* 与 /dl/* 一律放行** | gate_matrix_test.go:57, 287-297 gateAllowsHumanAPI |
| **/api/* 被标注为 KNOWN-BROAD**（作者自己知道门太宽）| gate_matrix_test.go:108 |
| hr_ 码可进 /agent/v1/stream（endpoint 自行鉴权）| gate_matrix_test.go:71-80 |
| hr_burned / 已吊销 secret -> 401 | gate_matrix_test.go:58,60 |
| 4 个 legacy tombstone -> **任何凭证都是 410** | gate_matrix_test.go:83-101 |
| /install.sh -> 任何凭证（含无）都 200 | gate_matrix_test.go:229-232 |
| /dl/homer 需鉴权（206 表示过了门）| gate_matrix_test.go:209-227 |
| 非回环无密码拒绝启动 | auth_test.go:46 |
| 回环无密码仍上锁（401）| auth_test.go:54 |
| setup 受网络限制（公网需 code）| auth_test.go:72 |
| 密码策略 >=12 runes、<=72 bytes | auth_test.go:103 ; password.go:484-489 |
| 登录失败退避（下次直接拒，约 2s）| auth_test.go:237 |
| join 命令需鉴权 | auth_test.go:265 |
| join/status 在码用掉后转 inactive | auth_test.go:291 |
| 删密码文件 -> session 失效 | auth_test.go:340 |
| 换密码文件 -> session 保留 | auth_test.go:366 |
| session 跨重启存活 | auth_test.go:383 |
| session 过半滑动续期 | auth_test.go:397 |
| 过期 / 篡改 session 拒绝 | auth_test.go:426,441 |
| 弱密码**不**烧 setup code | auth_test.go:598 |
| 显式停用 adapter 的 pull -> **400 adapter-disabled-for-agent** | agent_overrides_test.go:249 |
| 默认 scope 也过滤停用项 | agent_overrides_test.go:450 |
| fanout 全停用 -> skip + reason | agent_overrides_test.go:280 |
| 幽灵机器（POST adapter override）-> 404 | agent_overrides.go:273-285 |
| keyring/ssh-key 永不出现在 adapter 列表 | agent_overrides.go:230-238 注释 |
| 插件未接 -> install/uninstall **503**，GET -> 200 空表 | plugins_test.go:20 ; api-reference.md:163-164 |
| agent-outdated -> 409 + 中文消息 | errors_test.go:8-14 |
| hub 版本 dev 不算落后于 release agent | handlers_version_test.go:10-30 |
| snapshot ETag | choices_stream_test.go:274 ; snapshot_etag_reset_test.go |
| fanout 并发有界（8）| choices_stream_test.go:324 |
| 下发带密钥必须先解锁，否则 422 unlock-required | keys_test.go:59,262 |
| 冲突保留本地时**仍要**解锁 | keys_test.go:367,415 |
| 无 applied writes 时**不**触发解锁 | keys_test.go:380 |
| 预览（未 confirm）**不**因绑定密钥被拒 | keys_test.go:237 |

---

## 8. 待验证问题

1. **/api/storage/file 的 adapter 参数 traversal**：当前 hub 无 generation
   （/api/snapshot 409），无法触发。有快照后 adapter=../../keys 是否可读
   keys/hub-token？sync.go:1081 只 join 未校验。**需先 collect 造出 generation 再验证。**
2. **/ws 端点是否存在**：全仓无 "/ws 命中；agent 走
   /agent/v1/stream（hub/agenthub.go:23）。**任务描述里的 /ws 在本 HEAD 不存在。**
   （qa/README.md 的 ws://127.0.0.1:3000 是 browserless CDP，不是 hub。）
3. **hr_ 码能否进 /agent/v1/stream 铸出永久 secret**：gate_matrix_test.go:71-80 断言可以。
   实机未做（需 WebSocket 客户端）。是 2.4 的加强版。
4. **/api/agents/<id>/adapters GET 对幽灵机器返回 200**：实机确认。
   POST 有幽灵防御、GET 没有。是否缺陷取决于产品意图，**待契约确认**。
5. **writeMutex 长阻塞的可观测后果**：13min WriteTimeout vs 12min budget，未压测。
6. **fanout（无 writeMutex）与单机器 pull（有 writeMutex）的 generation 交错**，未验证。
7. **/api/sync?direction=dispatch 全 skip 时返回 200 ok:true synced**：
   代码读出来是这样（sync.go:259-264），未实机验证（需停用全部 adapter 的机器）。
8. **hub 重启后未兑换 hr_ 码失效**（codes 只在内存，enrollment.go:30-31），
   未实机验证（重启会打断 QA 环境）。
9. docs/api-reference.md 缺 /api/plugins/install 的 404 分支——文档缺失还是实现偏离，待确认。
10. CallBudget（proto.go:62-72）与 PullWait/UpgradeWait/ToolUpgradeWait
    （task.go:77-79）是**两套重复常量**，值相同，是否有漂移风险未验证。

---

## 9. 环境事实（供猎人直接用）

- hub 控制台（宿主）http://127.0.0.1:17760；容器内 http://hub:7760。
- Bearer qa-hub-token；管理员密码 qa-admin-password（已预置，
  /api/auth/status -> configured:true, setupCodeRequired:false）。
- 当前 **1 台在线机器**：agentId=e363efdcf085-7b6d、hostname=e363efdcf085、
  version=dev、stale=false，host=linux/amd64 Alpine Linux v3.20、
  kernel 6.1.0-53-amd64、mac=6e:cd:ec:b2:86:f5、addrs=["172.21.0.4/16"]、
  cpu 8 核 AMD Ryzen 5 3400G、memory 16GB。
  drift.error = 未找到 homer 配置: /root/.homer/homer.json（**hub 机器自身未 init**）。
  tools=[pi]，error=读不出版本：输出里没有版本号。
- **中心无 generation**：GET /api/snapshot -> 409 no-snapshot；
  /api/console -> published:false, generation:0。
  -> 所有依赖快照的下发路径（dispatch、/api/storage/*）**都停在 409**，
  猎人要测这些路径**必须先 collect 一次**。
- /api/keys -> {"ok":true,"status":"listed"}（无密钥）。
- 铸码：curl -H "Authorization: Bearer qa-hub-token" http://127.0.0.1:17760/api/auth/join
  -> {"code":"hr_<48hex>","command":"curl -fsSL .../install.sh | sh -s -- --token hr_<48hex>","ok":true}。
- 3 个 idle box（homer-qa-box-a/b/c）未见在 /api/agents 中注册（列表只有 1 台）。

---

verdict:

```json
{"ok": true, "reason": "recon complete: 9 sections, ~130 cited facts, 10 open questions; strongest finding: web/auth.go:62 lets an un-redeemed hr_ code act as a full hub token (contradicting hub/auth.go:49-55), confirmed live on /api/agents,/api/console,/api/keys,/dl/homer.gz plus code self-multiplication via /api/auth/join"}
```
