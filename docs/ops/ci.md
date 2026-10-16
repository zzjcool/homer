# CI 与发版触发

合并到 `master` 前，[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) 应全绿。它在指向 `master` 的 pull request，以及推送到 `master` 时运行：

- `go build ./...`
- `go vet ./...`
- `gofmt -l .`（有输出则失败；排除路径名里含 `vendor` 的行，与 README 验收命令一致）
- `go test`（`-count=1 -timeout 20m`），**不含** `tests/e2e`
- `sh -n install.sh`

Go 版本跟 `go.mod`（`actions/setup-go` 的 `go-version-file`），与发版工作流一致。作业会先设置 git 身份（`user.email=ci@homer-cli.local`，`user.name=homer-ci`），因为个别用例用环境里的身份做 commit，而 GitHub runner 默认没有这项配置。

这是 PR 单元门禁：build、vet、gofmt、以及除 `tests/e2e` 以外的测试。`tests/e2e`（需要本机 `pi`、浏览器 / chromedp）仍不进这条门禁。

## Hub 容器化冒烟（`hub-smoke`）

多容器拓扑验收走独立工作流 [`.github/workflows/hub-smoke.yml`](../../.github/workflows/hub-smoke.yml)：

```sh
bash e2e/hub-smoke.sh
```

需要本机 / runner 上有 Docker、`docker compose`、`curl`、`jq`。脚本拉起 origin + hub + agent-a + agent-b，覆盖健康检查、双 agent、资源快照、跨容器 push/pull、鉴权负例、密钥环跨容器解开、按机器停用后的扇出跳过；结束（含失败）会 `compose down -v`。

**何时跑（策略 C）**：指向 `master` 的 PR、推到 `master`，且改动命中 path-filter 时；也可手动 `workflow_dispatch`。过滤路径：

- `e2e/**`
- `internal/hub/**`、`internal/agentd/**`、`internal/stream/**`、`internal/web/**`
- `cmd/homer/**`、`go.mod`、`go.sum`（烟测镜像会重建二进制）
- `.github/workflows/hub-smoke.yml` 自身

失败时脚本会向 stderr 打出 `compose ps` / 近期日志；CI 另设 `HUB_SMOKE_ARTIFACT_DIR`，在 `down -v` 前落盘 `compose-ps.txt` / `compose-logs.txt`，并由 Actions 上传 artifact `hub-smoke-compose-logs`。

本地需要时再跑：`go test ./tests/e2e/...`，以及上面的 `bash e2e/hub-smoke.sh`。

仓库内另有一份与 workflow 内容相同的副本 `e2e/hub-smoke.github-actions.yml`（便于审阅）；Actions 只认 `.github/workflows/hub-smoke.yml`。

打 `v*` tag 才会跑已有的 [`.github/workflows/release.yml`](../../.github/workflows/release.yml)：goreleaser 产出 Release 资产，并对 `dist` 里的 `install.sh` 做一次冒烟。资产纪律见 [release-discipline.md](release-discipline.md)。
