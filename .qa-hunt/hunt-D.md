# 狩猎角度 D · agent 生命周期 / 下发链路 / 状态卡死

仓库 `/root/code/homer`，HEAD `010521a`。时间窗口 2026-10-16 12:58Z ~ 13:30Z。
产物目录 `.qa-hunt/D/`。角度边界：**只做**生命周期与下发链路（断线/重连/重复接入/半开、
任务卡中间态、下发后状态不一致）；不做鉴权矩阵、不做密钥、不做文档一致性。

## 基建

- `docker ps`：hub + box-a/b/c + browser 全 Up（PASS）
- `curl /api/agents`（Bearer qa-hub-token）→ 200，返回机器列表（PASS）
- CDP `http://127.0.0.1:3000` 可连、可登录、可 eval（PASS）
- 未跑 `make qa-restart`（hub 状态共享，其他 hunter 在用，避免互相打断）

⚠️ **环境是共享的**：狩猎期间观察到至少 1 位其他 hunter 在 `172.21.0.1` 上跑 agent，
且曾**复用我的 agentID 顶替我的 session**（hub 日志
`agent stream superseding prior connection agent="54d0c3373428-62af" remote="172.21.0.1:36206"`）。
我作废了那次被污染的数据（`.qa-hunt/D/hopen2.log`），改用**独占 agentID + 独占 home**
重做（`.qa-hunt/D/hopen3.log`）。所有 candidate 都基于独占复现。

---

## Candidates

### C1 【高】一台半开（假死）机器把全局写锁占住 ~60s，其他**健康**机器的下发被整体冻结

- 疑似位置: `internal/web/handlers.go:483`（`writeMutex.Lock()` 包住 `handleAgentPull`）
  + `internal/web/server.go:242-245`（注释自陈 package-global、无超时、无 try-lock）
  + `internal/stream/session.go:16`（`DefaultPingTimeout=75s`：session 要等这么久才 done）
  + `internal/hub/dispatcher.go:373`（`session.Call` 一直等到 session done 才返回 err）
- **detector**:
  - 后端在线状态：`curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents`
    取该 agent 的 `stale` 字段（`stale` 由 `agentIsStale` 现算，非缓存）
  - 阻塞测度（核心）：`curl -s -o /dev/null -w 'http=%{http_code} t=%{time_total}' -X POST
    -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}'
    http://127.0.0.1:17760/api/agents/<健康机>/pull`
    —— **`t` 就是被 writeMutex 卡住的秒数**
  - 半开判定（进程被 SIGSTOP、TCP 仍 ESTABLISHED）：
    `docker compose -f qa/env/docker-compose.yml exec -T box-c sh -c 'awk "{print \$3}" /proc/<pid>/stat'`
    → 输出 `T` 表示进程已停止而 socket 未关
- 前置状态探测: 冻结前确认目标 `54d0c3373428-4323` 的 `stale=False age≈17s`（在线）；
  冻结后确认 `/proc/617/stat` = `T`；再对健康机 `b6675d68ddf7-7920` 打一次基线 pull，
  确认 `t=0.008s`（未阻塞）**之后**才发冻结机的 pull。
- 证据: `.qa-hunt/D/mutex.log`、`.qa-hunt/D/mutex2.log`（带 `?confirm=true`）、
  `.qa-hunt/D/mutex3.log`、`.qa-hunt/D/hold.log`、`.qa-hunt/D/hold3.log`；
  半开时序 `.qa-hunt/D/hopen3.log`
