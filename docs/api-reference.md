# Hub API 参考（部分）：插件管理与适配器下发

> **范围说明**：本文**不是**完整的 Hub API 清单，只覆盖插件管理与按 adapter 范围下发这两组
> 端点。hub 还实现了约 40 个 `/api/*` 路由（`/api/snapshot`、`/api/keys/*`、`/api/resolve/*`、
> `/api/storage/*`、`/api/auth/*`、`/api/console`、`/api/config`、`/api/health` 等），
> **尚未在本文记录**；对接前请以源码 `internal/web/handlers.go` 的路由表为准。

本文记录插件 API 与按 adapter 范围下发的用法。除特别说明外，`/api/*` 端点需要 hub
Bearer token；示例用 `HUB` 和 `HOMER_HUB_TOKEN` 代表 hub 地址与 token：

```sh
export HUB=http://127.0.0.1:7760
export HOMER_HUB_TOKEN='<hub-token>'
```

通用错误响应使用 `error.code`、`error.message` 和 `error.details`：

```json
{"error":{"code":"bad-request","message":"请求参数无效","details":[]}}
```

## `GET /api/plugins`

返回已安装插件和可安装的官方插件。`available` 只包含官方目录中尚未安装的插件；
`installed` 可包含官方插件与已安装的第三方 manifest 插件。

```sh
curl -sS "$HUB/api/plugins" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN"
```

```json
{
  "schemaVersion": 1,
  "installed": [
    {
      "id": "claude",
      "role": "adapter",
      "name": "Claude Code",
      "description": "Claude Code 的配置同步",
      "adapter": {
        "root": "~/.claude",
        "categories": {
          "settings": {
            "kind": "file",
            "paths": ["settings.json"],
            "mode": "merge"
          }
        }
      },
      "machineCount": 2
    }
  ],
  "available": [
    {
      "id": "ssh-key",
      "role": "action",
      "name": "登录公钥",
      "description": "把 GitHub 用户公钥写入机器 authorized_keys",
      "action": {
        "method": "ssh-key",
        "form": [
          {
            "field": "githubUser",
            "label": "GitHub 用户名",
            "required": true,
            "placeholder": "例如 octocat"
          }
        ]
      },
      "machineCount": 0
    }
  ]
}
```

每个条目用 `role` 标识 `adapter`、`carrier` 或 `action`；适配器配置位于 `adapter`，动作
表单位于 `action`。`machineCount` 是在线且上报过该 adapter ID 的机器数，不是插件安装次数；
没有机器覆盖数据时字段可能省略，界面按 0 显示。非 adapter 插件没有 adapter 覆盖数。

## `POST /api/plugins/install`

安装官方插件时传 `id`。成功返回 `{ "ok": true, "plugin": {...} }`。

```sh
curl -sS -X POST "$HUB/api/plugins/install" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"id":"ssh-key"}'
```

```json
{
  "ok": true,
  "plugin": {
    "id": "ssh-key",
    "role": "action",
    "name": "登录公钥",
    "description": "把 GitHub 用户公钥写入机器 authorized_keys",
    "action": {
      "method": "ssh-key",
      "form": [
        {
          "field": "githubUser",
          "label": "GitHub 用户名",
          "required": true,
          "placeholder": "例如 octocat"
        }
      ]
    }
  }
}
```

第三方 v1 安装使用 `manifest`，其 `schemaVersion` 必须为 1，且 `role` 只能是 `adapter`。
manifest 顶层的 `root`、`categories` 会转换为返回对象的 `plugin.adapter`：

```sh
curl -sS -X POST "$HUB/api/plugins/install" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{
    "manifest": {
      "schemaVersion": 1,
      "id": "claude",
      "role": "adapter",
      "name": "Claude Code",
      "description": "Claude Code 的配置同步",
      "root": "~/.claude",
      "categories": {
        "settings": {
          "kind": "file",
          "paths": ["settings.json"],
          "mode": "merge"
        },
        "commands": {
          "kind": "dir",
          "paths": ["commands/"],
          "mode": "mirror"
        }
      }
    }
  }'
```

已知错误包括：

- HTTP 422：schema 版本不支持
- HTTP 422：第三方 role 不是 adapter
- HTTP 409：ID 与官方或已安装插件冲突
- **HTTP 404，`code` 为 `plugin-not-found`**：`id` 在官方插件目录中不存在，且请求未提供
  `manifest`。错误文案为「官方插件不存在；第三方插件请提供 manifest」。
  ⚠️ 调用方注意：这一条是 404 而不是 422，**不要**把 `plugin-not-found` 当成网络错误去重试。

