# CI 与发版触发

合并到 `master` 前，[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) 应全绿。它在指向 `master` 的 pull request，以及推送到 `master` 时运行：

- `go vet ./...`
- `go test ./... -count=1 -timeout 20m`

Go 版本跟 `go.mod`（`actions/setup-go` 的 `go-version-file`），与发版工作流一致。

打 `v*` tag 才会跑已有的 [`.github/workflows/release.yml`](../../.github/workflows/release.yml)：goreleaser 产出 Release 资产，并对 `dist` 里的 `install.sh` 做一次冒烟。资产纪律见 [release-discipline.md](release-discipline.md)。

`e2e/hub-smoke.sh` 要起多容器（origin + hub + agent-a + agent-b），不进每次 PR 的必跑检查。需要时在本机执行 `bash e2e/hub-smoke.sh`。