- **实机时间戳（原文摘）**
  ```
  mutex.log:
  2026-10-16T13:14:35Z BASELINE live pull: http=409 t=0.008387      <- 冻结前，健康机 8ms
  2026-10-16T13:14:35Z ISSUE pull to FROZEN 54d0c3373428-4323 (background)
  2026-10-16T13:15:36Z NOW live pull (should block if mutex held): http=409 t=58.995398
  2026-10-16T13:15:36Z NOW keys (also takes writeMutex): http=200 t=0.000670
  2026-10-16T13:15:36Z DEAD RESULT: {"error":{"code":"agent-offline","message":"机器已离线",
    "details":["agent: agent-offline: agent \"54d0c3373428-4323\" stream closed:
     stream: connection closed (code 4000): ping timeout"]}} DEAD_HTTP=503 DEAD_T=60.990877

  mutex2.log (confirm=true):
  2026-10-16T13:15:54Z BASELINE live: http=422 t=0.008173
  2026-10-16T13:15:56Z BLOCKED live:          <- 100s curl 超时都未返回（空）
  2026-10-16T13:17:36Z BLOCKED keys: http=200 t=0.001336   <- 锁一释放立刻返回
  2026-10-16T13:17:55Z DEAD: ... DEAD_HTTP=503 DEAD_T=58.382081

  mutex3.log / hold.log / hold3.log:
  hold_http=503 hold_t=62.561076
  hold_http=503 hold_t=66.023527
  hold_http=503 hold_t=72.864880
  ```
  **5 次独立复现**，健康机被卡 `58.99 / >100 / 62.56 / 66.02 / 72.86` 秒；
  同进程的 `/api/keys`（`keys.go:79` 也取 writeMutex）在锁释放瞬间才返回。
- 复现步骤（可复制粘贴）:
  ```sh
  # 1) 起一台独占 agent（box-c，私有 home，避免被别人顶替）
  C=$(curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/auth/join \
      | python3 -c "import json,sys;print(json.load(sys.stdin)['code'])")
  docker compose -f qa/env/docker-compose.yml exec -T box-c sh -c \
    "mkdir -p /root/.hhuntD && nohup /root/.local/bin/homer agent --hub http://hub:7760 --token $C --home /root/.hhuntD >> /root/.hhuntD/agent.log 2>&1 &"
  # 2) 基线：健康机 pull 耗时
  curl -s -o /dev/null -w 't=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' \
    -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/b6675d68ddf7-7920/pull?confirm=true"
  # 3) 制造半开：SIGSTOP（TCP 不断，对端不再回 pong）
  docker compose -f qa/env/docker-compose.yml exec -T box-c sh -c 'kill -STOP <pid>'
  # 4) 对假死机下发（后台），2 秒后再对健康机下发
  ( curl -s -w 'DEAD_T=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' \
      -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/54d0c3373428-4323/pull?confirm=true" ) &
  sleep 2
  curl -s -o /dev/null -w 'BLOCKED t=%{time_total}\n' -X POST -H 'Authorization: Bearer qa-hub-token' \
      -H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:17760/api/agents/b6675d68ddf7-7920/pull?confirm=true"
  # 5) 解冻
  docker compose -f qa/env/docker-compose.yml exec -T box-c sh -c 'kill -CONT <pid>'
  ```
- 我未排除的伪影:
  1. `curl` 的 `time_total` 含连接建立时间——但基线 8ms vs 阻塞 59~73s 差 4 个数量级。
  2. 被卡的 pull 最终返回 **503 `agent-offline`**（不是超时错误），说明卡住的原因是
     "等半开 session done"，而不是 agent 真执行了 60s。锁是被**等超时**这件事占住的。
  3. `mutex.log` 那次基线/阻塞对用的是**不带 confirm** 的 pull（409 早早 return）。
     `handlers.go:483` 的取锁在 `confirmValue` 判断**之前**，所以 409 路径同样会卡——
     这加重了情节，但我不能排除"409 路径本不该取锁"是独立的设计争议。
  4. 共享环境：冻结期间别的 hunter 可能也在打 API。但 5 次复现量级一致，且每次都精确
     等于「假死机 pull 的 DEAD_T」，因果链闭合。
  5. **UI 侧我没拿到直接证据**：目标机器没有「下发」按钮（无 drift），对话框起不来。
     只观测到 `#refreshed-at` 仍在更新（30s 轮询不受锁影响）。"用户看到卡死"是推断。
- 建议修复: 给 `writeMutex` 加 try-lock + 超时（或改 per-agent 锁）；`handleAgentPull`
  在**取锁前**先做一次 `requireOnline`，把明显不活的 session 挡在锁外；
  `PingTimeout` 与 `AgentStaleAfter` 对齐并缩短。

---

### C2 【中】同 hostname 重复接入不去重：一台机器变多行，且各行**按钮集合不同**

- 疑似位置: `internal/hub/registry.go:91`（`Attach` 只按 `AgentID` 查，不看 hostname）
  + `internal/hub/registry.go:321-323`（列表按 AgentID 排序）
  + AgentID 生成 = `hostname-<8hex>`，每次 re-enroll 换后缀
  + re-enroll 路径 `internal/web/installscript.go:59-70`（备份 agent.json）