adapter/carrier 安装会发布新的中心配置世代；action 安装只
更新插件状态。

## `POST /api/plugins/uninstall`

卸载 action 直接传 `id`。成功时返回 HTTP 200 且 `ok` 为 `true`：

```sh
curl -sS -X POST "$HUB/api/plugins/uninstall" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"id":"ssh-key"}'
```

```json
{"ok":true,"plugin":{"id":"ssh-key","role":"action"}}
```

adapter/carrier 卸载还需显式传 `{"id":"claude","force":true}`，并通过数据守卫；例如中心仍有该
adapter 的数据，或 keyring 中仍有绑定到已安装 adapter 的密钥时，返回 HTTP 409、`code` 为
`uninstall-guard`。`force` 不会绕过守卫。卸载不会删除机器上的本地文件；v1 也没有清理中心
store 的 API。当 hub 的 `ServeOptions` 未接入插件状态（嵌入式模式）时，install 与 uninstall
返回 HTTP 503、`code` 为 `plugins-disabled`；GET 返回 200 的空表（`available` 为空）。

## 按 adapter 范围下发：`POST /api/sync?direction=dispatch`

将中心当前世代中指定 adapter 的内容下发给在线机器。把 adapter ID 放在 JSON body 的
`adapters` 数组中；`confirm=true` 表示已确认执行：

```sh
curl -sS -X POST "$HUB/api/sync?direction=dispatch&confirm=true" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"adapters":["pi"]}'
```

```json
{
  "ok": true,
  "status": "synced",
  "direction": "dispatch",
  "agents": [
    {
      "agentId": "laptop-1",
      "hostname": "laptop-1",
      "ok": true,
      "skipped": false
    }
  ],
  "errors": null
}
```

一次请求可传多个 ID，例如 `{"adapters":["pi","vscode"]}`。离线机器会标记为跳过，单台机器
失败不会中止其他机器；整体 `status` 可能是 `partial`。未确认时返回 `aborted`；中心尚无快照、
adapter ID 无效或下发触发密钥守卫时，请先处理相应错误再重试。插件页的「下发到全部机器」
按钮复用此端点，不会新增 agent/stream 协议。

## `GET /api/agents/<id>/adapters`

> **本端点不校验机器是否存在。** 未知 `agentId` 同样返回 200 与「中心已安装 adapter」列表
> （此时「该机器最后上报的 adapter」集合为空，响应退化为纯中心列表），因此**不能**把它当作
> 机器存在性探针。需要判断某台机器是否已接入，请用 `GET /api/agents`。
> 这是与 `POST /api/agents/<id>/adapters/<adapterID>` 不对称的地方：POST 侧有幽灵机器防御，
> 未知 agentId 返回 404 `agent-not-found`。

某台机器视角的适配器列表（中心已安装 adapter ∪ 该机器最后上报的 adapter）。
`keyring`/`ssh-key` 永不出现。每项：`{id, name, disabled, installed}`——
`installed` 区分「中心已安装（可停用）」与「仅机器上报的陈旧行（只能启用）」。

```sh
curl -sS -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  "$HUB/api/agents/<agentID>/adapters"
```

```json
{"adapters":[{"id":"pi","name":"pi","disabled":false,"installed":true}]}
```

Agents 未启用时 501 `agents-disabled`。

## `POST /api/agents/<id>/adapters/<adapterID>`

按机器停用/启用一个适配器插件。停用 = 该机器不再同步此适配器（collect/dispatch
自动排除、扇出跳过）；本地文件保留；机器侧声明不动（hub 侧覆盖层方案）。

```sh
curl -sS -X POST "$HUB/api/agents/<agentID>/adapters/pi" \
  -H "Authorization: Bearer $HOMER_HUB_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"disabled":true}'
```

- 200 `{"ok":true}`；400/422 `override-invalid`（坏 body、非法 adapterID、
  未安装或非 adapter 角色、幽灵机器 404 `agent-not-found`）
- 覆盖层未接（嵌入式）503 `overrides-disabled`
- 显式下发停用项：`POST /api/agents/<id>/pull` 返回 400 `adapter-disabled-for-agent`
- 扇出跳过：`POST /api/sync?direction=dispatch` 响应 `agents[].skipped=true` +
  `reason: "adapters disabled for this agent"`
