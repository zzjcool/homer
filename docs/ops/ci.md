# CI 与发版触发

合并到 `master` 前，[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) 应全绿。它在指向 `master` 的 pull request，以及推送到 `master` 时运行：

- `go build ./...`
- `go vet ./...`
- `gofmt -l .`（有输出则失败；排除路径名里含 `vendor` 的行，与 README 验收命令一致）
- `go test`（`-count=1 -timeout 20m`），**不含** `tests/e2e`
- `sh -n install.sh`

Go 版本跟 `go.mod`（`actions/setup-go` 的 `go-version-file`），与发版工作流一致。作业会先设置 git 身份（`user.email=ci@homer-cli.local`，`user.name=homer-ci`），因为个别用例用环境里的身份做 commit，而 GitHub runner 默认没有这项配置。

这是 PR 单元门禁：build、vet、gofmt、以及除 `tests/e2e` 以外的测试。`tests/e2e`（需要本机 `pi`、浏览器 / chromedp）仍不进这条门禁。多容器拓扑与浏览器控制台各有独立可选工作流（见下）。

## Hub 容器化冒烟（`hub-smoke`）

多容器拓扑验收脚本：

```sh
bash e2e/hub-smoke.sh
```

需要本机 / runner 上有 Docker、`docker compose`、`curl`、`jq`。脚本拉起 origin + hub + agent-a + agent-b，覆盖健康检查、双 agent、资源快照、跨容器 push/pull、鉴权负例、密钥环跨容器解开、按机器停用后的扇出跳过；结束（含失败）会 `compose down -v`。

Actions 线见 issue #34 / PR #76（策略 C：path-filter + `workflow_dispatch`）。本页不替代该工作流的文档；边界见下文。

## Console chromedp 套件（`console-e2e`）

浏览器控制台验收走独立工作流 [`.github/workflows/console-e2e.yml`](../../.github/workflows/console-e2e.yml)：

```sh
go test -tags consolee2e ./tests/e2e -run TestConsole -count=1 -timeout 40m
```

需要本机 / runner 上有 Docker、`docker compose`，以及 `/usr/bin/chromium`（chromedp；见 `tests/e2e/console_docker_test.go` 的 `//go:build consolee2e`）。套件按 `e2e/console/docker-compose.yml` 拉起 hub + keys + 若干 box，用真实浏览器走登录、接入、收取/下发等控制台场景。

**子集说明**：`-run TestConsole` 匹配 `TestConsoleDockerScenarios`（当前 consolee2e 标签下的控制台套件），**不是** `tests/e2e` 下全部用例（那些仍要本机 `pi` / 其它 mock UI，留在本地）。

**何时跑（策略 C）**：指向 `master` 的 PR、推到 `master`，且改动命中 path-filter 时；也可手动 `workflow_dispatch`。过滤路径：

- `internal/web/**`
- `tests/e2e/**`
- `e2e/console/**`
- `e2e/Dockerfile`、`e2e/keys-server/**`（console 镜像构建上下文）
- `cmd/homer/**`、`go.mod`、`go.sum`（镜像会重建二进制）
- `.github/workflows/console-e2e.yml` 自身

失败时 CI 上传 artifact `console-e2e-logs`（`go test` 输出 + `compose ps` / `compose logs`）。套件本身不落盘浏览器截图。

仓库内另有一份与 workflow 内容相同的副本 `e2e/console-e2e.github-actions.yml`（便于审阅）；Actions 只认 `.github/workflows/console-e2e.yml`。若推送方无 `workflow` OAuth scope，需经 GitHub UI 或 `gh auth refresh -s workflow` 把副本晋升到该路径。

### 与 `ci.yml` / `hub-smoke` 的边界

| 门禁 | 覆盖 | 不覆盖 |
| --- | --- | --- |
| `ci.yml` | build / vet / gofmt / 非 e2e 单测 | `tests/e2e`、compose 拓扑、chromedp |
| `hub-smoke` | `e2e/hub-smoke.sh` 四容器 API/拓扑冒烟 | 浏览器控制台 UI |
| `console-e2e` | chromedp `TestConsole*` + `e2e/console` compose | hub-smoke 拓扑场景；其它非 consolee2e 的 e2e |

三者互补，互不替代。

打 `v*` tag 才会跑已有的 [`.github/workflows/release.yml`](../../.github/workflows/release.yml)：goreleaser 产出 Release 资产，并对 `dist` 里的 `install.sh` 做一次冒烟。资产纪律见 [release-discipline.md](release-discipline.md)。
