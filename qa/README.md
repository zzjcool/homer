# homer-cli QA 网页环境

AI QA 通过浏览器界面测试 hub 控制台的专用环境。`make qa-up` 从源码一键
启动，无需预装 Go 或浏览器。

## 快速开始

```sh
make qa-up        # 源码构建 + 启动 hub + 3 个 idle box + 浏览器
make qa-status    # 容器状态
make qa-logs      # 跟随 hub 日志
make qa-down      # 停止（数据清零）
make qa-restart   # down + up，全新环境
```

启动完成即打印访问信息：

| 项 | 值 |
|---|---|
| 控制台（宿主） | http://127.0.0.1:17760 |
| 控制台（容器网络内，浏览器用这个） | http://hub:7760 |
| 管理员密码 | `qa-admin-password` |
| hub token（API 断言用） | `qa-hub-token` |
| 浏览器 CDP | `ws://127.0.0.1:3000`（browserless v2） |

## 拓扑

- **hub** — 源码构建的 homer 镜像（复用 `e2e/Dockerfile`）。管理员密码
  通过只读挂载 `qa/env/hub-password`（bcrypt 哈希）预置，绕过 one-shot
  初始化门；`--token qa-hub-token` 固定 token。
- **box-a / box-b / box-c** — idle 容器，镜像内置 `pi` stub。从网页复制
  接入命令后在里面执行，agent 由 `install.sh` 从 hub 下载二进制自举。
- **browser** — `ghcr.io/browserless/chromium`，AI QA 走 CDP 操控页面。

## AI QA 操作指南

### 1. 连接浏览器

browserless v2 的 `/json/version` 返回根 `webSocketDebuggerUrl`。注意
根连接是 browser 级：**必须先 `Target.attachToTarget(flat:true)` 拿
sessionId，再带 sessionId 调 `Page.*` / `Runtime.*`**（裸调会报
`'Runtime.evaluate' wasn't found`）。每次连接一个新 page target，
session cookie 不跨连接保留。

```js
// Node 示例（已实测可用）
const version = await fetch("http://127.0.0.1:3000/json/version").then(r => r.json());
const ws = new WebSocket(version.webSocketDebuggerUrl.replace("ws://0.0.0.0", "ws://127.0.0.1"));
// ... onmessage 按 id 匹配 promise ...
const targets = await send("Target.getTargets");
const page = targets.result.targetInfos.find(t => t.type === "page");
const sid = (await send("Target.attachToTarget", {targetId: page.targetId, flatten: true})).result.sessionId;
const eval_js = expr => send("Runtime.evaluate", {expression: expr, returnByValue: true}, sid);
await send("Page.enable", {}, sid);
await send("Page.navigate", {url: "http://hub:7760/"}, sid);
```

### 2. 登录

密码已预置，直接登录（不会出现 setup 门）：

```js
await eval_js(`document.querySelector('#in-login-pw').value='qa-admin-password'`);
await eval_js(`document.querySelector('#btn-login').click()`);
// 等待 !document.querySelector('#workspace').classList.contains('hidden')
```

已知行为（QA 断言时注意）：

- 登录失败会触发退避：下一次尝试被直接拒绝，需等 ~2s 再试
  （`internal/web/password.go` verifyPassword）。
- 密码错误时 `#gate-login` 不隐藏（错误留在原表单）。

### 3. 接入新机器（agent）

1. 页面点「接入命令」（`#btn-join-top`），从 `#join-cmd` 取命令，形如：
   `curl -fsSL http://hub:7760/install.sh | sh -s -- --token hr_<48hex>`
   （浏览器访问 `http://hub:7760` 时，命令内嵌的 hub 地址就是容器网络
   地址，box 内可直接执行）。
2. 在 box 里执行：

```sh
docker compose -f qa/env/docker-compose.yml exec -T box-b sh -c '<命令>'
```

3. install.sh 从 hub `/dl/homer.gz` 下载二进制（hr_ 码可鉴权下载但只
   能兑换一次注册）、写入 token、后台启动 agent。几秒后机器出现在
   网页机器列表（新机器 · 等待下发）。
4. hr_ 码一次性：复用已兑换的码再接入会失败（QA 负例素材）。
5. box 重启后 agent 不会自动拉起，手动启动：
   `exec box-a sh -c 'nohup /root/.local/bin/homer agent --home /root/.homer >>/root/.homer/agent.log 2>&1 &'`

### 4. API 旁路断言

QA 除了操作网页，还可以用固定 token 直接调 API 做状态断言：

```sh
curl -H 'Authorization: Bearer qa-hub-token' http://127.0.0.1:17760/api/agents
```

`/api/auth/*`（login/status/join）不需要鉴权头。

### 5. 常用页面选择器

| 元素 | 选择器 |
|---|---|
| 登录密码框 | `#in-login-pw` |
| 登录按钮 | `#btn-login` |
| 工作区 | `#workspace`（hidden class 表示未登录） |
| 接入命令按钮 | `#btn-join-top` |
| 接入命令文本 | `#join-cmd` |
| 机器列表区 | `main section:not(.hidden)`（`#sec-agents` 等） |

## 修改管理员密码

```sh
htpasswd -bnBC 10 "" 'new-password' | tr -d ':\n' > qa/env/hub-password
make qa-restart
```

（bcrypt，任意工具生成均可；尾部换行无影响。）

## 文件

- `qa/env/docker-compose.yml` — 环境定义
- `qa/env/hub-password` — 预置密码哈希（qa-admin-password）
- `Makefile` — qa-up / qa-down / qa-restart / qa-logs / qa-status

## 已知限制

- hub 容器无挂载 volume：`make qa-down` 后 hub 状态（机器列表、
  enrollment 码）清零。QA 需要干净环境时 `make qa-restart`。
- 接入命令内嵌的地址取决于浏览器访问 hub 时的 Host 头；宿主浏览器走
  `http://localhost:17760` 时命令里的地址在 box 内不可达，需把
  `127.0.0.1:17760` 替换成 `hub:7760` 再执行，或让浏览器（browserless）
  直接访问 `http://hub:7760`（推荐，QA 浏览器本来就在容器网络内）。
- `e2e/Dockerfile` 的 `pi` stub 会让「应用升级」类测试受限（pi 永远
  0 版本）。
