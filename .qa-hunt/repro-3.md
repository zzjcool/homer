# 独立复现判定 · 第 3 组（角度 C 的 C1/C2/C3/C5/C6 + 角度 D 的 C1~C5）

仓库 /root/code/homer，HEAD `010521a`。hub `http://127.0.0.1:17760`（Bearer `qa-hub-token`）。
判定人：qa-repro（**未复用任何 hunter 脚本**；本轮脚本全在 `.qa-hunt/repro3/`）。
时间窗 2026-10-16 13:38Z ~ 13:55Z。

## 判据总原则（本轮统一纪律）

hunter 的判据以 **curl `time_total`** 和 **UI 文案字符串** 为主。前者是"渲染/传输产物"
（会被网络、连接建立、对端响应影响），后者是**可用文案推断替掉的**状态真值。
本轮一律换成以下三类更靠近真值的判据：

| 代号 | 判据 | 为什么更硬 |
|---|---|---|
| **D-disk** | 目标机器磁盘上 inode/md5/内容的 before→after | 是"是否真写入"的物理真值，不受文案影响 |
| **D-struct** | 响应 JSON 的**字段集合**（`applied` / `conflicts[]` / `reason` 有没有）| 契约真值，不是措辞 |
| **D-phase** | 并发请求的**完成相位 / 释放时刻绝对对齐** | 不依赖"多少秒"，依赖因果（谁挡住谁） |
| **D-render** | CDP 直接读 DOM 的 `className` / `disabled` / 文本节点 | 绕开截图 OCR 与文案推断 |
| **D-behav** | 端点**行为相变**（挂起等 agent ⇄ 毫秒级 503）| 状态机的可观测输出 |

---

## C-C1 fanout 把「部分文件已写入」播成「没有应用这次同步」，丢弃单机真实错误

**判定：true（高）**

### Detector 审计
- hunter 判据：响应里 `error == "<hostname>没有应用这次同步。"` + `docker exec box-b ls` 时间戳。
- grep 结论：`没有应用这次同步。` 全仓仅 `sync.go:73` 定义、`:498,565` 使用；`msgAgentNotApplied` 只命中 sync.go。
  **符号本身无歧义**，但它是**渲染产物**——hunter 用它做唯一判据，等于"用被测方的措辞证明被测方措辞"。
  且 `ls` 时间戳容易被并发 hunter 污染（hunter 自己也列了这条干扰）。
- 我换用的判据：**D-disk + D-struct**。不看那句话，看两件事——
  ① `agents[]` 条目的**字段集合**里有没有 `applied` / `conflicts`；
  ② 磁盘上 center-only 文件 `c.md` 的 **inode 从无到有**、内容等于中心 `BOX-C-FILE`。

### 前置状态探测
中心 `GET /api/snapshot` gen=5，`store.pi = {"agents/c.md":"BOX-C-FILE\n", "context/SYSTEM.md":"SYSTEM-CONTENT-BOXC2\n", ...}`；
box-a 复位到 `c.md ABSENT` + `SYSTEM.md = SYS-REPRO3-LOCAL-<ts>`（制造 modify-vs-modify 冲突）。

### 三层证据
**① 纯函数/契约层（字段集合对照，`.qa-hunt/repro3/CC1b.log`）**
```
fanout 条目 keys : ['agentId','error','hostname','ok','skipped','status']
fanout 条目      : {"agentId":"e363efdcf085-1563","ok":false,"skipped":false,
                    "status":"conflicts-remain","error":"e363efdcf085没有应用这次同步。"}
single keys      : ['applied','conflicts','errors','ok','status','warnings']
single applied   : {"written":[{"adapterId":"pi","category":"agents","relPath":"c.md"}],
                    "deleted":[],"conflicts":[{"adapterId":"pi","category":"context","relPath":"SYSTEM.md"}]}
single conflicts : [{...,"reason":"modify-vs-modify",
                     "localContent":"SYS-REPRO3-LOCAL-1792158778\n",
                     "remoteContent":"SYSTEM-CONTENT-BOXC2\n"}]
```
→ fanout 侧 `applied` / `conflicts` **两个字段整体不存在**（`sync.go:45-53` 的 `agentApplyResult`
  结构体里就没有这两个字段），即"哪个文件写了、哪个文件冲突、两边内容"在 fanout 通道**结构上不可能传达**。

**② 端到端真实调用路径（`.qa-hunt/repro3/CC1b.log`）**
```
BEFORE: c.md ABSENT
fanout POST /api/sync?direction=dispatch&confirm=true  -> HTTP 422, status=partial
AFTER : inode=5795743 mtime=1792158744 /root/.pi/agent/agents/c.md
        BOX-C-FILE                       <- 中心内容确实落盘
        SYSTEM.md 仍为本地内容 SYS-REPRO3-LOCAL-1792158744
```
第二次复核（`.qa-hunt/repro3/CC1.log`）同样：`c.md ABSENT → md5 63a844dc…（=BOX-C-FILE）`。

**③ 用户可见（前端路径）**
`index.html:3786`：`setPluginError((result.errors||[]).join("；") || result.status || "下发没有完成")`。
fanout 顶层 `errors` 恒为 `null`（`sync.go:264` 字面量）→ 落到 `result.status` → 弹窗红条显示 `partial`，
插件页 `#plugin-error` 拿不到任何"c.md 已写入 / SYSTEM.md 冲突待 merge"的信息。
（本条与 hunter C4 是同一处 UI 现象，C4 不在本组任务内，不重复判。）

### 成对对照
| 轮次 | 操作 | c.md（磁盘真值） | 该机器 error 文案 |
|---|---|---|---|
| control-1 | 复位后不下发 | ABSENT | — |
| 实验-1 | fanout dispatch | **inode 新建，内容=BOX-C-FILE** | `e363efdcf085没有应用这次同步。` |
| control-2 | 复位后不下发 | ABSENT | — |
| 实验-2 | 单机 pull（同机器同状态） | inode 新建，内容=BOX-C-FILE | `检测到 1 个冲突（已保留本地）：请运行 homer merge 逐项裁决。` + 完整 conflicts[] |

