---
name: pkgsite-api
description: 通过 web_reader toolcall 或 pkgsite-cli 查询 pkg.go.dev API，获取 Go 包/模块的元数据、版本、符号、依赖等信息。
keywords:
- pkg.go.dev
- api
- pkgsite
- go
- package
- module
- metadata
- version
- search
- symbol
- lightpanda
- web_reader
- toolcall
---

# pkgsite-api

查询 pkg.go.dev API 的技能，AI 直接调用 **web_reader toolcall**，
命令行参考见 **pkgsite-cli**。

## 背景

pkg.go.dev 提供了 REST API（`/v1beta/`），可查询 Go 包和模块的元数据。
API 采用纯 GET 架构，无状态，为高效缓存设计。

## API 端点速查

| 端点 | 说明 |
|------|------|
| `/v1beta/package/{path}` | 包信息 |
| `/v1beta/module/{path}` | 模块信息 |
| `/v1beta/versions/{path}` | 模块版本列表 |
| `/v1beta/packages/{path}` | 模块下所有包 |
| `/v1beta/search?q={query}` | 搜索 |
| `/v1beta/symbols/{path}` | 包符号列表 |
| `/v1beta/imported-by/{path}` | 导入该包的包列表 |
| `/v1beta/vulns/{path}` | 已知漏洞 |

可选 `?version=` 参数指定版本（语义版本或 `master`/`main`）。

## web_reader toolcall（AI 直接调用）

作为 AI，直接调用 `web_reader` toolcall 查询 pkg.go.dev API：

```
web_reader(url="https://pkg.go.dev/v1beta/package/github.com/google/go-cmp/cmp")
```

`web_reader` 通过 lightpanda 浏览器抓取，自动返回美化后的 JSON/Markdown，
无需启动外部进程。

端点与 URL 对照：

| 端点 | url 参数 |
|------|----------|
| 包信息 | `"https://pkg.go.dev/v1beta/package/{path}"` |
| 模块信息 | `"https://pkg.go.dev/v1beta/module/{path}"` |
| 版本列表 | `"https://pkg.go.dev/v1beta/versions/{path}"` |
| 搜索 | `"https://pkg.go.dev/v1beta/search?q={query}"` |
| 符号列表 | `"https://pkg.go.dev/v1beta/symbols/{path}"` |

返回示例：

```json
{
  "modulePath": "github.com/google/go-cmp",
  "version": "v0.7.0",
  "isLatest": true,
  "path": "github.com/google/go-cmp/cmp",
  "name": "cmp",
  "synopsis": "Package cmp determines equality of values.",
  "isRedistributable": true
}
```

## pkgsite-cli（命令行参考）

`pkgsite-cli` 是官方参考客户端，可通过 shell 工具调用。

### 安装（如未安装）

```bash
go install golang.org/x/pkgsite/cmd/internal/pkgsite-cli@latest
```

### 常用命令

```bash
# 查看包详情
pkgsite-cli package github.com/google/go-cmp/cmp

# 搜索包
pkgsite-cli search "uuid"

# 查看模块信息和版本
pkgsite-cli module -versions github.com/google/go-cmp

# 同时查看模块的版本和包列表
pkgsite-cli module -packages -versions github.com/google/go-cmp

# 查看哪些包导入了指定包
pkgsite-cli package --imported-by github.com/google/go-cmp/cmp

# 查看包声明的符号
pkgsite-cli package --symbols github.com/google/go-cmp/cmp
```

### 输出示例

```
$ pkgsite-cli package github.com/google/go-cmp/cmp
github.com/google/go-cmp/cmp
  Name:     cmp
  Module:   github.com/google/go-cmp
  Version:  v0.7.0 (latest)
  Synopsis: Package cmp determines equality of values.

$ pkgsite-cli search "uuid"
github.com/google/uuid
  Module:   github.com/google/uuid@v1.6.0
  Synopsis: Package uuid generates and inspects UUIDs.
```

## 两种方式对比

| | web_reader toolcall | pkgsite-cli |
|---|---|---|
| 使用者 | AI 助手 | 命令行 / shell |
| 输出格式 | Markdown（美化 JSON） | 结构化文本 |
| 安装 | 内置 dscli | 需额外安装 |
| 适合场景 | AI 查询、自动化 | 人工阅读、快速查询 |
| 版本指定 | `?version=` 参数 | `@version` 语法 |
| 分页 | 需手动处理 | 自动处理 |

## 注意事项

- API 端点挂在 `/v1beta/` 下，未来可能迁移到 `/v1/`
- API 要求精确指定模块——当包路径多义时返回候选列表而非自动选择
- `pkgsite-cli` 命令行接口尚未稳定，可能随 API 演进变化