- **detector**（UI 侧；hostname 用 `.hostname` 文本，agentId 用 `data-agent-id` 属性）:
  ```js
  [...document.querySelectorAll('.agent-card[data-agent-id]')]
    .map(c=>({id:c.dataset.agentId, host:c.querySelector('.hostname')?.textContent,
              stale:c.classList.contains('stale'), ip:c.querySelector('.ip')?.textContent}))
  ```
  已确认这些符号在目标组件外不出现（grep 依据）：
  `grep -n 'agent-card\|data-agent-id\|"hostname"' internal/web/static/index.html`
  → 只在 `renderAgents`（1885/1890 行）生成。**未使用** `┃ │ ╭ ╰ ─` 等通用边框字符。
- 前置状态探测: 接入前确认 box-c 只有 1 行；接入后立即确认变成 2 行，且两行
  `ip` 相同（`172.21.0.3`）、`host` 相同（`54d0c3373428`）、只有 `id` 后缀不同。
- 证据: `.qa-hunt/D/duplicates.txt`
- **实机证据（13:08Z / 13:11Z）**
  ```json
  [{"id":"54d0c3373428-b745","host":"54d0c3373428","stale":false,"ip":"172.21.0.3"},
   {"id":"54d0c3373428-bc77","host":"54d0c3373428","stale":false,"ip":"172.21.0.3"},
   {"id":"b6675d68ddf7-7920","host":"b6675d68ddf7","stale":false,"ip":"172.21.0.6"},
   {"id":"e363efdcf085-1563","host":"e363efdcf085","stale":false,"ip":"172.21.0.4"},
   {"id":"e363efdcf085-7b6d","host":"e363efdcf085","stale":true,"ip":"172.21.0.4"}]
  ```
  同一物理机 `54d0c3373428`（同一 IP `172.21.0.3`）出现**两行**。按钮集合（13:11Z）：
  ```
  54d0c3373428-4323 (live):  查看|收取|下发|详情|密钥|登录公钥|吊销|移除
  54d0c3373428-b745 (stale): 查看(D)|详情|密钥(D)|登录公钥(D)|吊销|移除
  54d0c3373428-bc77 (live):  查看|详情|密钥|登录公钥|吊销|移除
  ```
  → 同一 hostname 下 3 行，其中一行能「下发」、另外两行不能，用户无法判断该操作哪台。
  install 脚本输出（实机）：
  ```
  >> 检测到旧的 agent.json，已改名为 agent.json.before-reenroll，将用新接入码重新接入
  >> agent 已在后台启动（日志: /root/.homer/agent.log，PID: 534）
  ```
- 复现步骤:
  ```sh
  C=$(curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/auth/join \
      | python3 -c "import json,sys;print(json.load(sys.stdin)['code'])")
  docker compose -f qa/env/docker-compose.yml exec -T box-c sh -c \
    "curl -fsSL http://hub:7760/install.sh | sh -s -- --token $C"
  sleep 5
  curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents | python3 -m json.tool
  # -> 两条 hostname=54d0c3373428、agentId 后缀不同的记录
  ```
- 我未排除的伪影:
  1. `e363efdcf085` 的两行**不是我造的**——狩猎开始时（12:59Z）就已存在
     （`-7b6d` + `-c78a`），是别的 hunter 重复接入留下的。我只**亲手**造了
     `54d0c3373428` 的重复（b745 + bc77）；`e363efdcf085` 仅作为同类现象佐证。
  2. 「同 hostname = 同一台机器」是我的推断（依据同 IP `172.21.0.3`）。
     若产品刻意允许多身份接入（多 home/多 worktree），则"不去重"本身是设计意图——
     但**即便如此，UI 用 hostname 唯一标识却不给区分标记**仍是问题。
  3. re-enroll 是真实用户路径（重装/换码），非臆造。
- 建议修复: `Attach` 时按 `(hostname, agentID)` 做去重提示；UI 对同 hostname 多行加
  「同一台机器的多个身份」标记并显示 agentId 后缀；re-enroll 优先复用旧 agentID。

---

### C3 【中】半开连接下离线判定要 ~75s；`AgentStaleAfter=90s` 那条分支从未成为决定因素