**两轮结论稳定**：fanout 播的是"没有应用"，磁盘是"部分已应用"，单机路径给出冲突明细。
非环境噪声（不依赖终端尺寸、不依赖并发 hunter；inode 从无到有是物理事件）。

### 契约
`docs/api-reference.md` 未规定 fanout 的错误粒度 → 不是文档违约。
但 `sync.go:63-64` 自陈 "These strings are the entire user-facing error vocabulary of sync"，
即作者把可插值文案当唯一出口，而 `agentApplyResult` 结构上无法承载 `applied/conflicts`
→ **实现与该注释声明的设计意图自相矛盾**。判 true。

### 最小复现脚本（我自己写的，可复制）
```sh
# .qa-hunt/repro3/CC1.sh 精简版
H='Authorization: Bearer qa-hub-token'; B=http://127.0.0.1:17760
DC="docker compose -f qa/env/docker-compose.yml exec -T"
AID=<一台在线机器的 agentId>; 
# 0) 中心必须有该机器没有的文件：GET /api/storage/file?adapter=pi&path=agents/c.md
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; printf "LOCAL-%s\n" "$(date +%s)" > /root/.pi/agent/SYSTEM.md'
$DC box-a sh -c 'test -e /root/.pi/agent/agents/c.md && echo YES || echo NO'      # -> NO
# 1) fanout：看 error 文案 + 再看磁盘
curl -s -X POST -H "$H" -H 'Content-Type: application/json' --data '{"adapters":["pi"],"confirm":true}' \
  "$B/api/sync?direction=dispatch&confirm=true"   # -> "...没有应用这次同步。"
$DC box-a sh -c 'cat /root/.pi/agent/agents/c.md'                                # -> 内容已写入（矛盾）
# 2) 单机对照：同一状态
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md; printf "LOCAL-%s\n" "$(date +%s)" > /root/.pi/agent/SYSTEM.md'
curl -s -X POST -H "$H" -H 'Content-Type: application/json' --data '{"adapters":["pi"],"confirm":true}' \
  "$B/api/agents/$AID/pull?confirm=true"          # -> applied.written + conflicts[] + 中文指引
```

---

## C-C2 全部机器因停用被 skip 时仍 200 / ok:true / status:synced

**判定：true（中高）**

### Detector 审计
- hunter 判据：`jq -e '.ok==true and .status=="synced" and ([.agents[]|select(.skipped)]|length)==(.agents|length)'`
  + UI toast 文本。
- grep 结论：`skipped` 在 `sync.go` 内外都出现（UI 侧 `index.html:3778` 也读 `skipped`），
  但**判据本身不歧义**；问题在于 hunter 用 **UI toast 文案**做第二证据——文案是渲染产物。
- 我换用的判据：**D-disk**（下发到底有没有发生）+ **D-struct**（顶层 ok/status/errors）+ **D-render**
  （把真实响应喂给 `index.html:3776` 的 JS 表达式，看它算出的字符串，而不是去看页面）。

### §7 不误报核对（关键）
`agent_overrides_test.go:280 TestFanoutFiltersDisabledAdaptersAndReportsSkipped`：
alpha 只停用 `editor`（`shell` 仍在 scope）→ alpha **未被 skip**，beta 全停用 → skip。
该测试断言的是 `response.Code == 200` + `beta.Skipped && beta.Reason` + `pulledAgent==["alpha"]`。
**它覆盖的是"部分 skip"，不是"全部 skip"**；`status=="synced"` 且 0 台收到的情形
**没有任何测试断言**。故本条不是"已断言为设计的行为"，可以报。

### 三层证据
**① 契约层**：`sync.go:249-256` — `allOK` 只统计 `!OK && !Skipped`，`status := "synced"` 无 skip 分支。

**② 端到端（`.qa-hunt/repro3/CC2.log`）**
```
control（未停用）: HTTP 422 status=partial   box-a c.md 是否落盘: YES
实验  （全停用）: HTTP 200 {"ok":true,"status":"synced", ..., "errors":null}
                  4 台机器全部 skipped=true, reason="adapters disabled for this agent"
                  box-a c.md 是否落盘: NO
J2 断言: machines=4 skipped=4 ok=True status=synced errors=None -> True
```
配对完整：同一台机器、同一中心内容、同一分钟内，control 写盘 YES / 实验写盘 NO。

**③ 用户可见（把真实响应喂进前端表达式）**
```
TOAST(全停用)          = 已下发「pi」；3 台机器因已停用该适配器被跳过
TOAST(全离线,无 reason)= 已下发「pi」到全部在线机器        <- 0 台收到却说"全部"
```
第一条已足够误导（暗示"其余的收到了"）；**第二条更糟**——`sync.go:462-465` 的离线 skip 分支
**不写 Reason**，于是 UI 的 `skipped && reason` 过滤把它漏掉，直接走"已下发到全部在线机器"。

### 成对对照
| 轮次 | 操作 | HTTP | ok | status | c.md 落盘 |
|---|---|---|---|---|---|
| control-1 | 无停用，dispatch | 422 | false | partial | **YES** |
| 实验-1 | 全停用，dispatch | 200 | true | synced | **NO** |
| control-2 | 恢复启用后（同脚本尾）| — | — | — | 已全部 `disabled:false` 复原 |
| 实验-2 | （CC1b 轮同步观测到 skipped 条目形状一致）| — | — | — | 一致 |

### 判定依据
三层齐全；对照稳定；非环境噪声；`docs/api-reference.md:239` 明文规定
"扇出跳过：响应 `agents[].skipped=true` + reason" —— **文档只讲了 per-agent 字段，没讲顶层语义**，
但 `status:"synced"` 与"0 台收到"的事实矛盾，属 **200 掩盖失败**。判 true。

