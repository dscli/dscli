---
name: jaeger-query
description: Jaeger Query HTTP API 技能 — 通过 slingshot jaeger 命令直接查询 Jaeger traces，无需 MCP Server，零依赖
author: Curie <curie@dscli.io>
keywords:
- jaeger
- tracing
- query
- api
- distributed-tracing
- opentelemetry
- dapper
- slingshot
---

# jaeger-query — Jaeger Query HTTP API 技能

替代 Jaeger MCP Server（不可靠），直接调用 Jaeger Query HTTP API。
使用 `slingshot jaeger` 命令（内嵌在 slingshot CLI 中，无需 curl/jq/python3）。

## 命令

| 命令 | 功能 |
|------|------|
| `services` | 列出所有注册的服务 |
| `operations <service>` | 列出服务的所有操作名称 |
| `search <service> [--limit N] [--tags JSON]` | 搜索 traces |
| `trace <traceID>` | 获取完整的 trace 详情 |
| `deps [--lookback DURATION]` | 获取服务依赖图 |

## 使用场景

### 查看服务
```bash
slingshot jaeger services
```

### 搜索 traces
```bash
slingshot jaeger search Dscli --limit 5
slingshot jaeger search Dscli --limit 10 --tags '{"error":"true"}'
```

### 深入分析
```bash
slingshot jaeger trace <traceID>
```

### 服务依赖拓扑
```bash
slingshot jaeger deps              # 默认 1h
slingshot jaeger deps --lookback 24h
```

## Jaeger API 注意事项

- **时间格式**: epoch 微秒 — `jaeger deps` 自动计算 `endEpoch` 和 `lookback`
- **`/api/dependencies`**: 使用 Go `time.Duration` 解析（1h, 24h, 3600s 等格式）
- **`/api/traces`**: 不带 start/end/lookback 即可返回最近 traces
- **`total` 字段**: Jaeger v2 始终返回 0（已知限制），实际数据在 `data[]` 中
- **tags**: `--tags` 标志自动 URL 编码

## 自定义 Jaeger 地址
```bash
# 环境变量方式
JAEGER_HOST=http://jaeger.example.com slingshot jaeger search Dscli

# 或 --host 标志
slingshot jaeger --host http://jaeger.example.com search Dscli
```

## 典型调试流程

1. 用户报告某个 tool call 慢
2. `slingshot jaeger operations Dscli` — 确认操作名
3. `slingshot jaeger search Dscli --limit 10` — 找最近的 traces
4. `slingshot jaeger trace <traceID>` — 找到对应操作的 span，分析耗时

## 依赖

- **slingshot**（内置，无需额外安装）
- Jaeger Query API（默认 `http://localhost:16686`）
