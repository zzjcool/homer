# issue 清理进度（父会话状态持久化）

仓库 `zzjcool/homer`，基准 HEAD `010521a`（master），执行时间 2026-10-16/17。
目标：清理全部 17 个 open issue（#106–#122）。

## 交付对照（17 条 + 1 chore）

| Issue | PR | 状态 |
|---|---|---|
| #106 storage/file adapter 穿越 | [#123](https://github.com/zzjcool/homer/pull/123) | CLOSED |
| #107 snapshot 文件名穿越 | [#124](https://github.com/zzjcool/homer/pull/124) | CLOSED |
| #108 接入码冒认 agentId | [#125](https://github.com/zzjcool/homer/pull/125) | CLOSED |
| #109+#110 接入码写权限/自我增殖 | [#128](https://github.com/zzjcool/homer/pull/128) | CLOSED |
| #111 writeMutex 无超时 | [#131](https://github.com/zzjcool/homer/pull/131) | CLOSED |
| #112+#117+#120 fanout 三连 | [#129](https://github.com/zzjcool/homer/pull/129) | CLOSED |
| #113+#116+#119+#121 前端四连 | [#130](https://github.com/zzjcool/homer/pull/130) | CLOSED |
| #122 api-reference 文档 | [#127](https://github.com/zzjcool/homer/pull/127) | CLOSED |
| #114+#115 假取消 / 轮询重建 | w114-115 worker | 进行中 |
| #118 吊销后可辨识 | w118 worker | 进行中 |
| chore: gitignore .qa-hunt | [#126](https://github.com/zzjcool/homer/pull/126) | MERGED |

## 已冻结的裁决（advisor 出具）

| Issue | 裁决 |
|---|---|
| #108 | 严格档：已**确认持有**的 agentId 拒绝 hr_ 码顶替；检查在 Redeem 之前（Redeem 是 one-shot，先兑后拒会白白烧码） |
| #109/#110 | 分层收口：admin(hub token\|session cookie)=写端点+join；agent-secret=`/api/snapshot`+`/dl/*`；hr_=仅 `/dl/*`+`GET /api/snapshot`+stream。默认拒绝+白名单 |

## 关键教训

### #108 孤儿绑定（务必记住）
worker 按「已绑定即拒绝」落地后，`tests/integration` 的 `TestBurnedEnrollCodeReconnect`
从 pass 变 fail。`BindAgent` 在 **welcome 之前**就建立 hub 侧绑定，welcome 丢失则留下
**无人持有的孤儿绑定**；只按「已绑定」拒绝 → 「同一 home 换新码重接入」永久失败
（与 `agentd/client.go:279` 的官方恢复指引冲突）。
**最终语义**：`IsConfirmed(已确认持有) || 有活跃会话` 才拒绝；未确认且离线的绑定允许新码重接。
盘上 load 的绑定默认标 confirmed（安全默认）；`PrincipalSecret` 握手成功时 MarkConfirmed。

### worker 僵尸进程检测
3 个 worker 曾同时因 provider `Connection error.` 死亡，表现为 state=working 但
**usage 计数冻结 + session jsonl mtime 停滞**。检测手法即看这两项，不要空等。
代码留在 worktree 里，由父会话接手验证并提交。

### 基线失败（非回归）
`internal/gitx` 的 `TestEnsureLocalIdentityWritesOnlyThisRepo` 在 baseline(010521a)
同样失败 —— CI 会先设 git 身份，本机没设。忽略。

### 门禁事实
CI 唯一强制门禁 = `.github/workflows/ci.yml` test job：
`go build ./...` + `go vet ./...` + `gofmt -l .`(grep -v vendor) +
`go test $(go list ./... | grep -v '/tests/e2e')` + `sh -n install.sh`。
无 golangci-lint / pre-commit。`tests/e2e` 被显式排除；本机无 chromium，UI 测试 Skip。
**改 index.html 必读 `internal/web/copy_test.go:19-99`**（冻结文案契约）。

### 派发环境坑
- 团队 `qa-hunting` 的 roster 不含 `worker` → 必须 `team: "default"` 派发。
- 内存门 `PI_LEGION_MIN_FREE_MEM_MB=1024` + 160MB/child，本机并发上限约 3 个 worker。
- 误把改动直接 push 到 master（分支保护拒绝强推回滚），后续一律走 PR。