### 最小复现脚本
```sh
H='Authorization: Bearer qa-hub-token'; B=http://127.0.0.1:17760
DC="docker compose -f qa/env/docker-compose.yml exec -T"
IDS=$(curl -s -H "$H" "$B/api/agents" | python3 -c "import json,sys;print(' '.join(a['agentId'] for a in json.load(sys.stdin)['agents']))")
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md'
curl -s -o /dev/null -w 'control http=%{http_code}\n' -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true"
$DC box-a sh -c 'test -f /root/.pi/agent/agents/c.md && echo control-write=YES || echo control-write=NO'
for id in $IDS; do curl -s -o /dev/null -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"disabled":true}' "$B/api/agents/$id/adapters/pi"; done
$DC box-a sh -c 'rm -f /root/.pi/agent/agents/c.md'
curl -s -w '\nexp http=%{http_code}\n' -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"],"confirm":true}' "$B/api/sync?direction=dispatch&confirm=true"
$DC box-a sh -c 'test -f /root/.pi/agent/agents/c.md && echo exp-write=YES || echo exp-write=NO'
# 恢复
for id in $IDS; do curl -s -o /dev/null -X POST -H "$H" -H 'Content-Type: application/json' \
  --data '{"disabled":false}' "$B/api/agents/$id/adapters/pi"; done
```

---

## C-C3 / D-C1（同一根因）package-global writeMutex 无超时，半开机占锁冻结全队

**判定：true（高）· 两条合并为一个 issue**

### Detector 审计
- hunter C-C3 判据：`timeout 20 curl -m 20 /api/keys` → `000 20.00`（0 bytes）。
  hunter D-C1 判据：`curl time_total` 59~73s vs 基线 0.008s。
- grep 结论：`writeMutex` 全仓只在 `server.go:245` 定义（`var writeMutex sync.Mutex`，
  注释自陈 "intentionally package-global"、无 try-lock、无超时）；持锁点 10 处：
  `handlers.go:329,351,475(push),483(pull),491(ssh-key)`、`keys.go:79`、`plugins.go:197,251`、
  `resolve.go:60`、`sync.go:103`。**`/api/agents/<id>/pull` 与 `/api/keys` 确实是同一把锁**——
  这是硬事实，不是计时推断。
- 但**纯计时判据有歧义**：`time_total` 含连接建立、DNS、对端排队，无法区分"等锁"与"hub 忙"。
- 我换用的判据（**不依赖秒数**）：
  - **D-phase（相位/因果）**：同一时刻并发发"锁内"与"锁外"请求，看**完成相位**；
    更强的因果证据是 **释放时刻绝对对齐**——所有被挡请求都在长请求交还锁的**同一瞬间**返回。
  - **D-struct（锁成员资格）**：用"是否阻塞"这个布尔本身当判据，判定哪些端点在同一互斥域
    （`/api/keys` GET、`/api/keys/op` POST、带 confirm 的 pull、**不带 confirm 的 pull**）。
  - 静态层：`server.go:245` + 10 处持锁点 grep（已列）。
  - 尝试过的辅助判据：`/proc/net/tcp` ESTABLISHED 计数 → 返回 0（hub 容器 netns 视图问题），
    **P4 判据本轮未成立，如实标注；结论不依赖它**。

### 前置状态探测
我在 box-c 上起了**独占探针 agent**（私有 home `/root/.r3`，agentId `54d0c3373428-d82d`，
专属 hr_ 码），先 `homer init --all` 让它具备真实 pull 能力（否则 409 秒退，锁一放就开）。
冻结前 `/proc/1310/stat` 第三字段 = `S`，冻结后 = `T`；解冻后回 `S`（**已 CONT，见"环境恢复"**）。

### 三层证据
**① 静态（锁成员资格）**
```
server.go:245  var writeMutex sync.Mutex        <- 唯一一把，package-global
handlers.go:483  writeMutex.Lock() / handleAgentPull
keys.go:79       writeMutex.Lock() / writeKey
```
且 `handleAgentPull` 内 `confirmValue(r)` 的检查在 `handlers.go:605-628`，**在 483 行取锁之后**。

**② 端到端（`.qa-hunt/repro3/mutex2.log`，完整一轮，未提前 CONT）**
```
PID=1310 ; kill -STOP -> stat: T
T+8  锁外复查: OUT-agents http=200 t=0.001680          <- 毫秒级
T+8  锁内复查: keys      http=000 t=6.002274 (curl exit=28, 0 bytes)   <- 挂起
--- 长请求返回后 ---
DEAD-PULL            http=503 t=68.987523   (ping timeout, agent-offline)
keys-GET             http=200 t=65.982593
live-pull-NOCONFIRM  http=409 t=65.986572    <- ★ 无 confirm 的 409 早退路径同样被挡
keys-OP-POST         http=200 t=65.984922
live-pull-CONFIRM    http=422 t=65.992011    <- ★ 健康机器（box-a）的下发被冻结
OUT-console          http=200 t=0.000748
OUT-agents           http=200 t=0.000651
恢复后: keys-after  http=200 t=0.000568
```
**因果对齐（最关键的非计时证据）**：长请求 T+0 发出、T+68.99 返回；
被挡请求 T+3 发出、T+65.99 返回 → **绝对释放时刻同为 T+68.99**。
四个锁内请求在**同一个瞬间**被放行，而两个锁外请求在 T+0.0007 就返回 ——
这只能由"一把互斥锁"解释，不能由"hub 忙/网络慢"解释。

**③ 用户可见**
`index.html:4831-4841` `loadKeys()` → `GET /api/keys` 无超时保护 → 密钥区永久 loading /
最终「密钥加载失败：Failed to fetch」；对健康机器点「下发」同样无限挂起，
`beginDispatchWait` 计时器继续涨、无 cancel 按钮。
**本轮我未能在冻结窗口内完成 CDP 采样**（冻结窗口 66s，浏览器登录+导航耗时不够），
故 UI 侧仍是**代码路径推断**——与 hunter D-C1 自陈的缺口一致。

