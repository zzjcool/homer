# 按机器适配器停用（per-agent adapter override）契约（冻结 v1）

> 2026-10-12。本文档是唯一接口；worker 发现矛盾 → 停止报告，不自行解释。
> 计划文档看不到 → 向父会话要内容，禁止新建同名文件。
> 前置阅读：docs/plan/2026-10-11-plugin-contract.md（插件体系）、
> SOP homer-plugin-system。

## 目标（用户需求原话）

「为什么适配器插件不可以每个机器独立部署？现在没有对一个机器的卸载入口吗？」
→ 落地为**中心管理、按机器下发差异**：

1. hub 管理员可对**单台机器**停用/启用一个已安装 adapter 插件（不动舰队）。
2. 停用的语义 = `Enabled=false` 声明下发（机器保留数据与本地文件，不再同步），
   不是删除——信任边界不变（覆盖层只在 hub 侧，机器不能自己改）。
3. collect/dispatch 对停用机器自动排除该 adapter；机器视图可见停用态。

## 硬约束（继承插件体系契约）

- 机器端零改动面**扩大**：本次允许改 `internal/agentd/executor.go` 的
  `applyConfigPolicy`（停用语义的一行变化，见下），其余 agentd/hub/stream/
  sync/engine/core/manifest 文件一律不动。
- 协议版本不升（不新增 method；全部经现有 pull/push/status 管道流动）。

## 数据模型（hub 侧覆盖层）

```go
// internal/web/agent_overrides.go（新文件，W1 所有）

// AgentOverrides 持久化在 <home>/agent-overrides.json，0600，原子写。
// 形状 {"schemaVersion":1,"disabled":{"<agentID>":{"<adapterID>":true}}}
// 仅存「停用」事实；启用 = 删除条目（回到中心世代默认）。
type AgentOverrides struct { /* 内部字段，并发安全 */ }

func LoadAgentOverrides(home string) *AgentOverrides
func (o *AgentOverrides) SetDisabled(agentID, adapterID string, disabled bool) error // 持久化
func (o *AgentOverrides) Disabled(agentID, adapterID string) bool
func (o *AgentOverrides) DisabledAdapters(agentID string) []string // 排序副本
```

校验：agentID 非空；adapterID 过 `core.ValidAdapterID`；只能停用**已安装的
adapter role 插件**（keyring/carrier 不允许停用——密钥随行语义；action 不适用）。

## Web API（W1）

| 路由 | 方法 | 行为 |
|---|---|---|
| /api/agents/<id>/adapters | GET | `{adapters:[{id, name, disabled, machineCount 不需要}]}`——该机器视角：中心已安装 adapter ∪ 该机器上报（来自 B2 ReportedAdaptersSource），每项 disabled 来自覆盖层 |
| /api/agents/<id>/adapters/<adapterID> | POST | body `{"disabled":true|false}`；true→SetDisabled，false→删除条目；校验失败 400/422 `code:"override-invalid"`（停用非 adapter 插件/未安装插件/非法 id） |

