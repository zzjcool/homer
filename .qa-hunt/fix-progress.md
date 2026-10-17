# issue 清理进度（父会话状态持久化）

仓库 `zzjcool/homer-cli`，基准 HEAD `010521a`（master），开始时间 2026-10-16。
目标：清理全部 17 个 open issue（#106–#122）。

## 已冻结的裁决（advisor 出具，worker 照做，不再重新论证）

| Issue | 裁决 | 关键依据 |
|---|---|---|
| #108 | **严格档**：agentId 已绑定 secret 时拒绝 hr_ 码顶替。检查放在 `Redeem` **之前**（Redeem 是 one-shot，先兑后拒会白白烧码） | install.sh 重跑会改名 agent.json → `DefaultAgentID()` 生成新 agentId（`agentd.go:58`），重装永不复用旧 ID；合法重接入走 `Revoke` 删绑定，之后自然放行 |
| #109/#110 | **分层收口**：admin(hub token\|session cookie)=写端点+join；agent-secret=`/api/snapshot`+`/dl/*`；hr_=仅 `/dl/*`+`GET /api/snapshot`+stream。默认拒绝+白名单 | hub/auth.go:48-55 已是此语义，web 侧是唯一背离；agent 端从不依赖 hr_ 走数据面写；UI 走 session cookie，`homer join` 走 hub token |

## 门禁事实（scout 侦察，ci.yml 为准，非 README）

- CI 唯一强制门禁 = `.github/workflows/ci.yml` 的 `test` job：
  `go build ./...` + `go vet ./...` + `gofmt -l .`(grep -v vendor) + `go test $(go list ./... | grep -v '/tests/e2e')` + `sh -n install.sh`
- **无** golangci-lint / pre-commit / Makefile lint target。
- `tests/e2e` 被 ci.yml 显式排除；chromedp UI 测试无浏览器则 Skip（本机无 chromium）。
- **改 `index.html` 的高危面**：`internal/web/copy_test.go` 的 `TestConsoleUserCopy` 是冻结的
  文案契约（git 词黑名单 + 必需串白名单）；`TestIndexInlineScriptParses` 用 `node --check`
  验内联 JS 语法。**改前端文案前必读 `copy_test.go:19-99`**。
- `internal/cli/hubcommands_test.go:148` bind 死 `127.0.0.1:7760` —— 多 worker 并行跑该包会冲突。

## Worker 派发表

| Issue | worker | 分支/worktree | 所有权文件 | verify | 状态 |
|---|---|---|---|---|---|
| #106 | w106 | fix/106-* | internal/web/sync.go(仅 storage/file 区) | go test ./internal/web/ | running |
| #107 | w107 | fix/107-* | internal/gens/gens.go + sync.go:597 错误处理 | go test ./internal/gens/ ./internal/web/ | running |
| #108 | w108 | fix/108-* | internal/hub/{enrollment,agenthub}.go | go test ./internal/hub/ ./internal/hubclient/ | running |
| #109+#110 | w109-110 | (待派) | internal/web/{auth,handlers,password}.go, gate_matrix_test.go | go test ./internal/web/ | 待派（内存门拦） |
| #111 | w111 | (待派) | server.go + 10 处取锁点 | go test ./internal/web/ + -race | 待派（内存门拦） |
| #112+#117+#120 | (待派) | | internal/web/sync.go fanout 区 + index.html:1033/3776 | | 待派 |
| #113 | (待派) | | index.html copyJoin | | 待派 |
| #114 | (待派) | | index.html dispatch cancel/abort | | 待派 |
| #115+#121 | (待派) | | index.html renderAgents/refresh | | 待派 |
| #116 | (待派) | | index.html logout | | 待派 |
| #118 | (待派) | | handlers.go /api/agents + index.html 卡片 | | 待派 |
| #119 | (待派) | | index.html watchJoinCode | | 待派 |
| #122 | 父会话自做 | | docs/api-reference.md | — | 进行中 |

## 合并纪律

- worker 不自己 merge；父会话统一合回 master 并开 PR。
- 并行 worker 的文件所有权互斥，**共享文件（sync.go、index.html）必须序列化**。
- 内存门：`PI_LEGION_MIN_FREE_MEM_MB=1024` + 160MB/child，本机并发上限约 3 个 worker。