### 成对对照
| 轮次 | 操作 | 锁外 /api/agents | 锁内 /api/keys | 健康机 pull(confirm) | 无 confirm pull |
|---|---|---|---|---|---|
| control（冻结前基线）| 无 | 200 t=0.0008 | 200 t=0.0007 | — | — |
| 实验-1（early CONT）| STOP + 长 pull | 200 t=0.0008 | **挂起 12.27s**（长请求 14.28s 后同刻放行）| — | — |
| control-2（解冻后）| CONT | 200 t=0.0008 | **200 t=0.0006** | — | — |
| 实验-2（完整）| STOP + 长 pull | 200 t=0.0007 / 0.0017 | **挂起 65.98s** | **挂起 65.99s** | **挂起 65.99s → 409** |

两轮结论一致（hunter 另有 5 次），锁外/锁内二分稳定，解冻即恢复。

### 子裁决：不带 confirm 的 409 早退路径也取锁 —— **不单列**
`handlers.go:483` 取锁先于 `confirmValue` 判断，实测 `live-pull-NOCONFIRM` 被挡 65.99s 后返回 409。
这与 C-C3/D-C1 **同一根因**（锁粒度太粗 + 取锁位置过早），单列会成为同一行代码的第二张 issue。
裁决：**并入同一 issue，作为"加重情节/修复提示"写明**（修复方向：把 `confirmValue` 与
`requireOnline` 前移到取锁之前）。不单列 issue。

### 最小复现脚本
```sh
# .qa-hunt/repro3/mutex2.sh
H='Authorization: Bearer qa-hub-token'; B=http://127.0.0.1:17760
DC="docker compose -f qa/env/docker-compose.yml exec -T"
# 0) 起一台独占 agent（私有 home，避免被别人顶替），并 homer init --all 让它能真 pull
C=$(curl -s -H "$H" "$B/api/auth/join" | python3 -c "import json,sys;print(json.load(sys.stdin)['code'])")
$DC -e HOMER_HOME=/root/.r3 box-c sh -c "mkdir -p /root/.r3 && curl -fsSL http://hub:7760/install.sh | HOMER_HOME=/root/.r3 sh -s -- --token $C"
$DC -e HOMER_HOME=/root/.r3 box-c sh -c 'HOMER_HOME=/root/.r3 homer init --all'
DEAD=<该 agentId>; LIVE=<另一台健康机 agentId>
PID=$($DC box-c sh -c 'ps -eo pid,args | awk "/[h]omer agent/ && \$1>1000 {print \$1}"' | head -1)
# 1) 基线
curl -s -o /dev/null -m 8 -w 'base keys t=%{time_total}\n' -H "$H" "$B/api/keys"     # ~0.0007
# 2) 造半开
$DC box-c sh -c "kill -STOP $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"   # -> T
# 3) 长请求（后台）+ 3s 后并发探测锁内/锁外
( curl -s -o /dev/null -w 'DEAD t=%{time_total}\n' -X POST -H "$H" -H 'Content-Type: application/json' \
   --data '{}' "$B/api/agents/$DEAD/pull?confirm=true" ) &
sleep 3
for ep in "GET $B/api/keys" "POST $B/api/keys/op" "POST $B/agents/$LIVE/pull" "POST $B/agents/$LIVE/pull?confirm=true"; do ... & done
curl -s -o /dev/null -w 'OUT-agents t=%{time_total}\n' -H "$H" "$B/api/agents"      # 对照：毫秒级
# 4) 必须解冻
$DC box-c sh -c "kill -CONT $PID"; $DC box-c sh -c "awk '{print \$3}' /proc/$PID/stat"   # -> S
```

---

## C-C5 幽灵机器 GET 200 / POST 404（语义相反）

**判定：downgraded（低·一致性/文档瑕疵，非功能缺陷）**

### Detector 审计
- hunter 判据：两个 HTTP 状态码 + "其余子路由均 404" 作对照。
- grep 结论：`agent-not-found` 在 `errors.go:79-80`（→404）与 `agent_overrides.go:282` 产生，
  无歧义。**判据本身没问题**，但结论"语义相反"是个**契约主张**，需要查文档怎么写的。
- 我换用的判据：**D-struct 等价性**——把"幽灵 GET"与"真实机器 GET"的**响应体逐字节**比对，
  判定 GET 是否真能充当"存在性探针"；再查文档对 GET 条目有没有存在性承诺。

### 证据（`.qa-hunt/repro3/` 实机）
```
GET  /api/agents/nope/adapters             -> 200
  {"adapters":[{herdr,...},{opencode,...},{pi,...},{vscode,...}]}   (4 项, 全 disabled:false)
GET  /api/agents/54d0c3373428-bc77/adapters -> 200
  {"adapters":[{herdr,...},{opencode,...},{pi,...},{vscode,...}]}   ← 与幽灵**逐字节相同**
POST /api/agents/nope/adapters/pi {"disabled":true}
  -> 404 {"error":{"code":"agent-not-found","message":"机器不存在或尚未接入","details":[]}}
```
即：**GET 对该端点根本不携带"机器是否存在"的信息**（它就是"中心已安装 adapter ∪ 该机器上报 adapter"，
幽灵的上报集合为空，于是退化成中心列表）——它不是存在性探针，只是恰好与真实机器输出同形。

### 契约
`docs/api-reference.md:202-214` 对 GET 条目的描述是
"某台机器视角的适配器列表（中心已安装 adapter ∪ 该机器最后上报的 adapter）"——**没有任何存在性承诺**；
404 语义被写在 **POST** 条目下（`:229-230`），与实现一致。
同类降级模式在别处也存在并被测试固定：`plugins_test.go:20`（插件未接 → GET 200 空表）。

### 判定依据
没有契约明文被违反 → 按"契约未规定的设计取舍 = 不报"，**不判 true**。
残余价值：实现内部 GET/POST 不对称，属于 **一致性瑕疵**；若要报，报法是
"文档补一句：GET 不校验机器存在性，不能用作存在性探针"。**downgraded → 低·文档**。

