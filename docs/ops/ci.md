# CI 与发版触发

合并到 `master` 前，[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) 应全绿。它在指向 `master` 的 pull request，以及推送到 `master` 时运行：

- `go build ./...`
- `go vet ./...`
- `gofmt -l .`（有输出则失败；排除路径名里含 `vendor` 的行，与 README 验收命令一致）
- `go test`（`-count=1 -timeout 20m`），**不含** `tests/e2e`
- `sh -n install.sh`

Go 版本跟 `go.mod`（`actions/setup-go` 的 `go-version-file`），与发版工作流一致。作业会先设置 git 身份（`user.email=ci@homer-cli.local`，`user.name=homer-ci`），因为个别用例用环境里的身份做 commit，而 GitHub runner 默认没有这项配置。

这是 PR 门禁：build、vet、gofmt、以及除 `tests/e2e` 以外的测试。`tests/e2e`（需要本机 `pi`、浏览器 / chromedp）和多容器 `e2e/hub-smoke.sh` 是另一条验收线，不进这次 PR CI。

打 `v*` tag 才会跑已有的 [`.github/workflows/release.yml`](../../.github/workflows/release.yml)：goreleaser 产出 Release 资产，并对 `dist` 里的 `install.sh` 做一次冒烟。资产纪律见 [release-discipline.md](release-discipline.md)。

本地需要时再跑：`go test ./tests/e2e/...`，以及 `bash e2e/hub-smoke.sh`（origin + hub + agent-a + agent-b）。
