# PocketBase MCP (Go)

> **English** | [中文](#pocketbase-mcp-go-中文说明)

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server for [PocketBase](https://pocketbase.io), written in Go. Expose PocketBase collections, record CRUD, user authentication, and database backups to AI clients via a single binary.

## Features

- **24 MCP tools** — collection/record CRUD, user auth, database backup, OAuth2, OTP, and bulk import
- **Dual transport** — stdio (Cursor, Claude Desktop) or Streamable HTTP (default, `/mcp` endpoint)
- **Single binary** — no Node.js; compile and run anywhere Go builds
- **Tested on PocketBase 0.26+** (acceptance tests pass on v0.39.3)

## Quick Start

### Install

```bash
go install github.com/zouxiaoliang/pocketbase-mcp-go@latest
```

Or clone and build:

```bash
git clone https://github.com/zouxiaoliang/pocketbase-mcp-go.git
cd pocketbase-mcp-go
go build -o pocketbase-mcp-go .
```

### Run (HTTP mode, default)

```bash
export POCKETBASE_URL="http://127.0.0.1:8090"
export POCKETBASE_ADMIN_EMAIL="admin@example.com"
export POCKETBASE_ADMIN_PASSWORD="your_password"
./pocketbase-mcp-go
# Listens on :8080/mcp by default; /health for health check
```

### Run (stdio mode, for Cursor / Claude Desktop)

```bash
./pocketbase-mcp-go -transport=stdio
```

### MCP Client Configuration (stdio)

```json
{
  "mcpServers": {
    "pocketbase": {
      "command": "/path/to/pocketbase-mcp-go",
      "args": ["-transport=stdio"],
      "env": {
        "POCKETBASE_URL": "http://127.0.0.1:8090",
        "POCKETBASE_ADMIN_EMAIL": "admin@example.com",
        "POCKETBASE_ADMIN_PASSWORD": "your_password"
      }
    }
  }
}
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `POCKETBASE_URL` | Yes | PocketBase instance URL |
| `POCKETBASE_ADMIN_EMAIL` | No* | Superuser email |
| `POCKETBASE_ADMIN_PASSWORD` | No* | Superuser password |
| `POCKETBASE_ADMIN_MODE` | No | `true` to execute record CRUD as superuser (default `false`) |
| `MCP_TRANSPORT` | No | `http` (default) or `stdio` |
| `MCP_HTTP_ADDR` | No | HTTP listen address (default `:8080`) |

\* Admin credentials are required for collection management (`list_collections`, `create_collection`, `backup_database`, etc.).

## License

[MIT](LICENSE)

---

<a id="pocketbase-mcp-go-中文说明"></a>

# PocketBase MCP (Go) 中文说明

基于 Go 的 [PocketBase](https://pocketbase.io) [MCP](https://modelcontextprotocol.io) 服务器。通过 MCP 协议向 AI 客户端暴露 PocketBase 的集合管理、记录 CRUD、用户认证、数据库备份等能力，编译为单个可执行文件即可部署。

仓库地址：[github.com/zouxiaoliang/pocketbase-mcp-go](https://github.com/zouxiaoliang/pocketbase-mcp-go)

## 参考项目

本项目基于社区开源项目 [mrwyndham/pocketbase-mcp](https://github.com/mrwyndham/pocketbase-mcp) 用 Go 重新实现。参考项目是一个 **TypeScript / Node.js** 编写的 PocketBase MCP 服务器，基于 PocketBase JS SDK 类型定义，提供集合管理、记录 CRUD、用户认证等 MCP 工具，目前仅支持 **stdio** 传输。

本 Go 版本在工具名称、参数与环境变量上与参考项目保持对齐，便于在 MCP 客户端中切换使用；同时额外提供基于 Gin 的 **HTTP** 传输（默认），并编译为**单二进制**，无需 Node.js 运行时。

| 对比项 | [mrwyndham/pocketbase-mcp](https://github.com/mrwyndham/pocketbase-mcp) | 本项目 |
|--------|--------------------------------------------------------------------------|--------|
| 语言 / 运行时 | TypeScript，需 Node.js | Go，单二进制 |
| 传输方式 | stdio | stdio + HTTP（默认） |
| 工具集 | 24 个 MCP 工具 | 同名 24 个工具 |
| 环境变量 | `POCKETBASE_URL`、`POCKETBASE_ADMIN_*` 等 | 相同 |

## 特性

- **24 个 MCP 工具**：集合/记录 CRUD、用户认证、数据库备份、扩展认证与批量导入
- **双传输模式**
  - `stdio`：兼容 Cursor、Claude Desktop 等 MCP 客户端
  - `http`（默认）：Gin + Streamable HTTP，端点 `/mcp`，附带 `/health` 健康检查
- **单二进制部署**：全部逻辑集中在根目录 `main.go`，构建后即可运行
- **PocketBase 0.26+**：已在 **v0.39.3** 真实实例上完成验收

## 快速开始

### 前置条件

- Go 1.25+
- 运行中的 PocketBase 实例（例如 `http://127.0.0.1:8090`）
- 超级用户（admin）账号（集合管理、备份等操作需要）

### 安装

**go install**（需 Go 1.25+，安装到 `$GOPATH/bin` 或 `$GOBIN`）：

```bash
go install github.com/zouxiaoliang/pocketbase-mcp-go@latest
```

克隆仓库后本地安装：

```bash
git clone https://github.com/zouxiaoliang/pocketbase-mcp-go.git
cd pocketbase-mcp-go
go install .
```

### 构建

推荐使用 [just](https://github.com/casey/just) 管理常用任务，也可直接使用 Go 工具链：

```bash
just build          # 构建二进制 pocketbase-mcp-go
just --list         # 查看全部任务
```

手动构建：

```bash
go build -o pocketbase-mcp-go .
```

### 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `POCKETBASE_URL` | 是 | PocketBase 地址，如 `http://127.0.0.1:8090` |
| `POCKETBASE_ADMIN_EMAIL` | 否* | 超级用户邮箱 |
| `POCKETBASE_ADMIN_PASSWORD` | 否* | 超级用户密码 |
| `POCKETBASE_ADMIN_MODE` | 否 | 是否以 superuser 执行记录 CRUD，默认 `false`；设为 `true` 时可管理 `listRule` 为空的集合 |
| `MCP_TRANSPORT` | 否 | `http`（默认）或 `stdio` |
| `MCP_HTTP_ADDR` | 否 | HTTP 监听地址，默认 `:8080` |

\* 集合管理（`list_collections`、`create_collection`、`backup_database` 等）始终需要 admin 凭据。记录 CRUD 仅在 `POCKETBASE_ADMIN_MODE=true` 时使用 superuser 身份。

### 命令行参数

命令行参数会覆盖同名环境变量：

| 参数 | 说明 |
|------|------|
| `-transport` | `http` 或 `stdio` |
| `-http-addr` | HTTP 监听地址 |
| `-admin-mode` | `true` 或 `false`，对应 `POCKETBASE_ADMIN_MODE` |

### 启动

**HTTP 模式（默认）**

```bash
export POCKETBASE_URL="http://127.0.0.1:8090"
export POCKETBASE_ADMIN_EMAIL="admin@example.com"
export POCKETBASE_ADMIN_PASSWORD="your_password"
export POCKETBASE_ADMIN_MODE="true"

just run            # 或 just run-http
just health         # 健康检查 GET /health
```

或直接运行二进制：

```bash
./pocketbase-mcp-go
# 等价于: ./pocketbase-mcp-go -transport=http -http-addr=:8080
```

**stdio 模式**

```bash
just run-stdio
```

或：

```bash
./pocketbase-mcp-go -transport=stdio
```

## MCP 客户端配置

### Cursor / Claude Desktop（stdio）

将 `command` 设为二进制绝对路径（可用 `just path` 获取）：

```json
{
  "mcpServers": {
    "pocketbase": {
      "command": "/absolute/path/to/pocketbase-mcp-go",
      "args": ["-transport=stdio"],
      "env": {
        "POCKETBASE_URL": "http://127.0.0.1:8090",
        "POCKETBASE_ADMIN_EMAIL": "admin@example.com",
        "POCKETBASE_ADMIN_PASSWORD": "your_password",
        "POCKETBASE_ADMIN_MODE": "true"
      }
    }
  }
}
```

### Streamable HTTP

将 MCP 端点配置为：

```
http://127.0.0.1:8080/mcp
```

健康检查：

```
http://127.0.0.1:8080/health
```

（具体配置项因客户端而异，参见各客户端 Streamable HTTP 文档。）

## 可用工具（24）

### 集合与记录

| 工具 | 说明 |
|------|------|
| `list_collections` | 列出集合（支持 `filter`、`sort`） |
| `get_collection` | 获取集合 schema |
| `create_collection` | 创建集合（自动附加 `created`/`updated` 字段） |
| `update_collection` | 更新集合（admin） |
| `delete_collection` | 删除集合（admin） |
| `list_records` | 分页列出记录 |
| `create_record` | 创建记录 |
| `update_record` | 更新记录 |
| `delete_record` | 删除记录 |
| `authenticate_user` | 密码登录（支持 `isAdmin`、自定义 `collection`） |
| `create_user` | 在 auth 集合中创建用户 |
| `backup_database` | 创建数据库备份（admin） |

### 认证扩展与导入

| 工具 | 说明 |
|------|------|
| `list_auth_methods` | 列出集合支持的认证方式 |
| `authenticate_with_oauth2` | OAuth2 登录 |
| `authenticate_with_otp` | 请求 OTP（返回 `otpId`） |
| `auth_refresh` | 刷新 auth token |
| `request_verification` | 请求邮箱验证 |
| `confirm_verification` | 确认邮箱验证 |
| `request_password_reset` | 请求密码重置 |
| `confirm_password_reset` | 确认密码重置 |
| `request_email_change` | 请求更换邮箱 |
| `confirm_email_change` | 确认更换邮箱 |
| `impersonate_user` | 管理员模拟用户（admin） |
| `import_data` | 批量导入（`create` / `update` / `upsert`） |

## 使用示例

在 MCP 客户端中调用工具（参数为 JSON）：

```json
// 列出集合
{}

// 创建记录
{
  "collection": "posts",
  "data": { "title": "Hello from MCP" }
}

// 管理员认证（可省略 email/password，使用环境变量）
{
  "isAdmin": true
}

// 批量导入
{
  "collection": "posts",
  "mode": "upsert",
  "data": [
    { "id": "rec1", "title": "First" },
    { "title": "Second" }
  ]
}
```

## 开发与测试

**单元测试与 mock 集成测试**（默认，无需真实 PocketBase）：

```bash
just test
# 或: go test ./...
```

**格式化、静态检查与单元测试**：

```bash
just check
```

**真实 PocketBase 验收**（自动下载 PocketBase、启动临时实例、跑集成测试）：

```bash
just accept
```

或对接已有实例：

```bash
export POCKETBASE_URL="http://127.0.0.1:8090"
export POCKETBASE_ADMIN_EMAIL="admin@example.com"
export POCKETBASE_ADMIN_PASSWORD="your_password"
just test-live
```

**本地 PocketBase 开发环境**（可选）：

```bash
just pb-download   # 下载 PocketBase 到 .local/
just pb-serve      # 启动本地实例，默认 http://127.0.0.1:18090
```

### 常用 just 任务

| 命令 | 说明 |
|------|------|
| `just` | 列出所有任务 |
| `just build` | 构建二进制 |
| `just test` | 单元 / mock 测试 |
| `just check` | fmt + vet + test |
| `just accept` | 完整真实环境验收 |
| `just test-live` | 对已有 PocketBase 跑集成测试 |
| `just pb-download` | 下载 PocketBase 到 `.local/` |
| `just pb-serve` | 启动本地 PocketBase（`:18090`） |
| `just run` / `just run-http` | 启动 MCP 服务（HTTP） |
| `just run-stdio` | 启动 MCP 服务（stdio） |
| `just health` | 探测 HTTP `/health` |
| `just path` | 输出二进制绝对路径（配置 MCP 客户端） |
| `just clean` / `just clean-all` | 清理构建产物 / 含 `.local/` |

## 项目结构

```
pocketbase-mcp-go/
├── justfile                # 构建、测试、运行任务
├── main.go                 # 实现（配置、PB 客户端、24 工具、Gin/stdio）
├── main_test.go            # 单元与 handler 测试
├── live_acceptance_test.go # 真实 PocketBase 验收（integration build tag）
├── go.mod / go.sum
├── scripts/
│   └── run-live-acceptance.sh
└── tests/                  # mock 与传输测试
```

## 故障排查

| 现象 | 处理 |
|------|------|
| `POCKETBASE_URL environment variable is required` | 设置 `POCKETBASE_URL` 后重启 |
| `admin credentials missing` | 设置 `POCKETBASE_ADMIN_EMAIL` / `POCKETBASE_ADMIN_PASSWORD` |
| 备份失败 `Must be in a valid format` | 备份名称需以 `.zip` 结尾（服务会自动补全后缀） |
| 连接被拒绝 | 确认 PocketBase 已启动且 URL 正确 |
| OAuth2 / OTP 失败 | 需在 PocketBase 管理后台配置对应 Provider |
| HTTP 健康检查失败 | 确认服务已启动且 `MCP_HTTP_ADDR` 与探测地址一致 |

## 许可证

[MIT](LICENSE)