---

## C-C6 `/api/plugins/install` 未知 id 返 404，文档只列 422/409

**判定：downgraded（低·纯文档缺失）**

### Detector 审计
- hunter 判据：`--data '{"id":"nope"}'` → 404；`grep -c 404 docs/api-reference.md` → 1。
- grep 结论：`plugin-not-found` 全仓只 `plugins.go:192` 产生，无歧义。
- 我换用的判据：文档**是否对该端点做出穷举承诺**（"已知错误包括…"这句是穷举还是举例），
  以及 404 是否真的可达于正常用户路径。

### 证据
```
POST /api/plugins/install {"id":"nope"}
  -> 404 {"error":{"code":"plugin-not-found","message":"官方插件不存在；第三方插件请提供 manifest","details":[]}}
docs/api-reference.md:141  "已知错误包括：schema 版本不支持时 HTTP 422、第三方 role 不是 adapter 时 HTTP 422、
                            ID 与官方或已安装插件冲突时 HTTP 409。"
grep -n 404 docs/api-reference.md  -> 只有 1 处（:230，adapters 那条）
```
代码侧三条分支齐备：`plugins.go:192`(404) / `:200,224`(409) / `:226`(422)，**实现无缺陷**。

### 判定依据
实现正确，缺的是文档一行。属**文档缺陷**，不是产品缺陷。
用户可见后果（客户端误把 404 当网关问题重试）是真实但轻微的。
**downgraded → 低·文档**，建议与 C-C7（文档覆盖缺口）合并成一张"API 文档补全"issue。

---

## D-C1 同 C-C3 —— 见上，**合并**

判定 **true（高）**，根因与 C-C3 完全相同。
唯一差别是观察面：C-C3 看 `/api/keys` 被挡，D-C1 看**健康机器的 pull** 被挡
（本轮实测 `live-pull-CONFIRM` 挂起 65.99s → 422，基线 0.008s）。
**UI 侧证据本轮仍未取得**（冻结窗口 66s，CDP 登录+导航不够），与 hunter 自陈一致，
故 D-C1 报为**后端阻塞缺陷**，UI 卡死表述为"后果推断"，不单列 UI issue。

---

## D-C2 同 hostname 重复接入不去重

**判定：false（设计意图）· 且 hunter 的"各行按钮集合不同"子主张被推翻**

### Detector 审计
- hunter 判据：`.agent-card[data-agent-id]` + `.hostname` + 各行按钮集合。
- grep 结论：`data-agent-id` / `.hostname` 只在 `renderAgents`（`index.html:1890-1895`）生成，无歧义。
  但"按钮集合不同"是**相关性误当因果**的风险点（stale 也会改按钮集合）。
- 我换用的判据：① **代码意图**（agent id 生成规则 + re-enroll 路径是否有意为之）；
  ② **D-render 控制变量**：比较**同 hostname 的两行**之间是否真的有 UI 差异，
  并把"差异"与 `stale` 做交叉归因。

### 证据
**① 代码意图（决定性）**
```
internal/agentd/agentd.go:58   return hostname + "-" + hex.EncodeToString(suffix[:])   // suffix = rand 2 bytes
internal/hub/registry.go:91    if current, ok := r.agents[info.AgentID]; ok { ... }    // 按 AgentID，不看 hostname
internal/web/installscript.go:59-70
   hr_* 码重跑 -> mv agent.json -> agent.json.before-reenroll
   "带一次性接入码(hr_)重跑脚本，意思就是「用这个码重新接入」...旧身份属于旧身份，不清掉的话新码永远用不上"
```
→ **re-enroll 产生新身份是明确的设计**，且有注释说明理由（换 hub / 重装时旧 secret 必须作废）。
`agenthub_test.go:526` 断言同 AgentID 重连顶替旧 session —— 去重维度**就是 AgentID**，不是 hostname。
`AgentInfo`（`registry.go:13-31`）里也没有任何"机器"概念，只有 `AgentID` + `Hostname`（后者只是展示字段）。

**② D-render 控制变量（`.qa-hunt/repro3/ui1.log`，真浏览器 CDP）**
```
54d0c3373428-bc77  host=54d0c3373428 ip=172.21.0.3 stale=false
   buttons=[查看,详情,密钥,登录公钥,吊销,移除]           cls="agent-card"
54d0c3373428-d82d  host=54d0c3373428 ip=172.21.0.3 stale=false   <- 我亲手造的重复接入
   buttons=[查看,详情,密钥,登录公钥,吊销,移除]           cls="agent-card"   ← 与上一行**完全相同**
e363efdcf085-7b6d  stale=true
   buttons=[查看(D),详情,密钥(D),登录公钥(D),吊销,移除]  cls="agent-card stale"  ← 差异源是 stale
```
**同 hostname 的两行按钮集合完全一致**；hunter 观察到的"按钮集合不同"来自 **stale 行**，
与重复接入无关 → **该子主张被推翻**。

**③ UI 是否已可区分**：`index.html:1892-1913` 为每行渲染 `.aid` span，显示**完整 agentId**
（CDP 实测 `aidVisible="54d0c3373428-bc77"`，`aidClipped=false`，且可点击复制）。
即两行是可区分的，只是没有"这两行是同一台机器"的聚合标记。

### 判定依据
- 契约/代码：`Attach` 以 AgentID 为身份主键（有测试断言），agent id 含随机后缀（设计），
  install.sh 主动为 re-enroll 造新身份（有注释）→ **不去重是设计，不是缺陷**。
- "按钮集合不同"子主张经控制变量证伪。
- 残余（没有"同一台机器多个身份"聚合标记）属 **UX 增味**，非缺陷。