- 疑似位置: `internal/hub/registry.go:341-343`（`agentIsStale` 的 90s 分支）
  vs `internal/stream/session.go:16`（`DefaultPingTimeout=75s`，先到先判定）
- **detector**:
  ```sh
  curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents \
   | python3 -c "import json,sys,datetime;d=json.load(sys.stdin);now=datetime.datetime.now(datetime.timezone.utc)
   [print(a['agentId'],a['stale'],(now-datetime.datetime.fromisoformat(a['lastSeen'].replace('Z','+00:00'))).total_seconds()) for a in d['agents']]"
  ```
  `stale` 后端现算、非缓存；`lastSeen` 单调递增可证明**期间没有重连污染**。
- 前置状态探测: 冻结前 `stale=False age=17.1s`；`/proc/617/stat` = `T`。
- 证据: `.qa-hunt/D/hopen3.log`（27 个采样点，5s 间隔）
- **实机曲线（摘）**
  ```
  2026-10-16T13:10:54Z PRE      stale=False age=17.1s
  2026-10-16T13:10:54Z STOP
  2026-10-16T13:11:49Z T+55s    stale=False age=72.8s   <- 仍显示"在线"
  2026-10-16T13:11:54Z T+60s    stale=True  age=77.9s   <- 翻转点
  2026-10-16T13:12:55Z T+120s   stale=True  age=138.5s
  2026-10-16T13:13:00Z T+125s   stale=True  age=143.5s
  ```
  `age` 单调（`72.8 → 77.9 → … → 143.5`）证明期间**没有重连**，`stale` 翻转是真判定。
  翻转发生在 `age≈77.9s`，即最后一次收帧后约 **75s** = `DefaultPingTimeout`；
  `AgentStaleAfter=90s` 分支（《scout-backend》§3.3 推测的"最坏 ~90s"）**本次实验中从未生效**。
- 可观测后果（按判据纪律，不报"常量不一致"这个抽象问题）:
  **一台已假死的机器在 UI 上继续显示"在线"约 75 秒**，这 75 秒里它的
  「下发 / 查看 / 密钥 / 登录公钥」按钮全部可点（见 C2 按钮采样：live 行有「下发」）。
  点「下发」的后果即 C1：整队机器被冻结 60s+。
- 我未排除的伪影:
  1. **box 内没有 iptables**（`sh: iptables: not found`），只能用 `kill -STOP` 模拟半开。
     SIGSTOP 下内核不发 RST、socket 保持 ESTABLISHED，我认为等价，但严格说这是
     **"进程冻结"而非"网络分区"**。没用 `docker pause`（避免忘解冻），故无 unpause 事项。
  2. 采样间隔 5s，翻转点精度 ±5s（75 vs 90 差 15s，仍能区分）。
  3. 第一次实验 `.qa-hunt/D/hopen2.log` 在 T+35s 翻转且 `age` 被重置到 2.1s ——
     那是**被别的 hunter 顶替**的污染（hub 日志有 `superseding prior connection`），
     已识别并作废，改用独占 agent 重做。
- 建议修复: 把 `AgentStaleAfter` 降到 ≤ `PingTimeout`，或让 `agentIsStale` 直接复用
  session liveness；UI 对"超过 N 秒没心跳但还没判离线"的机器给中间态（灰点 + "无响应"）。

---

### C4 【中】revoke 后机器**仍留在列表**（只是变灰），UI 上"吊销成功"与"机器掉线"完全不可区分

- 疑似位置: `internal/web/handlers.go:1003-1005`（注释自陈"保留 registry 行"）
  + `internal/hub/agenthub.go:89-93`（`SetRevokeHook` → `Kick(CloseRevoked)`）
- **detector**:
  - API：`POST /api/agents/revoke` 返回体 + `/api/agents` 里该 agentId 是否仍在
  - UI：`[...document.querySelectorAll('.agent-card[data-agent-id]')].map(c=>c.dataset.agentId)`
    + `.classList.contains('stale')` + 各按钮 `disabled`
  - agent 真被踢（hub 侧）：`docker logs homer-qa-hub` 中
    `kicking agent stream agent="..." code=4401 reason="agent secret revoked"`
  - agent 侧：`/root/.homer/agent.log` 中 `agentSecret 已吊销，请重新接入`
