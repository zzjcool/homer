# SOP：homer 发布与跨平台二进制纪律

> 2026-10-10 Mac agent 变砖事故沉淀。适用：打 tag 发 Release、升级 agent、二进制变砖恢复。

## 发布纪律

合并到 `master` 前 CI 应全绿；打 `v*` tag 才跑 goreleaser 发布工作流。见 [ci.md](ci.md)。

1. **Release 资产只能由 goreleaser CI 产出**。本地交叉编译的二进制（即使 Mach-O 逐字段一致）
   不进正式 Release——构建环境差异（公证/quarantine 上下文）无法靠 diff 排除。
   事故案例：2026-10-10 本地编译的 darwin/arm64 进 Release，Mac 上 exec format error。
2. `gh release create` 预传资产后，CI goreleaser 会因资产重名 422 失败，且 rerun 不幂等。
   要么不预传资产等 CI 全量产出，要么 `gh release delete-asset` 后 rerun。
3. 版本号注入：`go build -ldflags "-s -w -X main.version=$(git describe --tags)"`。
   裸 `go build` 是 dev；控制台的「落后版本」对比依赖真实版本号上报。

## Mac 恢复命令（agent 变砖时）

```bash
curl -fsSL https://github.com/zzjcool/homer/releases/latest/download/homer_darwin_arm64.tar.gz -o /tmp/homer.tar.gz \
  && tar xzf /tmp/homer.tar.gz -C /tmp \
  && mv /tmp/homer ~/.local/bin/homer.new && mv ~/.local/bin/homer.new ~/.local/bin/homer
# 若 exec format error 先清 quarantine：
xattr -d com.apple.quarantine ~/.local/bin/homer
~/.local/bin/homer --version && nohup ~/.local/bin/homer agent --hub https://homerhw.openaaas.org > /tmp/homer-agent.log 2>&1 &
```

## 升级语义（v1.3.5 起）

- Linux/macOS：hub 控制台「更新程序」下载后 `syscall.Exec` 原地换进程（`reexec_unix.go`），
  即点即生效，无需重启。
- 跨平台：hub 只有自己的平台二进制，其它平台从 GitHub Releases 代理（`serveReleaseBinary`），
  带 `X-Homer-Platform` 响应头，agent 侧校验拒绝错架构。
- Windows：`reexec_other.go` 空实现，升级后需手动重启。

## hub 自身升级与重启（控制台一键完成）

控制台右上角 **Hub** 按钮可以直接「更新并重启」或「仅重启」中心，不必登录机器。

- **更新并重启**：从 GitHub Releases 拉最新版（带 `checksums.txt` SHA256 校验）→ 换二进制 →
  重启本机 agent → 原地重启 hub。源码构建（`version == dev`）的 hub 会拒绝此操作：
  那会把 dev 降级成 release，请走下面的手工流程。
- **仅重启**：只重启 hub，不碰 agent 进程。
- 重启方式 = `syscall.Exec` **原地换进程（PID 不变）**。这与 `systemctl --user restart`
  的区别很重要：不依赖 unit 的 `Restart=` 策略，`Type=simple` / `Restart=always` /
  `Restart=no` 三种配置行为一致，也不会和 supervisor 抢端口。
- 重启会先给所有 agent 发 `close 1001 "hub restarting"`，agent 自动退避重连，无需人工干预。
  正在进行的同步会被中断（最多丢掉一次约 12 分钟的同步），恢复后重新发起即可。
- 前端以 `/api/health` 的 `bootId` 变化判定重启成功（纯重启版本号不变，所以不能看版本）。

**防崩溃循环**：换二进制前会把现程序备份成 `<binary>.prev`，并在 `keys/.hub-boot`
写启动标记。若同一份新二进制连续两次没活到监听成功，会自动回滚到 `.prev` 并再 exec 一次，
控制台会显示「已回滚到上一个版本」。同一版本只回滚一次，不会循环。

**没有 supervisor 时**（裸 `nohup`、容器 PID 1、macOS 终端跑）：exec 失败不会让 hub 消失，
它会重新绑定端口继续服务并在日志里报错。检测到 systemd（`INVOCATION_ID` 非空）才交回给
supervisor。

### 手工升级（源码构建 / dev 版本 / 无网络时）

```bash
cd ~/code/homer && git pull
go build -ldflags "-s -w -X main.version=$(git describe --tags)" -o /tmp/homer ./cmd/homer
install -m 755 /tmp/homer ~/.local/bin/homer.new && mv ~/.local/bin/homer.new ~/.local/bin/homer
systemctl --user restart homer-serve   # hub；同机 agent 也跑这个二进制，记得 restart homer-agent
```

注意：手工替换二进制后如果**不**重启 hub，`/dl/homer` 会流式下发的仍是磁盘上那份新二进制
（`serveSelfBinary` 读当前文件而非启动快照），但 hub 进程本身仍跑旧代码。