路由挂在 handlers.go 的 `/api/agents/` 通配分支内（handleAgentRoute 模式），
继承 /api/* 宽闸。**gate_matrix_test.go 补 GET 行 + POST 行**（POST 用合法凭证
断言 4xx 而非 401——覆盖层 Server 未接时允许 503，同 plugins 模式）。

`web.ServeOptions` 新增 `Overrides *AgentOverrides`（nil 全兼容）。
生产装配：`internal/cli/hub.go` runServe 里 `Overrides: web.LoadAgentOverrides(options.Home)`
（W1 改，与 Plugins 同一行区域）。

## dispatch/collect 过滤（W1）

1. **单机下发**（`/api/agents/<id>/pull`）：`readSyncScope` 之后、
   `requireCenterAdapters` 之前：若 scope.Explicit 且请求含该机器停用的
   adapter → 400 `code:"adapter-disabled-for-agent"`，error「<adapter> 已在这台机器停用」。
   （不静默过滤——显式选择被停用项应当报错，与 requireCenterAdapters 同模式。）
2. **扇出下发**（`/api/sync?direction=dispatch`）：`fanoutPullOnlineAgents` 对每台
   机器把停用 adapter 从 scope 中剔除（显式 scope 剔空 → 该机器 skipped，理由
   `adapters disabled for this agent`）；全默认 scope 同样剔除。
3. **收取**（collect choices / /api/sync collect）：机器上报的 adapters 在构建
   choices 时过滤掉该机器停用项（`BuildCollectChoices` 调用前过滤 report.Adapters，
   或在 choices 构建后置 Enabled=false+Reason「已在这台机器停用」——采用**后者**，
   保留可见性：用户能看到「这台机器停用了 pi」而不是 pi 凭空消失）。
4. **机器视图**（/api/agents/<id>/status 的消费侧）：UI 层呈现（W2），web 不改。

## 机器侧停用语义（W1，applyConfigPolicy 一行变化）

`internal/agentd/executor.go applyConfigPolicy`：center 有而本地没有的 adapter
无条件加回——**当且仅当**该 adapter 不在 hub 下发的停用集合时。停用集合怎么到
机器？**不新增协议字段**：hub 在 dispatch 时若该机器有停用 adapter，把世代 meta
中该 adapter 的 `Enabled` 置 false 后下发（scoped dispatch 已按 scope 过滤
adapters，meta 是整包——W1 在 `publishPluginGeneration` 之外新增只读变换：
`taskOptionsForScope` 不动，改在 web 下发 meta 的拼装处（executor 下载快照时
meta 来自 /api/snapshot 的世代 meta——查证：hub 侧 `handleSnapshotDownload`
直接回世代 meta。W1 需在该处按请求机器的覆盖层动态改写 meta 中停用 adapter 的
Enabled=false 后返回；世代本体不动）。机器 bootstrap 收到 Enabled=false 的
adapter：加入本地 homer.json 时保留 Enabled=false（现状代码加回时不看 Enabled
——加回 + 保留声明原样即可，ScanAdapter 已跳过 disabled）。
**若查证后发现 meta 动态改写不可行（如 ETag/缓存耦合），降级方案**：
bootstrap 不改，停用过滤只在 hub 侧 choices/dispatch 生效（机器仍有 adapter
声明但永远不被下发/收取）——语义弱化可接受，verdict 里说明即可。

## UI（W2）

机器「查看」抽屉（openMachineView）：每个 adapter 分组标题行加「停用/启用」
开关按钮（small-btn，与现有风格一致）：
- 点击 → 确认弹窗（复用 #dlg-confirm 三段式）→ POST /api/agents/<id>/adapters/<aid>
- 停用态显示徽章「已停用」，分组内容照常展示（数据还在）
- keyring/ssh-key 不出现开关（后端也不允许）
- collect 弹窗中「已在这台机器停用」的行置灰 + Reason 展示（choices 已带）

e2e：tests/e2e/agent_overrides_ui_test.go（chromedp，mock 模式同 plugins_ui_test）
+ 既有断言不破坏（dispatch_unlock 的 data-adapter 契约不动）。

## 文件所有权

| Owner | 文件 |
|---|---|
| W1 后端 | internal/web/agent_overrides.go（新）、internal/web/agent_overrides_test.go（新）、internal/web/scope.go（过滤插入）、internal/web/sync.go（fanout 过滤+snapshot meta 改写）、internal/web/handlers.go（路由分发两行）、internal/web/server.go（Overrides 字段）、internal/web/choices.go（collect 停用行）、internal/web/gate_matrix_test.go（两行）、internal/cli/hub.go（装配一行）、internal/agentd/executor.go（仅 applyConfigPolicy 的 Enabled 语义，若采用 meta 改写方案则不改）、W1 的测试 |
| W2 UI | internal/web/static/index.html、tests/e2e/agent_overrides_ui_test.go（新）、（如需）既有 ui e2e 的小修 |

W1 与 W2 无共享文件，**并行派发**；W2 的 API 契约以上表为准（等 W1 落地前 W2
可先用 mock fetch 开发）。

## 验收

- W1：`go build ./...`；`go vet ./internal/web/... ./internal/cli/... ./internal/agentd/...`；
  `go test ./internal/web/ ./internal/cli/... ./internal/agentd/`全绿；新增测试：
  覆盖层持久化往返、非法停用（keyring/未安装/坏 id）、单机 pull 400、扇出剔除+skipped、
  collect choices 停用行、（若走 meta 改写）snapshot meta 的 Enabled 改写单测。
- W2：`go test ./internal/web/`；新 e2e：开关渲染、停用确认流、停用后 collect 行置灰。
- 父会话终验：全量 `go test ./...` + e2e 抽查 + 合并 + 发 v1.4.1。

## 集成顺序

W1、W2 并行 → 父会话合并（W1 先，UI 依赖其端点真实形状时以 W1 实现为准对齐）
→ 父会话补缝隙：README 插件节补「按机器停用」小节、api-reference 补两端点、
SOP 更新 → 发版 v1.4.1。