### 推翻证据（判 false 必填）
换了判据：从"UI 里有几行、按钮一不一样"换成"代码身份主键是什么 + re-enroll 是否有注释说明
+ 同 hostname 两行的渲染签名比对"。跑出的相反结果：**同 hostname 两行渲染签名相同**
（`cls`/`dot`/`buttons` 全等），差异来自 `stale`；且 `agentd.go:58` 的随机后缀 +
`installscript.go:59-70` 的注释证明多身份是刻意设计。**判 false。**

---

## D-C3 半开下离线判定 75s vs 90s

**判定：downgraded（技术主张成立，但不单列 issue；并入 D-C1）**

### Detector 审计
- hunter 判据：`stale` 布尔 + `lastSeen` 算 age（**`stale` 是被测函数 `agentIsStale` 自己的输出**，
  用被测输出证明被测常量，循环论证风险）。
- 我换用的判据：**D-behav（行为相变）**——不看 `stale` 布尔，看
  `POST /api/agents/<半开机>/pull` 的**可观测行为**：
  翻转前 `requireOnline` 放行 → 请求**挂起等 agent**（`curl -m 2` 超时，exit 28）；
  翻转后 `requireOnline` 拒绝 → **毫秒级 503 agent-offline**。
  这测的是"这台机器是否还被认为可下发"，即真正的用户可观测后果。

### 证据（`.qa-hunt/repro3/stale.log.txt`）
```
pre-pull(未冻结) http=200 t=0.004107
STOP at 13:47:24, /proc/1310/stat = T
T+0s  beh=HANG(等agent)  stale/age=False age=2.3
T+36s beh=HANG(等agent)  stale/age=False age=37.9
T+71s beh=HANG(等agent)  stale/age=False age=73.5
T+76s beh=RET=503        stale/age=True  age=76.6      <- 相变
CONT -> stat: S
```
**相变发生在 age≈76.6s**（采样间隔 3s → 精确值 75~77s），与 `stream.DefaultPingTimeout=75s`
吻合，与 `AgentStaleAfter=90s` 差 14s（远超误差）。

### 常量归因（读 `stream/session.go` / `registry.go` / `hub/task.go`）
```
registry.go:341-343  agentIsStale = session==nil || sessionDone(session) || now-LastSeen >= AgentStaleAfter
hub/task.go:74       AgentStaleAfter = 90s
stream/session.go:16 DefaultPingTimeout = 75s
stream/session.go:693-760 pingLoop: now-lastRecv > timeout -> conn.Close(CloseHeartbeat,"ping timeout")
agenthub.go:236      session.SetHeartbeat(h.pingInterval(), h.pingTimeout())   // 75s 下发到 agent
agenthub.go:620      registry.Touch(agentID)   // 唯一更新 LastSeen 的点（touchConn.Read）
```
→ 半开场景下：无帧到达 → 不 Touch → `LastSeen` 冻结（90s 分支要等 90s）；
   但 `pingLoop` 在 **75s** 就 `Close` → `sessionDone` 为真 → `agentIsStale` **由 75s 分支先判定**。
**猎人"90s 分支从未生效"的主张在半开场景下成立**（75 < 90，先到先判）。
反之，只要还在收帧（含 pong），`Touch` 就会刷新 `LastSeen`，90s 分支也追不上 ——
该分支实际只在"session 存活但长期不读帧"的窄窗口里可能成为决定因素。

### 判定依据
- 技术主张**成立**（75s 是决定因素，实测 76.6s）。
- 但"离线检测要 75s"本身**没有任何契约/文档承诺过检测时延**，属心跳协议的设计取舍
  （`25s ping / 75s timeout` 是标准配置）。按"契约未规定的设计取舍 = 不报"。
- 它造成的**用户可见后果与 D-C1 完全重合**（假死机在 75s 内仍可点「下发」→ 一按就冻结全队 66s）。
- 裁决：**downgraded → 低 / 并入 D-C1 作为根因链的一环**（修复 writeMutex 后本条后果即消失大半）。
  若要单独提，只提"UI 对'已超时但未判离线'的机器缺中间态"这条 UI 增味，**不是本条**。

---

## D-C4 revoke 后机器仍留列表，与"掉线"不可区分

**判定：true（中）· UI 呈现缺陷（后端保留行是设计，不报）**

### Detector 审计
- hunter 判据：`/api/agents` 里 agentId 仍在 + 卡片 `.classList.contains('stale')` + 按钮 disabled。
- grep 结论：保留 registry 行是**设计意图**（`handlers.go:1003-1005` 注释 + scout §3.7），
  报它必被驳。hunter 自己也划了这条界（"请 qa-repro 按 UI 判据复核"）——这条边界划得对。
- 我换用的判据：**D-render 直接 diff**——抓"我亲手吊销的机器"与"本来就掉线的机器"
  两张卡片的 `className` / `dot class` / **按钮 disabled 向量** / 徽章文本，
  判**结构签名**是否相同（而不是看 hunter 的"都变灰"描述）。

### 证据（`.qa-hunt/repro3/dc4.log` + `ui2.log`，真浏览器 CDP）
```
POST /api/agents/revoke {"agentId":"54d0c3373428-d82d"} -> 200 {"ok":true}
POST 第二次                                              -> 409 not-enrolled
   "该机器没有有效凭证（可能从未接入或已吊销）"
hub 日志: kicking agent stream agent="54d0c3373428-d82d" code=4401 reason="agent secret revoked"
          agent stream disconnected ... (code 4401): agent secret revoked     <- 真踢了

渲染签名 diff（cls / dot / buttons 三元组）：
  revoked : ["agent-card stale","dot off",["查看(D)","详情","密钥(D)","登录公钥(D)","吊销","移除"]]
  offline : ["agent-card stale","dot off",["查看(D)","详情","密钥(D)","登录公钥(D)","吊销","移除"]]
  => 三元组完全相同。唯一不同的 statusText 是 drift 计数（"已对齐" vs "↑1 项未收取…"），
     与吊销与否无关 —— 一台有 drift 的机器被吊销后，与掉线机**逐字段相同**。
  badge 扫描：两行都无任何含「吊销/已吊销」的文本节点。
```