- 前置状态探测: revoke 前确认 `54d0c3373428-b745` 存在且 `stale=False`。
- 证据: `.qa-hunt/D/revoke.txt`
- **实机证据（13:11Z）**
  ```
  --- before ---      [('54d0c3373428-b745', False)]
  --- revoke ---      {"ok":true}
  --- after 4s ---    [('54d0c3373428-b745', True)]     <- 行还在，只是 stale 翻了

  hub 日志: hub: kicking agent stream agent="54d0c3373428-b745" code=4401 reason="agent secret revoked"
            hub: agent stream disconnected ...: stream: connection closed (code 4401): agent secret revoked
  box-c agent.log: 13:11:12 与 hub 的连接已断开...：stream: connection closed (code 4401): agent secret revoked
                   13:11:12 agentSecret 已吊销，请重新接入（5 分钟后重试）

  UI 采样: {"id":"54d0c3373428-b745","host":"54d0c3373428","stale":true,"cls":"agent-card stale",
            "btns":["查看(D)","详情","密钥(D)","登录公钥(D)","吊销","移除"]}

  第二次 revoke: {"error":{"code":"not-enrolled","message":"该机器没有有效凭证（可能从未接入或已吊销）","details":[]}}
  ```
  **关键点**：吊销后 UI 呈现与"这台机器断线了"**完全一样**（同一个 `.agent-card.stale`
  灰化样式、同一套 disabled 按钮）。用户看到灰卡片 → 以为吊销没成功 → 再点「吊销」→
  弹「该机器没有有效凭证」→ 进一步强化"吊销失败"的错觉。
- 复现步骤:
  ```sh
  curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents | python3 -m json.tool
  curl -s -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' \
       -d '{"agentId":"<在线agentId>"}' http://127.0.0.1:17760/api/agents/revoke     # {"ok":true}
  sleep 4
  curl -s -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents | python3 -m json.tool  # 行仍在，stale=true
  curl -s -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' \
       -d '{"agentId":"<同一个>"}' http://127.0.0.1:17760/api/agents/revoke     # 409 not-enrolled
  ```
- 我未排除的伪影:
  1. **保留行是设计意图**（`handlers.go:1003-1005` 注释 + scout-backend §3.7 明列）。
     我报的不是"保留行"，而是**UI 没有任何视觉/文案区分**——属 UI 呈现缺陷。
     请 qa-repro 按 UI 判据复核，不要按后端逻辑判 false。
  2. 我 revoke 的是自己刚造的重复身份 `54d0c3373428-b745`，事后已 `remove` 清理。
  3. 「用户会误解」是基于 UI 一致性的推断，**没有真人可用性测试**佐证。
- 建议修复: revoke 后给卡片独立状态标记（「已吊销 · 凭证失效」badge + 不同色点），
  「吊销」按钮置灰或改文案为「已吊销」；`not-enrolled` 文案区分"从未接入"与"已吊销过"。

---

### C5 【低】fanout 下发对离线机器静默 `skipped`（`ok:true`），且半开机器被误判为已跳过

- 疑似位置: `internal/web/sync.go:462-465`（`info.Stale → Skipped` 直接 continue，无 Reason）
  + `internal/web/sync.go:250-263`（`allOK` 只统计 `!OK && !Skipped`，skipped 不影响）
- **detector**: `POST /api/sync?direction=dispatch&confirm=true` 响应体里的
  `agents[].skipped` / `agents[].ok` / `status` 三个字段
- 前置状态探测: 中心已有 generation（实测 `GET /api/console` → `published:True gen 3`），
  否则停在 409；确认列表中存在 `stale:true` 机器。
