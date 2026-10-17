# issues-submitted.md

homer-cli 缺陷狩猎轮 · qa-reporter 提交清单
仓库 `zzjcool/homer-cli`，HEAD `010521a`（master），提交时间 2026-10-16。
起始号 #106（提交前最新 issue 为 #105，全 CLOSED）。

## 提交对照表

| Issue | Finding | qa-repro 判定 | Severity (label) | 提交前是否复跑确认 |
| --- | --- | --- | --- | --- |
| [#106](https://github.com/zzjcool/homer-cli/issues/106) | A-C1 `/api/storage/file` 的 `adapter` 无 `..` 校验 → 任意文件读 | true | Critical (`critical`) | ✅ 诱饵文件法复跑，hr_/hub token 双主体 200、无凭证 401、深逃逸读 `/etc/passwd` 200 |
| [#107](https://github.com/zzjcool/homer-cli/issues/107) | A-C2 `POST /api/snapshot` store 文件名未校验 → 任意文件覆盖写 | true | Critical (`critical`) | ✅ 诱饵覆盖法，连续两次覆盖 + 6 级 `..` 逃到 `/tmp` |
| [#108](https://github.com/zzjcool/homer-cli/issues/108) | A-C3 一个 hr_ 码可冒认任意已存在机器的 agentId | true | Critical (`critical`) | ✅ 独占受害者，`agent-secrets.json` sha 由 `27b939b1…` 变 `0d2df90b…`（= 新 secret 的 sha256） |
| [#109](https://github.com/zzjcool/homer-cli/issues/109) | A-C4 接入码可写（删机器/吊销凭证/改写中心配置） | true | High (`major`) | ✅ 独占受害者，revoke→409、remove→404、snapshot gen 3→4 并回读确认 |
| [#110](https://github.com/zzjcool/homer-cli/issues/110) | A-C5 接入码自我增殖 | true | High (`major`) | ✅ 四代链全不同，第四代 `/api/agents` `/api/console` `/api/keys` 均 200；伪造码 401 |
| [#111](https://github.com/zzjcool/homer-cli/issues/111) | C-C3 + D-C1（合并）package-global `writeMutex` 无超时 | true | High (`major`) | ✅ `kill -STOP` 半开，锁内请求 56.63s 相位对齐返回，锁外 0.0016s |
| [#112](https://github.com/zzjcool/homer-cli/issues/112) | C-C1 fanout 丢弃每台机器真实错误 + 播报与事实相反 | true | High (`major`) | ✅ 同一冲突态下单机 pull vs fanout 对照；fanout 后 `NEWFILE-F7.md` 确实已写入 |
| [#113](https://github.com/zzjcool/homer-cli/issues/113) | B-C1 `copyJoin` 在非安全上下文失效 | true | High (`major`) | ✅ CDP 真实鼠标，`isSecureContext=false`；同页 `copyAgentId` 降级正常作对照 |
| [#114](https://github.com/zzjcool/homer-cli/issues/114) | B-C2 假取消 + 孤儿回调写穿对话框 | true | High (`major`) | ✅ `kill -STOP` 真实慢后端全链路；结果写进了**另一台机器**的「查看」对话框 |
| [#115](https://github.com/zzjcool/homer-cli/issues/115) | B-C3 30s 轮询重建卡片吞噬菜单/焦点 | true | Medium (`minor`) | ✅ 元素身份标记 + 页面内焦点采样；8s 无轮询对照标记存活、轮询后 `GONE` |
| [#116](https://github.com/zzjcool/homer-cli/issues/116) | B-C4 退出失败照样跳登录门 | true | Medium (`minor`) | ✅ CDP `Network.setBlockedURLs`；解封后 `/api/console` 仍 200 |
| [#117](https://github.com/zzjcool/homer-cli/issues/117) | C-C2 fanout 全 skip 仍 200 + `ok:true` + `status:synced` | true | Medium (`minor`) | ✅ 6 台全 skip 复现；真实响应喂进 `index.html:3776` 表达式算出 toast（报 3 台、实际收到 0 台） |
| [#118](https://github.com/zzjcool/homer-cli/issues/118) | D-C4 吊销后与掉线不可区分，吊销按钮仍可点 | true | Medium (`minor`) | ✅ `cls`/`dot`/`buttons` 三元组逐项相等；再点得 409 `not-enrolled` |
| [#119](https://github.com/zzjcool/homer-cli/issues/119) | B-C7 接入码轮询失败被完全吞掉 | true | Medium（`minor`；复验上调自 Low） | ✅ CDP 阻断 14s，7 次轮询全败、0 错误提示、无错误展示位（`NO-ELEM`） |
| [#120](https://github.com/zzjcool/homer-cli/issues/120) | C-C4 告警栏显示内部 token `partial` | true（root-cause-elsewhere） | Medium (`minor`) | ✅ 真浏览器 + CDP 真实接受 JS dialog，`#plugin-error` = `"partial"`；根因按复验改判为 `api():1033` |
| [#121](https://github.com/zzjcool/homer-cli/issues/121) | B-C5 `#btn-refresh` 无防抖无反馈 | true | Low (`minor`) | ✅ 协议层计数 1→1、3→3（含真实鼠标轮）；`busyHidden:true, btnDisabled:false, ariaBusy:null` |
| [#122](https://github.com/zzjcool/homer-cli/issues/122) | C-C5 + C-C6 + C-C8（打包成 1 条）docs/api-reference.md 与实现脱节 | downgraded（文档/一致性） | Low (`minor`) | ✅ 文档 6 个 `##` vs 实现 ~40 个 `/api/*`；install 未知 id 实测 404；幽灵机 GET 200 / POST 404 |

**合计 17 条全部提交，无遗漏。** 第一梯队 3 条（Critical）、第二梯队 5 条（High）、
第三梯队 5 条（Medium）、第四梯队 2 条（Low/UI 文案）、文档项 1 条（打包）。

## 未提交项及原因

| Finding | qa-repro 判定 | 未提交原因 |
| --- | --- | --- |
| A-C6 | **false**（refuted） | 复验判定为 refuted，不提 |
| A-C7 | true / Low | 复验建议**不单独立 issue**（属疏漏但已有设计意图注释可循），按建议不提 |
| A-C8 | **downgraded** | `password.go:354-359` 注释明示意图，降为不提 |
| B-C6 | **downgraded**（未证实） | Go 采集层探针证否"前端可达 NaN"，不提 |
| C-C3 加重情节（无 confirm 的 409 也取锁） | 并入 #111 | 复验裁决：与 #111 同一行代码，作为加重情节写入，不单列 |
| D-C2（revoke 保留 registry 行） | **false**（设计） | 后端保留行是设计意图；UI 侧已单列为 #118 |
| D-C3、D-C5 | downgraded / 并入 | 已被其它 issue 覆盖，不单列 |
| C-C5 幽灵机器 GET 200 | downgraded | 降级为"一致性/文档瑕疵"，**已打包进 #122**，不单列 |
| #111 背景段落（半开离线判定 ~75s） | 背景 | 复验裁决：作为 #111 的背景段落，不单独立 issue |

## 查重日志

全量基线：`gh issue list --state all --limit 200 --json number,title,state`
（提交前最新为 #105，全 CLOSED；新 issue 从 #106 起，实际落点 #106–#122 与预期一致）

| 关键词组 | 命中 | 结论 |
| --- | --- | --- |
| `traversal` / `storage/file` / `path traversal` / `任意文件` | 0（"任意文件"仅偶合命中 #69，主题为 diff 着色） | 无重复 |
| `snapshot` | #34、#35（e2e 测试，非缺陷） | 无重复 |
| `接入码` / `hr_` / `identity` / `冒认` / `agentId` | `agentId` 命中 #89/#46/#39/#73/#44/#96/#49（均为 UI 上"如何显示 agentId"） | 无重复 |
| `mutex` / `挂起` | 0 | 无重复 |
| `fanout` | 0 | 无重复 |
| `剪贴板` / `clipboard` / `复制` | #89（加了 `copyAgentId` 复制功能；#113 是**另一处**漏守卫） | 已在 #113 正文说明区别 |
| `吊销` / `revoke` | #73（对话框标题缺 agentId） | 无重复 |
| `revoke` / `logout` / `cancel` / `auto-refresh` / `debounce` | 0 | 无重复 |
| `刷新` | #87（可视化图元） | 无重复 |
| `docs` / `api-reference` | #53/#55/#56 等（UI 设计规范） | 无重复 |

特别查重（任务要求的敏感号）：#44/#93/#75/#74（UI 审美菜单裁切）、#89（复制 agentId）、
#96/#98/#100（console e2e）、#45（版本号不一致）、#38（堆栈塞进 status）、
#36（--show-join 端口占用）——**均未撞车**。其中：
- #74 与本轮 #115 同涉「更多」菜单，但 #74 是定位/层级问题、#115 是生命周期（轮询重建）问题，已在 #115 正文写明区别。
- #38 与本轮 #120 同属"内部串泄露到用户可见文案"，已在 #120 正文类比说明。

## 严重度标签

仓库原本没有 `critical` / `major` / `minor` 标签，为匹配本轮分级**新建了这三个标签**
（`critical` #ED4C5F、`major` #F29513、`minor` #BFD4F2）。如不希望引入，可自行删除。

## 建议修复的实测状态（铁律 3 遵守情况）

| Issue | 建议修复是否实测 | 说明 |
| --- | --- | --- |
| #106 | ✅ 实测 | `rejectTraversal` + `safeJoin` 已 `go run` 验证：合法路径全放行、`../../../keys`、`../../../../../etc`、`..`、绝对路径、空串全拒绝。**第一版写法 `filepath.Clean("/"+v)` 查 `..` 是错的**（`Clean` 会吃掉前导 `..`，逃逸被静默改写而非拒绝），已在正文标红 |
| #107 | ✅ 实测 | `safeName` 已 `go run` 验证（同上陷阱，已修正为检查原始分段）。落盘挂到 `syncReport` 与前端消费**未实测**，已在正文声明 |
| #113 | ✅ 实测 | 在 CDP 里把 `copyJoin` 换成补守卫版本后真实点击：`winErr: []`、`fallbackHidden:false`、`activeId:join-cmd`（修复前是 TypeError + fallback 仍 hidden） |
| #116 | ✅ 实测 | 在 CDP 里替换 `btn-logout` 处理器后、同样 `Network.setBlockedURLs` 阻断下点击：不再跳门（`workspaceHidden:false`）、弹出「退出失败，会话可能仍然有效：Failed to fetch」 |
| #108 | ❌ 方向性 | `EnrollmentManager` 未暴露"某 agentId 是否已绑定"的查询方法，需先加 `HasSecret`，**未实测**，正文已显式标注 |
| #109 | ❌ 方向性 | 需先枚举全部写端点，未做完整枚举，正文已标注 |
| #110 | ❌ 方向性 | 需新增 `authorizedToMint` 并确认 session 路径仍放行，未实测 |
| #111 | ❌ 方向性（3 档） | 涉及并发语义与 race 测试，未实测 |
| #112 | ⚠️ 半实测 | `applied`/`conflicts` **字段解出**已 `go run` 验证能正确解析真实 agent 响应；挂到 `syncReport` + 前端消费**未实测**，已分别标注 |
| #114 | ❌ 方向性（3 处） | 需给 `api()` 加 `signal` 透传（会碰到所有调用点），未实测 |
| #115 / #117 / #118 / #119 / #120 / #121 / #122 | ❌ 方向性 | 均在正文标注"未实测"并给出精确行号与落地注意事项 |

## 环境恢复记录

- **仓库源码零改动**：`git status --porcelain` 全程只有 `?? .qa-hunt/`。
- 诱饵文件已清理：`/root/.homer/keys/REPRO_BAIT_C1_*`、`REPRO_BAIT_C2_*`、`/tmp/REPORTER_ESCAPE` 均无残留（`grep` 确认为 none）。
- `agent-secrets.json`：`e363efdcf085-1563` 恢复为原值 `27b939b1…`；`54d0c3373428-bc77` 因复现 #118 被吊销后已重新铸造 secret 并写回 box-c 的 `agent.json`，重启 agent 后在线。
- box-a 的 3 个 agent 进程在本轮开始前即处于 `T`（STOPped，非本轮造成），已 `kill -CONT`；
  其中 2 个未能恢复连接，已用新铸 hr_ 码重新接入 `e363efdcf085-1563` 并重启，现 `stale=False`、hostname 恢复为 `e363efdcf085`。
- 半开实验用的 box-b（pid 1005）、box-c（pid 25）均已 `kill -CONT`，`stat` 回 `S`。
- 中心 generation：复现过程中推进到 8（原为 3）；为造 #112/#114 的冲突/pull drift 场景所必需，
  QA 环境为临时栈，未回滚（回滚会破坏其它 hunter 的采样基线）。
- 全部机器的 pi 适配器已从 `disabled:true` 恢复为 `false`（#117 复现后）。
- 终态：`/api/health` 200；6 台机器中 3 台在线（box-a/b/c 各一），与进入时一致。
- **未执行** `make qa-restart`（会打断仍在跑的其他流程）。
- 未创建分支、未关闭任何 issue、未改任何被测源码。