### 判定依据
- 后端保留行 = 设计，不报（hunter 也是这个边界）。
- **UI 侧无区分标记是真实的、可机械断言的**：`cls`/`dot`/`buttons` 三元组逐项相等。
- 更实的一条（非推断）：吊销后「吊销」按钮**仍为 enabled**（`d:false`），
  用户再点必然拿到 `409 not-enrolled "该机器没有有效凭证"` —— 一个把"吊销已成功"
  呈现成"吊销失败"的错误。这是**可复现的功能性误导**，不依赖可用性推测。
- 三层齐全（代码 `handlers.go:1003-1005` + API 实机 200/409 + CDP DOM 渲染签名）。判 true。

### 最小复现脚本
```sh
H='Authorization: Bearer qa-hub-token'; B=http://127.0.0.1:17760
AID=<一台在线机器>
curl -s -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"agentId\":\"$AID\"}" "$B/api/agents/revoke"  # {"ok":true}
sleep 5
curl -s -X POST -H "$H" -H 'Content-Type: application/json' -d "{\"agentId\":\"$AID\"}" "$B/api/agents/revoke"  # 409 not-enrolled
# CDP：抓卡片 cls/dot/buttons，与一台 stale(掉线) 机器比对 -> 三元组相同
```

---

## D-C5 fanout 对离线机器 skipped(ok:true) 且 errors:null，却 HTTP 422

**判定：downgraded（低）· 大部分被 C-C2 吸收；"静默成功"的表述不成立**

### Detector 审计
- hunter 判据：`agents[].skipped` / `agents[].ok` / 顶层 `status`。
- grep 结论：`skipped` 在 `sync.go` 与 `index.html:3778` 都出现，但结构字段无歧义。
- 我换用的判据：**把真实响应喂进前端表达式**（`index.html:3774-3788`），
  看 UI **实际走哪个分支、算出什么字符串**，而不是推断"前端会呈现成功"。

### §7 不误报核对
`choices_stream_test.go:369`：`if result.Skipped != infos[i].Stale` —— **`skipped == stale` 被断言为设计**。
故"离线机器被 skipped 且 ok:true"**是已断言行为，不能报**。

### 证据（`.qa-hunt/repro3/dc5.json`）
```
HTTP 422  {"ok":false,"status":"partial","errors":null,"agents":[
  {"54d0c3373428-bc77", ok=true,  skipped=false},
  {"54d0c3373428-d82d", ok=true,  skipped=true,  reason=<缺失>},   <- 离线/吊销 skip：sync.go:462-465 不写 Reason
  {"b6675d68ddf7-7920", ok=false, status=conflicts-remain, error="...没有应用这次同步。"},
  {"e363efdcf085-1563", ok=false, status=conflicts-remain, error="..."},
  {"e363efdcf085-7b6d", ok=true,  skipped=true,  reason=<缺失>}]}

喂进前端：result.ok=false -> 不走 toast 分支 -> setPluginError(errors.join() || status)
  => 显示 "partial"（errors 为 null 所以落到 status）
```

### 判定依据
- "HTTP 422 却 errors:null、前端只读 errors 会呈现成功" → **不成立**：
  本例 `ok=false`，前端直接走错误分支。hunter 自己也标了"低"。
- 真实残余只有两条，且都已另有所属：
  (a) `errors:null` + 422 → UI 裸显 `partial`（内部 token）— 归 C-C1/C4 那条 UI 链；
  (b) **离线 skip 不写 `Reason`**，导致全部机器都离线时 `allOK=true` → 200 synced，
      toast 走"已下发到全部在线机器" — **这条正是 C-C2 的严重形态**，归 C-C2。
- 裁决：**downgraded → 不单列 issue**；把 (b) 作为 C-C2 的加重证据写入，(a) 并入 UI token 那条。

---

## 汇总表

| 编号 | 猎人判 | 我的判定 | 严重度裁决 | 我换的判据 | 与猎人判据差异 |
|---|---|---|---|---|---|
| C-C1 | 高 | **true** | 高 | D-disk(inode) + D-struct(字段集合) | 不用那句文案；改用"applied/conflicts 字段结构性缺失"+ inode 从无到有 |
| C-C2 | 高 | **true** | 中高 | D-disk(写盘YES/NO配对) + 把响应喂进 index.html:3776 表达式 | 不用 toast 截图；用控制变量 + 真实响应算 toast |
| C-C3 | 高 | **true** | 高 | D-phase(释放时刻绝对对齐) + 静态 grep 锁成员 | 不用 time_total；用"4 个锁内请求同一瞬间放行、2 个锁外毫秒级" |
| C-C5 | 中 | **downgraded** | 低·文档 | 幽灵 GET vs 真实 GET 响应体逐字节比对 + 文档承诺核查 | 从"语义相反"改成"GET 是否承载存在性信息" |
| C-C6 | 中 | **downgraded** | 低·文档 | 穷举承诺核查 + 三分支代码核对 | 从"未文档化=缺陷"改成"实现正确、文档缺一行" |
| D-C1 | 高 | **true**（合 C-C3）| 高 | 健康机 pull 的相位 + 因果对齐 | 同 C-C3；并明确 UI 半仍缺证据 |
| D-C2 | 中 | **false（设计）** | — | agentd.go:58 随机后缀 + installscript.go:59-70 注释 + 同 hostname 两行渲染签名比对 | 从"有 2 行"改成"身份主键是谁 + 差异是否由 stale 造成" |
| D-C3 | 中 | **downgraded** | 低·并入 D-C1 | D-behav：pull 从"挂起"相变为"毫秒级 503"，不看 stale 布尔 | 不用被测函数自己的输出做判据 |
| D-C4 | 中 | **true** | 中（UI）| D-render：revoked vs offline 卡片 cls/dot/buttons 三元组 diff | 从"都变灰"改成可机械比对的三元组 |
| D-C5 | 低 | **downgraded** | 低·并入 C-C2 | 真实响应喂进前端分支表达式 | 证否"静默成功"：ok=false 走错误分支 |