- 证据: `.qa-hunt/D/fanout.txt`
- **实机证据（13:12Z）**
  ```
  POST /api/sync?direction=dispatch   body={"confirm":true}
  -> HTTP 409 {"ok":false,"status":"aborted","direction":"dispatch","agents":[],
               "errors":["没有确认，未做任何改动。"]}      <- confirm 必须放 query，不放 body

  POST /api/sync?direction=dispatch&confirm=true
  -> HTTP 422
  {"ok":false,"status":"partial","direction":"dispatch","agents":[
   {"agentId":"54d0c3373428-4323","hostname":"54d0c3373428","ok":true,"skipped":true},
   {"agentId":"54d0c3373428-b745","hostname":"54d0c3373428","ok":true,"skipped":true},
   {"agentId":"54d0c3373428-bc77","hostname":"54d0c3373428","ok":false,"skipped":false,
    "status":"conflicts-remain","error":"54d0c3373428没有应用这次同步。"},
   {"agentId":"b6675d68ddf7-7920","hostname":"b6675d68ddf7","ok":false,"skipped":false,
    "status":"conflicts-remain","error":"b6675d68ddf7没有应用这次同步。"},
   {"agentId":"e363efdcf085-1563","hostname":"e363efdcf085","ok":false,"skipped":false,
    "status":"conflicts-remain","error":"e363efdcf085没有应用这次同步。"},
   {"agentId":"e363efdcf085-7b6d","hostname":"e363efdcf085","ok":true,"skipped":true}],
   "errors":null}
  ```
  `skipped:true` 的机器 `ok:true`，响应里**没有任何 `reason` 字段**说明它被跳过了。
  `errors:null` 但 HTTP 是 **422** —— 错误全在 `agents[].error`，前端若只读 `errors`
  会呈现"成功但无事发生"。
  ⚠️ 附带观察：`54d0c3373428-4323` 当时正被我 SIGSTOP 冻结（**半开，不是离线**），
  却被计为 `skipped`——fanout 只看 `info.Stale`，而 Stale 有 75s 延迟（C3），
  半开机器在"其实已死但还没判离线"的窗口里行为不可预期。
- 复现步骤:
  ```sh
  curl -s -X POST -H 'Authorization: Bearer qa-hub-token' -H 'Content-Type: application/json' -d '{}' \
    "http://127.0.0.1:17760/api/sync?direction=dispatch&confirm=true" | python3 -m json.tool
  # 观察 stale 机器条目的 ok / skipped 字段与顶层 errors
  ```
- 我未排除的伪影:
  1. `conflicts-remain` 是环境里真有冲突（其他 hunter 造成的），不是我造的。
  2. **没有验证前端实际渲染**（时间不够），只验证了后端给前端的数据形状。
     "UI 怎么说"这一半是推断。
  3. 单机器下发离线机器时后端给的是明确的 503 + 「机器已离线」中文文案（实测，见下），
     **那条路径没问题**——本条只针对 fanout 路径。
- 建议修复: `skipped` 机器填 `Reason`（"离线已跳过"），前端显式列出被跳过的机器；
  fanout 对半开窗口内的机器做一次 liveness 探测而非只看 `Stale`；
  HTTP 422 时不要把 `errors` 置 null。

---

### C6 【低·倾向判 false】已兑换的 hr_ 码在 Web API 通道的表现与既有侦察结论不一致

- 疑似位置: `internal/web/auth.go:62`（`ValidCode` 只 check 不 burn）
  vs `internal/hub/enrollment.go:103-121`（`Redeem` 才 delete）
- **detector**: 用**已兑换过**的码做 Bearer 打 `/api/agents`，看 HTTP 状态码
- 前置状态探测: 先确认该码确已被兑换——
  `GET /api/auth/join/status?code=<码>` → `{"active":false,"ok":true}`；
  且 agent 侧确认该码已用过（`WebSocket 401：凭证无效/接入码已用`）
- 证据: `.qa-hunt/D/code-reuse.txt`
- **实机证据（13:20Z）**
  ```
  重用 code1(hr_31f33f...) 起 agent：
  homer agent: 13:20:53 WebSocket 401：凭证无效/接入码已用:请到控制台重新生成接入码，
    并用 `homer agent --token <新码>` 重新接入（只改 keys/hub-token 不会生效，
    agent.json 里已有 secret 时它会被忽略）（5 分钟后重试）        <- agent 路径正确拒绝

  GET /api/auth/join/status?code=hr_9d19ccaa...(已兑换)   -> {"active":false,"ok":true}
  GET /api/agents  Bearer = 同一个已兑换码                  -> http=401
  ```
  **结果与 scout-backend §2.4 的实机结论（200）相反**：我这边是 **401**。
- 我的判定建议: **倾向判 false（伪影）**，列出来只为让 qa-repro 知道两边不一致、别直接采信任一边。
  可能原因：(a) scout 测的是"铸了但从未走 Redeem 的码"，我测的是"真被 agent 兑换过的码"，
  两者 `ValidCode` 语义可能不同；(b) 期间别的 hunter 做过 revoke/remove 影响状态。
