# 缺陷狩猎 · 角度 C：API 契约一致性 / 错误语义 / 200 掩盖失败

仓库 `/root/code/homer`，HEAD `010521a`。hub `http://127.0.0.1:17760`，Bearer `qa-hub-token`。
box-a=`e363efdcf085`，box-b=`b6675d68ddf7`，box-c=`54d0c3373428`（hub 容器名 `homer-qa-hub`）。
全部结论来自 curl 实机 + CDP 真浏览器；产物在 `.qa-hunt/artifacts-C/`。

## 基建
```
$ curl -sS -H "Authorization: Bearer qa-hub-token" http://127.0.0.1:17760/api/health
{"homerHome":"/root/.homer","ok":true,"status":"healthy","version":"dev"}   # PASS
$ docker compose -f qa/env/docker-compose.yml ps   # hub + box-a/b/c + browser 全 Up（28min）  PASS
```
接入 3 台 box（install.sh + `homer init --all`）并用 `POST /api/sync?direction=collect&confirm=true`
造出 generation，依赖快照的路径（dispatch / /api/storage/*）已可测。基建 PASS。

---

## Candidates

### C1 fanout 把「部分文件已写入」播成「没有应用这次同步」，且丢弃每条机器的真实错误
- 疑似位置: `internal/web/sync.go:482-489`（`result.Error = name + msgAgentNotApplied`（498 行），常量 `sync.go:73`）
- **detector**:
  ```
  curl -sS -m 110 -X POST -H "Authorization: Bearer qa-hub-token" \
    -H 'Content-Type: application/json' --data '{"adapters":["pi"],"confirm":true}' \
    'http://127.0.0.1:17760/api/sync?direction=dispatch&confirm=true'
  # 断言：响应里该机器的 error 字段 == "<hostname>没有应用这次同步。"
  # 再断言：同一台机器磁盘上出现了中心 generation 独有的文件
  docker compose -f qa/env/docker-compose.yml exec -T box-b sh -c 'ls -la /root/.pi/agent/agents/'
  ```
  符号唯一性：`没有应用这次同步。` 全仓仅 `internal/web/sync.go:73` 一处定义 + `sync.go:498,565` 使用；grep `msgAgentNotApplied` 只命中 sync.go。`adapters disabled for this agent`（skipped 的 reason）与它不冲突。
- 前置状态探测: 先 `GET /api/storage` 确认 `generation:3` 且含 `agents/c.md`；先 `docker exec box-b ls /root/.pi/agent/agents/` 确认 **c.md 不存在** 且 `SYSTEM.md=SYS-box-b-LOCAL-<ts>`（本地改动 = 制造冲突）。
- 证据: `artifacts-C/C1-fanout-evidence.txt`、`C1-fanout-ab.json`、`C1-single-ab.json`、`C1-fanout-vs-single-evidence.txt`
- 复现步骤:
  1. box-b 上 `rm -f ~/.pi/agent/agents/c.md`，再把 `~/.pi/agent/SYSTEM.md` 改成本地内容（制造 modify-vs-modify 冲突）。
  2. `POST /api/sync?direction=dispatch&confirm=true` body `{"adapters":["pi"]}`。
  3. 观察该机器结果：`{"ok":false,"skipped":false,"status":"conflicts-remain","error":"b6675d68ddf7没有应用这次同步。"}`
  4. `docker exec box-b ls -la /root/.pi/agent/agents/` → **c.md 时间戳变成刚才（13:15），文件确实写进去了**。
  5. 对照：对**同一台**机器 `POST /api/agents/b6675d68ddf7-7920/pull?confirm=true` 返回
     `{"ok":false,"status":"conflicts-remain","conflicts":[{"adapterId":"pi","category":"context","relPath":"SYSTEM.md","reason":"modify-vs-modify","localContent":"SYS-box-b-LOCAL-1792156502\n","remoteContent":"SYSTEM-CONTENT-BOXC2\n"}],"errors":["检测到 1 个冲突（已保留本地）：请运行 `homer merge` 逐项裁决。"]}`
  → 同一失败，单机路径给出「哪个文件、哪种冲突、两边内容」，fanout 路径只给一句**与事实相反**的话。
- 用户可见后果: 控制台机器卡片 → 插件页「下发到全部机器」的 `#plugin-error` 只显示 `b6675d68ddf7没有应用这次同步。`；用户以为一台都没动，实际部分文件已被覆盖，**他不知道自己有一半内容被改了**，也无从知道该去哪台机器 `homer merge` 什么。
- 我无法排除的干扰: box-b 上 `c.md` 的新时间戳来自我这轮 dispatch，但共享 hub 上另一 hunter 也可能同一秒在写；不过 c.md 内容 = 中心 generation 独有文件，且我在同一脚本内 before/after 对照，判定成立。`grep -c` 未做全仓 exhaustive（只 grep 了 internal/ + 前端）。

### C2 所有机器都因适配器被停用而 skip 时，HTTP 200 + `ok:true` + `status:synced`
- 疑似位置: `internal/web/sync.go:254-263`（`allOK` 只统计 `!OK && !Skipped`；`status := "synced"` 在 255 行）；UI 文案 `internal/web/static/index.html:3781`
- **detector**:
  ```
  # 1) 给每台机器都停用 pi
  for id in $(curl -s -H "Authorization: Bearer qa-hub-token" $B/api/agents | jq -r '.agents[].agentId'); do
    curl -s -X POST -H "Authorization: Bearer qa-hub-token" -H 'Content-Type: application/json' \
      --data '{"disabled":true}' "$B/api/agents/$id/adapters/pi"; done
  # 2) 断言：全 skip 仍 200 + ok:true + status:synced
  curl -s -o /tmp/r -w '%{http_code}\n' -X POST -H "Authorization: Bearer qa-hub-token" \
    -H 'Content-Type: application/json' --data '{"adapters":["pi"],"confirm":true}' \
    "$B/api/sync?direction=dispatch&confirm=true"
  # 断言表达式：jq -e '.ok==true and .status=="synced" and ([.agents[]|select(.skipped)]|length)==(.agents|length)' /tmp/r
  ```
  符号唯一性：`"adapters disabled for this agent"` 只在 `sync.go` 一处产生（fanoutScopeForAgent 的 skip 分支）；`status := "synced"` 在 dispatch 方向上只有 `sync.go:255` 一处；同函数内 `status = "partial"` 在 257。
- 前置状态探测: 先 `GET /api/agents/<id>/adapters` 确认每台 `"disabled":true`；先确认中心 `generation:3` 有 pi 内容（否则会被 no-snapshot 409 提前挡掉）。
- 证据: `artifacts-C/C2-allskip.json`、`C2-evidence.txt`、`C2-ui-dispatch-200-synced.json`、真浏览器 CDP 断言脚本 `harness-cdp2-allskip-toast.js`（`window.__toasts` 文本）
- 复现步骤:
  1. 停用全部机器上的 pi（6 台机器全部 200）。
  2. `POST /api/sync?direction=dispatch&confirm=true` body `{"adapters":["pi"],"confirm":true}`。
  3. 响应 **HTTP 200** `{"ok":true,"status":"synced","direction":"dispatch","agents":[{...,"ok":true,"skipped":true,"reason":"adapters disabled for this agent"},...],"errors":null}` —— 6 台全 skip，`errors:null`。
  4. 真浏览器（CDP http://127.0.0.1:3000）插件页点「下发到全部机器」（pi）：toast = 「已下发「pi」；3 台机器因已停用该适配器被跳过」——**只提被跳过的台数，不说「实际 0 台收到」**，且 `#plugin-error` 为空（无错误）。
  5. 检查机器磁盘：`ls /root/.pi/agent/agents/` 与中心 generation 独有文件不一致（无任何新写入）。
- 用户可见后果: 中心有内容、机器上什么都没有，控制台说「同步成功 / 已下发」，用户以为全员一致。这是典型的 200 掩盖失败。
- 我无法排除的干扰: 另一 hunter 的 SIGSTOP/disable 动作可能偶发让某台变 stale 从而走 skip 分支（也是 200），但本条的判据是「**全部** skip 仍 synced」，两种来源结论一致。截图里 3 台是因为该轮另有 stale 机器，不是驳因。

### C3 package-global writeMutex 无超时：一台机器的长下发会把**所有其他机器**的密钥操作挂死
- 疑似位置: `internal/web/server.go:245`（`var writeMutex sync.Mutex`）+ 持锁点：`handlers.go:329,351,475,483,491`、`keys.go:79`、`plugins.go:197,251`、`resolve.go:60`、`sync.go:103`
- **detector**:
  ```
  # 后台起一个必挂的长下发（box-b agent 被 SIGSTOP，12min budget 会一直等）
  curl -s -m 145 -X POST -H "Authorization: Bearer qa-hub-token" -H 'Content-Type: application/json' \
    --data '{}' "$B/api/agents/b6675d68ddf7-7920/pull?confirm=true" &
  sleep 4
  # 断言表达式：timeout 20 curl -m 20 -o /dev/null -w '%{http_code} %{time_total}' -H "$H" "$B/api/keys"
  #   -> 期望 200 <0.01s；实测 "000 20.00"（curl exit 28，0 bytes received）
  # 对照（同锁外的端点保持毫秒级）：
  #   /api/console /api/agents /api/storage /api/plugins /api/status 全部 200 <0.01s
  ```
  符号唯一性：`writeMutex` 全仓只在 `internal/web/server.go:245` 定义；grep `writeMutex.Lock()` 命中 handlers.go:329,351,475,483,491 / keys.go:79 / plugins.go:197,251 / resolve.go:60 / sync.go:103，共 10 处。未与其它锁混淆（`generationMutex` 是另一个名字，`sync.go:583-587`）。
- 前置状态探测: 操作前先 `GET /api/keys` 实测 `HTTP 200 time=0.001283`（未持锁基线）；再 `docker exec box-b ps -eo pid,args | grep '[h]omer agent'` 拿到 pid=36，`kill -STOP 36` 后确认进程 `STAT=S`（可观测停住），才发长请求。
- 证据: `artifacts-C/C3-mutex-block.txt`
- 复现步骤:
  1. `docker compose ... exec -T box-b sh -c 'kill -STOP <agent pid>'`
  2. 后台起 `POST /api/agents/b6675d68ddf7-7920/pull?confirm=true`（会挂在等 agent 回帧上，最长 12min）。
  3. 立刻 `timeout 22 curl -m 20 -H "$H" $B/api/keys` → **超时，0 bytes**（不是 4xx/5xx，是无限挂起）。
  4. `POST /api/keys/op {"action":"list"}` 同样超时。
  5. 同一时刻 `/api/console`、`/api/agents`、`/api/storage`、`/api/plugins`、`/api/status` 均 200 <0.01s（证明是这把锁，不是 hub 整体卡死）。
  6. `kill -CONT 36` 后长请求返回 503 agent-offline，`/api/keys` 立刻恢复 200 0.001s。
- 用户可见后果: A 机器在大下发的 12 分钟里，用户打开控制台密钥区（`loadKeys()` → `GET /api/keys`，`index.html:4831-4841`）会一直挂在 loading，超时后 `#key-body` 显示「密钥加载失败：Failed to fetch」；对 B 机器点「下发」同样无限挂起（无 cancel 按钮，`beginDispatchWait` 计时器还会一直涨）。用户无法自助区分「在忙」与「坏了」。
- 我无法排除的干扰: 这是**共享 hub**，另一 hunter 的并发请求也可能短暂占锁；但我用「同锁内超时 / 同锁外毫秒」「STOP 前基线 / CONT 后恢复」三重对照，且全程只有我这一个 client（同一条脚本顺序执行），归因成立。

### C4 控制台把内部状态机 token 原样贴给用户：红色告警栏出现 `partial`
- 疑似位置: `internal/web/static/index.html:3786`（`setPluginError((result.errors||[]).join("；") || result.status || "下发没有完成")`）
- **detector**: 真浏览器 CDP 断言 `document.querySelector('#plugin-error').textContent === "partial"`
  ```
  # UI 路径：插件页 → 「下发到全部机器」→ 部分机器失败
  # 后端确实返回 errors:null（sync.go:264 writeJSON 的 `nil` 字面量，fanout 的失败只写进 agents[].error）
  ```
  符号唯一性：`partial` 这个裸词在本轮响应里只来自 `sync.go:262-263` 的 `status = "partial"`；UI 侧 `result.status` 只此一处被当作展示文案（`index.html:3786`）。
- 前置状态探测: 先确认插件页已打开（`#dlg-plugins.open === true`）且 `#plugins-installed .plugin-row[data-plugin-id=pi]` 存在，才点按钮。
- 证据: `artifacts-C/C4-ui-raw-partial.png`（截图可见红色竖条告警写着 `partial`）
- 复现步骤:
  1. 让至少一台机器下发失败（例如给该机器制造冲突，或停用它的 pi）。
  2. 真浏览器登录控制台 → 插件 → pi → 点「下发到全部机器」。
  3. 弹窗内红色告警栏文字 = `partial`（英文内部 token），没有任何中文解释、没有说明哪台机器怎么了。
- 用户可见后果: 用户看到 `partial` 这个词，不知道是「部分成功」还是内部错误码，也不知道下一步做什么。（注意：`ok:true` 全 skip 的分支走的是 toast 那条，不会到这里；能到这里的都是真失败。）
- 我无法排除的干扰: 截图是 1280×720 的 browserless 视口，正文可能被截断；但 `#plugin-error` 元素 textContent 由 CDP `Runtime.evaluate` 直接读出为 `"partial"`，与截图一致，不依赖 OCR。

### C5 幽灵机器的 GET 与 POST 语义相反：GET 返 200 完整列表，POST 返 404
- 疑似位置: `internal/web/agent_overrides.go:208-211`（GET 只校验 agentID 非空）vs `agent_overrides.go:270-284`（POST 有幽灵防御，404 在 282）
- **detector**:
  ```
  curl -s -o /dev/null -w '%{http_code}\n' -H "$H" "$B/api/agents/nope/adapters"          # -> 200
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "$H" -H 'Content-Type: application/json' \
       --data '{"disabled":true}' "$B/api/agents/nope/adapters/pi"                        # -> 404
  # 对照：其余 agent 子路由对幽灵机器均 404
  # status / diff / keys / sync/choices?agent=nope 全部 404 agent-not-found
  ```
  符号唯一性：目标组件标识用 `agent-not-found`（`errors.go:79-80` 映射 404）+ 文案「机器不存在或尚未接入」（grep 确认全仓仅 `agent_overrides.go:282`）。避开通用边框字符，用响应体 JSON 字段判定。
- 前置状态探测: 先 `GET /api/agents` 列出真实 agentId，确认 `nope` 不在其中（证明它确实是幽灵）。
- 证据: `artifacts-C/C5-C6-C7-C8-evidence.txt`
- 复现步骤:
  1. `GET /api/agents/nope/adapters` → **200** `{"adapters":[{"id":"herdr",...,"installed":true},...4 项]}`
  2. `POST /api/agents/nope/adapters/pi` body `{"disabled":true}` → **404** `{"error":{"code":"agent-not-found","message":"机器不存在或尚未接入"}}`
  3. 其余子路由对 `nope` 均 404。
- 用户可见后果: 文档 `docs/api-reference.md:229-230` 把这个 404 语义作为该端点的契约；一个按文档写的脚本（先 GET 探测机器是否存在，再决定是否 POST）会得到相反答案——`GET` 说有、`POST` 说没有；反过来，把 GET 结果当「机器存在」的自动化会一直在幽灵机器上失败。这是「契约与实现不一致」本身，不只是一个状态码口味问题。
- 我无法排除的干扰: 是否「缺陷」取决于产品意图（GET 返回的是**中心已安装插件**的并集，未必需要机器存在）。但同一份代码里 POST 已经明确做了「幽灵机器 404」的语义决定（含注释：避免 agent-overrides.json 积累垃圾），GET 没跟上 —— 两者不可能同时是意图。

### C6 `/api/plugins/install` 未知 id 的 404 分支在文档中不存在（文档只列 422/409）
- 疑似位置: `internal/web/plugins.go:192`（`plugin-not-found` → 404）；文档 `docs/api-reference.md:141`
- **detector**:
  ```
  curl -s -o /tmp/p -w '%{http_code}\n' -X POST -H "$H" -H 'Content-Type: application/json' \
    --data '{"id":"nope"}' "$B/api/plugins/install"      # -> 404
  cat /tmp/p   # {"error":{"code":"plugin-not-found","message":"官方插件不存在；第三方插件请提供 manifest","details":[]}}
  grep -c "404" docs/api-reference.md                     # -> 1（只有 adapters 那条提到 404）
  ```
  符号唯一性：`plugin-not-found` 全仓只在 `plugins.go:192` 产生。
- 前置状态探测: 先 `GET /api/plugins` 确认 `nope` 既不在 `installed` 也不在 `available`（确实是未知 id）。
- 证据: `artifacts-C/C5-C6-C7-C8-evidence.txt`
- 复现步骤: 见上，3 条命令。
- 用户可见后果: 按文档写客户端的开发者只为 422/409 写了分支，遇到 404 会当成「网络/网关问题」重试或报「端点不存在」，而正确动作是「换个插件 id」。
- 我无法排除的干扰: 文档缺 404 是否「故意」无法从代码判定；但文档明确宣称这是「Hub API 参考」并逐条列出了该端点的错误码，缺一条就是文档缺陷本身。

### C7 文档目录声称是「Hub API 参考」，实际只覆盖 5/约 30 个端点
- 疑似位置: `docs/api-reference.md`（6 个 `##` 标题中 5 个是端点，第 6 个是 dispatch）；实现侧 `internal/web/handlers.go:100-230`（20 条路由分支）+ `handlers.go:445-520`（9 个 agent 子路由）+ `password.go` 的 6 个 `/api/auth/*` + `hub/agenthub.go` 的 `/agent/v1/*`
- **detector**:
  ```
  grep -oE '^## `[A-Z]+ [^`]+`' docs/api-reference.md | wc -l        # -> 5
  grep -oE 'case (path == |strings\.HasPrefix\(path, )"/[^"]*"' internal/web/handlers.go | wc -l   # -> 20 路由分支
  # 加上 agent 子路由 9 个 + /api/auth/* 6 个 + /agent/v1/* 5 个
  ```
  符号唯一性：判据是「文档标题数」与「实现 switch 分支数」两个可数集合，不依赖任何 UI 符号。
- 前置状态探测: 无（纯静态）。
- 证据: `artifacts-C/C5-C6-C7-C8-evidence.txt`
- 复现步骤: 上面两条 grep 直接对比。文档完全没提的端点包括 `/api/console`、`/api/status`、`/api/keys/*`、`/api/storage/*`、`/api/snapshot`、`/api/resolve*`、`/api/agents/<id>/pull|push|status|diff|keys|upgrade|tool-upgrade|ssh-key` 等。
- 用户可见后果: 运维/脚本作者无从知道 `/api/keys` 会**明文传口令**（`keys.go` body 带 password）、`/api/agents` 会回吐机器 MAC/IP/CPU ——这些是有安全含义的契约；也没人能从文档得知 `/api/resolve` 的 `record=true` 需要 `adapters`。
- 我无法排除的干扰: 文档标题自称范围是「插件管理与适配器下发」，可以辩解为「只写这一块」。但 `README.md:143` 把它作为唯一跳转目标，用户无法从任何地方得到其余端点参考——所以对**使用者**而言仍是覆盖缺口。

### C8 `writeMethodNotAllowed` 对所有路由都发误导性 `Allow: GET, POST`
- 疑似位置: `internal/web/handlers.go:872-874`（`w.Header().Set("Allow", "GET, POST")` 在 873 行）
- **detector**:
  ```
  for p in /api/console /api/status /api/agents /api/plugins /api/keys /api/storage /api/config /api/diff /api/sync/choices; do
    printf "%s -> " "$p"
    curl -s -D - -o /dev/null -X DELETE -H "$H" "$B$p" | grep -iE '^(HTTP/|allow)' | tr -d '\r' | tr '\n' ' '; echo
  done
  # 全部返回：HTTP/1.1 405 Method Not Allowed Allow: GET, POST
  ```
  符号唯一性：`Allow: GET, POST` 这个字面值在 `internal/web/` 只由 `handlers.go:873` 设置（`agentd/executor_cas_test.go` 是测试 stub；`hub/agenthub.go:116` 设的是 `Allow: GET`，值不同）。
- 前置状态探测: 这些路由在 `GET` 下确实 200（先 GET 一次确认端点存在），才发 DELETE 观察 405。
- 证据: `artifacts-C/C5-C6-C7-C8-evidence.txt`
- 复现步骤: 见上循环。例如 `/api/console` 只接受 GET，却宣告 `Allow: GET, POST`。
- 用户可见后果: 按 RFC 7231 §6.5.5，`Allow` 是客户端发现可用方法的契约。遵守它的自动化会拿 `POST /api/console` 去试并再吃一个 405；`Allow: POST` 还是 CSRF 探测的常用信号，错误的 `Allow` 会误导安全工具判断该端点接受写操作。用户可见后果偏弱（一个额外往返 + 误判），故列为低危。
- 我无法排除的干扰: 无环境依赖，纯 header 观察，最干净的一条。

---

## 证伪（我实测后**否掉**的怀疑点，供 reporter 别再报）

| 原怀疑 | 实测 | 结论 |
|---|---|---|
| #4 `agents-disabled` 有 501 与 422 两种状态 | `handlers.go:615-620`：当 `messages[0]=="这台 hub 不能在机器上解开密钥"` 时**同时**把 code 设为 `agents-disabled`、status 设为 `StatusNotImplemented`(501)；`errors.go:89` 也是 501。`resolve.go:225-233` 同构 | **同一个 code 恒为 501，不存在 422 分支。证伪**（scout 的 618-619 读数有误） |
| #3 `writeRemoteWriteReport` 把 `ok:true`+`conflicts-remain` 播成 200 | `newPullCommandReport`（`pull.go:99-102`）`OK = status==applied||no-drift`；`resolutions.go:326` 失败时 `report.OK=false`。全仓无任何路径产出 `ok:true`+`conflicts-remain`。实测单机 pull 冲突恒为 **422** | **证伪** |
| #8 二进制文件预览 200 `{"binary":true,"content":""}` 会让前端显示「文件是空的」 | `index.html:4345` 明确分支：`f.binary ? "这个文件不是文本，无法在这里预览。" : (f.content \|\| "(空文件)")` | **证伪**（有正确文案） |
| #10 13min WriteTimeout vs 12min pull | 需要撞上长耗时下发，60min 预算内无法安全逼近；代码事实（`cli/hub.go:142` 13min、`proto.go:64` 12min、`agentd/handlers.go:147-166` 远端 limit=requestBudget）成立，但**我没有实机复现** | **未验证，见「未覆盖」** |

## 未覆盖
1. **#10 WriteTimeout 时间窗**：未能实机复现「客户端拿到空响应但文件已写」。要复现需一次 ≥13min 的 pull（或 N 次串行 12min pull 撑爆 writeMutex 队列），超出 60min 预算。代码事实已列证。
2. **C3 的「无限」上限**：我只证到 ≥20s 阻塞。真正的排队上限（N 个并发 pull 全部 12min）没测，因为要占满 60min。
3. **`/api/storage/file` 的 adapter 参数 traversal**：实测 `adapter=..&path=../../keys/hub-token` 返 404（`filepath.Join` 把 `..` 吃掉落到 store 外 → 文件不存在），**未拿到 keys/hub-token 内容**，但也**没有确认在有 generation 时 `adapter=..` 是否可读 store 内其它 adapter 的目录**（那是设计内还是越界，需要契约裁决）。未下结论。
4. **`/agent/v1/stream` 的 WebSocket 层错误语义**（malformed hello / 缺 subprotocol 时客户端看到什么）：本轮只走 HTTP/curl，未起 WS 客户端。
5. **`ssh-key` 端点按错误消息子串判状态码**（`handlers.go:688-693`）：只测到「用户名无效 → 400」，未测「不存在 → 404」与「没有公钥 → 404」是否会被其它 GitHub 文案绕过（外部依赖，不稳）。
6. **前端 `refresh()` 四处静默吞错**（`index.html` 14 处 `catch(_){}`）：本轮只验证插件页这一条路径，其余未逐条触发。
7. **C7 那 30 个未文档化端点里各有多少个错误码未列**：只统计了标题数，没有逐个端点核对错误码清单。

## 环境
- 仓库 `/root/code/homer` @ `010521a`；go1.23.4。
- hub `http://127.0.0.1:17760`（容器 `homer-qa-hub` / 内部 `http://hub:7760`），Bearer `qa-hub-token`，密码 `qa-admin-password`。
- box-a=`e363efdcf085` / box-b=`b6675d68ddf7` / box-c=`54d0c3373428`，容器内 hub 主机名 `44e53f65e565`。
- 浏览器 CDP `http://127.0.0.1:3000`（browserless chromium），node 用 `/root/code/pi-async/node_modules/ws` 连根 ws，`Target.attachToTarget(flat)` 取 sessionId，页面走 `http://hub:7760/`。
- harness：`/tmp/cdp.js`、`/tmp/cdp2.js`、`/tmp/cdp3.js`（CDP 驱动）；curl 脚本逐段存进 `artifacts-C/*evidence.txt`。
- 造场景：`docker compose -f qa/env/docker-compose.yml`；`homer init --all` 后 collect 出 `generation:3`（含 `pi/agents/c.md`、`pi/context/SYSTEM.md`）。
- 阻塞技巧：`kill -STOP <agent pid>` 制造「假在线但不应答」的长 pull（不阻塞我的 shell）。
- ⚠ 共享 hub：另一 hunter 也在改同一 hub（出现过 hostname=`hunter-probe`、gen 被改成 5/6）。所有断言都在同一脚本内 before/after 对照，不依赖全局唯一性。