**判 true 的**：C-C1、C-C2、C-C3/D-C1（同一根因）、D-C4 —— 共 **4 个独立 issue**。
**判 false 的**：D-C2（设计，且按钮差异子主张被推翻）。
**判 downgraded 的**：C-C5、C-C6（文档/一致性，低）、D-C3、D-C5（并入其它 issue）。

---

## 明确建议：C-C3 与 D-C1 是否合并为一个 issue

**合并。**

理由（三条硬依据）：
1. **同一行代码、同一把锁**：`server.go:245 var writeMutex sync.Mutex`（package-global、无 try-lock、
   无超时）；阻塞点是 `handlers.go:483` 的 `writeMutex.Lock()` 包住 `handleAgentPull`。
   C-C3 观察的是 `/api/keys`（`keys.go:79`），D-C1 观察的是健康机 `/api/agents/<id>/pull`
   （`handlers.go:483`）—— **两者是同一互斥域的不同成员**，不是两个缺陷。
2. **同一因果链、同一时间轴**：本轮实测四个锁内请求（keys GET、keys/op POST、健康机 pull with
   confirm、健康机 pull without confirm）在**同一瞬间**（长请求交还锁的 T+68.99）集体放行，
   锁外请求全程毫秒级。一张图就能同时证明 C-C3 和 D-C1，拆成两张 issue 会让两份报告互相重复证据。
3. **修复方案唯一**：try-lock + 超时，或 per-agent 锁，或把 `requireOnline`/`confirmValue` 前移到
   取锁之前。任何一个修复同时消掉两条。拆开提会导致两条 issue 被同一次修复关闭。

**合并后的 issue 应包含三个观察面（不是三个 issue）**：
- (a) 密钥面：`GET /api/keys`、`POST /api/keys/op`、插件安装/卸载（`plugins.go:197,251`）、
  `/api/push` `/api/pull`（`handlers.go:329,351`）、`/api/resolve`（`resolve.go:60`）全部被冻结；
- (b) 下发面：**健康机器**的下发被冻结 ~66s（基线 0.008s）；
- (c) 加重情节（**不单列**）：不带 `confirm` 的 pull（409 早退路径）同样取锁被挡
  —— `handlers.go:483` 取锁先于 `confirmValue` 判断，修复提示是把前置校验移到取锁前。

**不合并进来的**：D-C3（75s 检测时延）虽然处在同一条因果链上游，但它没有契约违约、
且修复 writeMutex 后其后果基本消失，建议**在 issue 里作为"为什么锁会被占这么久"的背景段落提及**，
不单独开 issue；若产品希望缩短假死检测，那是另一条独立的（产品）诉求。

---

## 环境恢复（必填）

- `kill -STOP` → **全部已 `kill -CONT`**：探针 PID 1310 末次 `/proc/1310/stat` 第三字段 = `S`
  （`.qa-hunt/repro3/mutex2.log`、`.qa-hunt/repro3/stale.log.txt` 均有记录）。
  **全程未使用 `docker pause`**，无 unpause 事项。
- 我的独占探针 agent `54d0c3373428-d82d` 已 `POST /api/agents/remove` → `{"ok":true}`，
  私有 home `/root/.r3` 已删除。
- 我在 box-c 起探针时用 `pkill -f "homer agent"` 收尾，**误杀了 box-c 上原有的 agent**
  （`54d0c3373428-bc77`）。已按 `qa/README.md` §3.5 用
  `nohup /root/.local/bin/homer agent --home /root/.homer &` 重启，日志显示
  `13:54:05 已连接 hub`，`/api/agents` 中 `54d0c3373428-bc77 stale=False`。**已恢复。**
  教训（建议沉淀）：box 上有多个 agent 进程时，收尾禁用 `pkill -f`，应按 PID 精确 kill。
- box-b 在 13:54 出现一次 `broken pipe` 重连，属我的探针清理引发的 agent 侧退避重连，
  13:54:19 已自动重连，`b6675d68ddf7-7920 stale=False`。
- box-a 上 PID 29/205/309 仍为 `T`（**hunter 之前的 SIGSTOP 残留，非我造成，未触碰**）。
- 最终 `/api/agents`：`54d0c3373428-bc77 False` / `b6675d68ddf7-7920 False` /
  `e363efdcf085-1563 False` / `e363efdcf085-7b6d True`（最后一台是环境原有的 stale，非我造成）。
- 未执行 `make qa-restart`（共享 hub）。
- ⚠ 仓库内有另一 hunter 留下的未跟踪文件 `internal/gens/zz_repro_c2_test.go`
  （`git status` 可见），**不在我的所有权内，未删除**，请主 agent 处理。

## 未覆盖 / UNVERIFIED
1. **C-C3/D-C1 的 UI 侧实机证据**：冻结窗口 ~66s，CDP 登录+导航+渲染耗掉大部分窗口，
   两轮均未在冻结期内完成 DOM 采样。UI 卡死仍为**代码路径推断**（`index.html:4831` loadKeys
   无超时保护）。UNVERIFIED，建议后续用"预登录页面 + 冻结期间只 eval"的方式补。
2. **连接态判据（P4）**：`/proc/net/tcp` 在 hub 容器内返回 ESTABLISHED=0（netns 视图问题），
   `netstat`/`lsof` 存在但未取到有效样本。该判据本轮**未成立**，结论不依赖它。
3. **writeMutex 的"无限"上限**：本轮阻塞 ~66s（= ping timeout），与 hunter 的 59~73s 同量级；
   真正的 12min 上限（一次完整 pull budget）未在 60min 预算内实机逼近。
4. **C-C2 的"全离线"形态**（离线 skip 无 Reason → toast 说"已下发到全部在线机器"）：
   我用真实响应跑出了该字符串，但**未在"全部机器都离线"的实机状态下跑完整 UI**，
   因为环境里始终有在线机器。属高置信推断，非实机。