- 我未排除的伪影: (a)(b) 都没能区分开；只测了 1 个已兑换码，样本不足。
- 建议修复: 若确认"已兑换码仍能进 /api"，应在 `ValidCode` 命中时同时检查是否已 Redeem 过。

---

## 已验证为**正常**、明确不报（避免误报）

- **同 agentID 重连顶替旧 session**：实机撞到过（hub 日志
  `agent stream superseding prior connection agent="54d0c3373428-62af" remote="172.21.0.1:36206"`），
  与 `agenthub_test.go:526` 一致 → 设计意图，**不报**。
- **离线机器单机器下发被正确拒绝**（`POST /api/agents/e363efdcf085-7b6d/pull` → 503）:
  ```
  {"error":{"code":"agent-offline","message":"机器已离线","details":["agent: agent-offline:
    机器 e363efdcf085-7b6d 离线（最后心跳 11m20s 前）。请等它重新上线或先在机器上重启 agent"]}}
  ```
  状态码与文案都对 → **不报**。
- **hr_ 码一次性（agent 接入路径）**：确认生效（401）→ **不报**。
- **revoke 立即踢在线 agent**：确认生效（4401 + agent 侧日志）→ **不报**。
- **revoke 未绑定过的机器 → 409 `not-enrolled`**：与 `handlers.go:1023` 一致 → **不报**。

---

## 未覆盖（预算内没跑到）

1. **hub 重启后 registry 全丢 / UI「机器消失」惊吓态**（怀疑点 3）：没做——
   重启会打断其他 hunter 的共享 hub，属我不该单方面做的事。需主 agent 协调时间窗。
2. **fanout 不持 writeMutex 与单机器 pull 的 generation 交错**（怀疑点 7）：没做——
   需并发构造 + 读 generation，时间不够。但 C1 已证明「一个持锁一个不持」的不对称真实存在，
   交错是合理怀疑，缺实机证据。
3. **`NoteWriteOutcome` 只在成功路径调用 → drift 停留旧值**（怀疑点 6）：没做——
   需构造"下发失败但机器在线"，环境里的失败都是 conflicts/离线；且 drift 有 30s
   轮询滞后（scout-ui 标注 e2e 容忍 75s），难与"陈旧"区分。
4. **C1 的 UI 侧实机证据**：目标机没有「下发」按钮（无 drift），对话框起不来；
   只拿到后端阻塞硬数据 + `#refreshed-at` 仍在更新。UI 冻结表现是推断。
5. **重复接入时 `ReportedAdapters` / caps 继承的副作用**（`registry.go:93-109`）：没测。
6. **无离线队列的"已加入队列"误导文案**（怀疑点 5 的 UI 半）：只验了后端数据形状，没看前端渲染。

---

## 环境

- 宿主 `/root/code/homer`，HEAD `010521a`；hub `http://127.0.0.1:17760`（容器内 `http://hub:7760`）
- Bearer `qa-hub-token`；管理员密码 `qa-admin-password`；CDP `http://127.0.0.1:3000`
- 主要用 **box-c**（hostname `54d0c3373428`）做假死/重复接入；
  box-a 未使用（其上有 STOPPED 进程 205/29/309，是别的 hunter 造成的，我未触碰）
- 半开手段：box 内**无 iptables**（`sh: iptables: not found`），改用
  `kill -STOP <agent pid>`（进程冻结、socket 保持 ESTABLISHED）。
  **全部已解冻**：收尾确认 box-c 目标 `/proc/617/stat` = `S`。
  **未使用 `docker pause`**，故无 unpause 事项。
- harness（全在 `.qa-hunt/D/`）：`cdp.js`（单次 eval）、`ui.js`（登录+批量 eval）、
  `uihang.js`/`uihang2.js`（UI 冻结观测）、`hopen2.sh`/`hopen3.sh`（半开时序）、
  `mutex.sh`/`mutex2.sh`/`hold.sh`/`hold3.sh`（写锁阻塞）、`uifrozen2.sh`
- 清理：我的两台探针 agent 已 `POST /api/agents/remove`；`/root/.hhuntD*` 已删除。
  **未执行 `make qa-restart`**（共享 hub，避免打断他人）。
- 工具：node v24.13.1、python3、docker compose（`qa/env/docker-compose.yml`）
