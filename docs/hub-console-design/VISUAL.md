# homer hub 控制台 · 视觉与动效规范（Kim 锁定）

> 给 Fiona 落地用。切片顺序：①视觉骨架 → ②可视化 → ③细动画。  
> 伞单：[#44](https://github.com/zzjcool/homer-cli/issues/44)。交互信息架构仍以 PR [#3](https://github.com/zzjcool/homer-cli/pull/3) `INTERACTION.md` 为准；本文件只锁**好看、可视化、动效**。  
> 现网 `internal/web/static/index.html` 已吃进 PR#3 色板与 hero 骨架，本规范在此基础上升级，不推倒重来。

---

## 0. 产品语气

- **冷静的巡检台**：干净时无聊、安静；有事时只有一个强调色动作。
- **黑白灰承担结构；颜色只编码语义**（同步 / 漂移 / 冲突 / 离线）。
- **动画是反馈，不是装饰**：默认 ≤220ms；必须尊重 `prefers-reduced-motion`。

---

## 1. 色板（含暗色）

骨架 PR 必须同时交付浅色 + 暗色 token。默认跟随 `prefers-color-scheme`；页眉可留「外观」三态切换（系统 / 浅 / 深），写入 `localStorage` 键 `homer-console-theme`。

### 1.1 浅色

| Token | 值 | 用途 |
|---|---|---|
| `--ink` | `#141414` | 正文 |
| `--ink-dim` | `#6b6b6b` | 次要 / 时间戳 |
| `--line` | `#e4e4e4` | 1px 分隔 |
| `--bg` | `#ffffff` | 页底 |
| `--bg-sink` | `#f4f4f4` | diff / 抽屉 / 斑马 |
| `--bg-elev` | `#ffffff` | dialog 浮层 |
| `--accent` | `#0a7d4f` | 同步 / 主按钮（白字 AA） |
| `--accent-d` | `#086840` | 主按钮 hover |
| `--accent-soft` | `#0a7d4f18` | 浅底强调、meter 填充底 |
| `--warn` | `#9a6700` | 漂移语义（非阻断；与 accent 区分） |
| `--warn-soft` | `#9a67001a` | 漂移浅底 |
| `--danger` | `#b3261e` | 冲突 / 错误 |
| `--danger-soft` | `#b3261e14` | 冲突浅底 / del 行 |
| `--ok` | `#0a7d4f` | 与 accent 同义，圆点「干净/在线」 |
| `--offline` | `#9a9a9a` | 离线圆点与整行降灰 |

### 1.2 暗色

| Token | 值 | 用途 |
|---|---|---|
| `--ink` | `#e8e8e8` | 正文 |
| `--ink-dim` | `#9a9a9a` | 次要 |
| `--line` | `#2e2e2e` | 分隔 |
| `--bg` | `#121212` | 页底 |
| `--bg-sink` | `#1c1c1c` | 凹陷面 |
| `--bg-elev` | `#1f1f1f` | dialog |
| `--accent` | `#3dba7e` | 主按钮/同步（暗底对比） |
| `--accent-d` | `#2f9a66` | hover |
| `--accent-soft` | `#3dba7e22` | 浅底 |
| `--warn` | `#e3b341` | 漂移 |
| `--warn-soft` | `#e3b34122` | |
| `--danger` | `#f07178` | 冲突 |
| `--danger-soft` | `#f0717822` | |
| `--ok` | `#3dba7e` | |
| `--offline` | `#6b6b6b` | |

### 1.3 Hero 四态用色（锁定）

| 态 | 圆点 | 结论字色 | 辅助 / 注 | 主 CTA |
|---|---|---|---|---|
| **干净** | `--ok` | `--ink` | `--ink-dim` 数字行 | **无按钮** |
| **漂移** | `--warn` | `--ink`（结论不染色） | 数字行可含 ↑↓ | 唯一 `primary`「处理漂移 →」用 `--accent` |
| **冲突** | `--danger` | `--danger`（`.hero.warn`） | `--danger-soft` 左边条 note | **无推拉按钮**；note 内终端指引 |
| **未初始化** | `--offline` | `--ink` | `.hero-note.info` 左边条 `--ink-dim` | 无主按钮（终端 `homer init`） |
| **无 agent**（#47） | `--offline` | `--ink` | info note | 唯一 `primary`「接入命令 →」 |

规则：一页同时最多一个 `button.primary`。干净态强调色只出现在圆点上。

---

## 2. 字体与间距节奏

```
字族
  UI：system-ui, -apple-system, "Segoe UI", sans-serif
  等宽：ui-monospace, "SF Mono", Menlo, Consolas, monospace
        （diff / 接入命令 / agentId / 计数）

字阶
  --fs-hero:  1.5rem     横幅结论（窄屏 1.25rem）
  --fs-title: 1.05rem    页眉站名
  --fs-body:  0.9375rem  正文 / 行
  --fs-small: 0.8125rem  标签 / 时间戳 / section h2

字重
  结论 650；hostname 600；section h2 600 + uppercase + letter-spacing .06em
  正文 400；等宽计数 500

行高：正文 1.5；hero 结论 1.25；diff 1.45

间距（4px 基网）
  常用：4 / 8 / 12 / 16 / 24 / 32
  页边：桌面 page max-width 880px、左右 24；窄屏左右 16
  section 上下 padding 20（现网 18 → 略松）
  机器行 / 适配器行 内边 10×4；行间靠 border-top，不靠大 margin

圆角：统一 4px（按钮 / 输入 / 抽屉 / dialog）；meter 轨 99px
触控：主按钮高 ≥40px；次按钮 / small-btn ≥32px
```

---

## 3. 动效曲线（锁定）

### 3.1 Token

```css
:root {
  --ease-out: cubic-bezier(0.2, 0.8, 0.2, 1);
  --ease-sheet: cubic-bezier(0.22, 1, 0.36, 1);
  --dur-1: 120ms;   /* hover / 焦点环 */
  --dur-2: 180ms;   /* 按钮态、opacity、drawer 高 */
  --dur-3: 220ms;   /* hero 切换、区块展开 */
  --dur-4: 320ms;   /* dialog 入场（唯一允许偏慢） */
}
```

### 3.2 允许的动画

| 场景 | 属性 | 时长 | 说明 |
|---|---|---|---|
| 按钮 hover/active | background, border-color | `--dur-1` | 无位移 |
| Hero 态切换 | 结论/数字 `opacity` + 轻微 `translateY(4px→0)` | `--dur-3` | 交叉淡入；不整页闪白 |
| 适配器/机器行展开 | `grid-template-rows` 或 max-height + opacity | `--dur-2` | |
| Diff 抽屉打开 | 同上 | `--dur-2` | |
| Dialog 打开 | opacity + scale(.98→1) | `--dur-4` / `--ease-sheet` | |
| Meter 填充 | width | `--dur-3` | 首次进入与数据刷新 |
| Toast | opacity + translateY | `--dur-2` | |
| 刷新 spinner | 现有 `spin` | 700ms linear | 仅按钮内小图标 |
| 骨架脉冲 | 现有 `pulse` | 1s alternate | 仅首屏骨架 |

**禁止**：大面积 parallax、弹跳 bounce、循环闪烁强调色、超过 400ms 的入场、页面切换整屏 slide。

### 3.3 `prefers-reduced-motion: reduce`

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation: none !important;
    transition: none !important;
    scroll-behavior: auto !important;
  }
}
```

现网已有此门闩——保留，新增动画不得另开旁路。

---

## 4. 可视化图元（切片 ②）

在现有 `.meter` 条基础上统一三件套，避免再堆纯数字卡：

1. **Hero 摘要条（漂移/冲突时）**  
   一条分段 meter：`push | pull | conflict` 按占比着色（`--warn` / `--accent` / `--danger`），右侧等宽计数。干净态不显示 meter。

2. **适配器行迷你条**  
   行尾 72×6px meter：漂移占比；冲突时整段 `--danger`。干净行不显示条，只留灰字「干净」。

3. **机器行状态**  
   圆点语义不变；可选右侧极细「相对中心」差额条（有漂移才出现）。不做重拓扑图；若要「舰队感」，用机器列表上方一行 **在线/离线/漂移** 三枚小徽标计数即可。

Diff 着色：`+` → `--accent` + `--accent-soft` 底；`-` → `--danger` + `--danger-soft` 底（与现网一致，暗色换 soft token）。

---

## 5. 组件边界（Kim / Fiona）

| 块 | Kim（规范/设计稿） | Fiona（实现） |
|---|---|---|
| Token + 暗色 + 主题切换 | 本文件锁定 | ①骨架 PR：`:root` / dark / 主题切换 |
| Hero 四态 + 无 agent | 用色与文案见上；交互见 #47/#48 | 按态上色 + 唯一 CTA |
| 适配器总览 | 可视化图元见 §4；交互见 #51 | 落地 sec-adapters |
| 机器行渐进披露 | 动效见 §3；交互见 #49 | 状态显隐 + 过渡 |
| Diff 抽屉绑定操作 | 动效见 §3；交互见 #50 | 抽屉 + 按钮绑定 |
| agentId 默认露出 | #46 | `.aid` 对比度暗色下 ≥ AA |
| 页眉版本 | — | #45 小修（插空） |

**Fiona 第 1 个 PR 只做 ①视觉骨架**：token、暗色、字阶间距微调、hero 四态上色对齐本表；不改信息架构、不加新可视化图元、不加新动画（可保留现有 spin/pulse）。

---

## 6. 对照现网 vs PR#3（设计差距摘要）

| 项 | 现网 | PR#3 原型 | 本规范 disposition |
|---|---|---|---|
| 色板 | 浅色 PR#3 token | 同左 | **升级 + 暗色** |
| Hero | 有；漂移 CTA 偏弱 | 唯一强调色按钮 | 跟 #48 + 本表四态色 |
| 适配器总览 | 无独立舰队区 | 有 | 跟 #51 + §4 迷你条 |
| 机器操作 | 常驻收取/下发 | 按态披露 | 跟 #49 |
| Diff 绑定 | 冲突路径有；日常漂移弱 | 抽屉绑定 | 跟 #50 |
| 动效 | spin/pulse + 弱 transition | 几乎无 | **§3 时间表** |
| 暗色 | 无 | 无 | **必须补** |
| 无 agent hero | 易显「全部对齐」假成功 | 机器区空态 | 跟 #47 |
| agentId | `.aid` 有但弱 | 有 | 跟 #46，暗色提高对比 |

交互类缺口已有 #46–#51，**勿重复开同题**。本规范配套新开的是视觉/动效切片 issue（见 PR 描述）。

---

## 7. 验收（设计侧）

- [ ] Fiona 骨架 PR 的 CSS 变量表与本文件 §1–§3 一致（可差 1–2 个衍生色，但四态映射不改）
- [ ] 暗色下正文/主按钮/冲突字对比均过 AA
- [ ] `prefers-reduced-motion: reduce` 时无位移动画
- [ ] 截图：浅/暗 × 干净/漂移/冲突/未初始化/无 agent 至少各一
