# 新增 RPC 服务

从一个名字到一个能编译、能注册、能被调用的 Kitex 服务。

## 前置判断

- 这个服务的边界是什么？它拥有哪些表？实时链路是否需要内聚在这里？见 [architecture.md](../references/architecture.md)。
- 仓库走 cwgo 还是原生工具？看 Makefile 里有没有 `cwgo`。见 [toolchain-and-generation.md](../references/toolchain-and-generation.md)。

## 步骤

### 1. 定义 RPC IDL

写 `idl/rpc/{svc}/{svc}.proto`：`service {Svc}Service`，方法名单数，字段注释在上一行。

**完成条件**：`protoc` 能解析（或 `make gen-svc` 不报 IDL 错误）。

### 2. 生成客户端与 kitex_gen

```bash
make gen-svc svc={svc}
```

**完成条件**：`rpc_gen/kitex_gen/{svc}/` 与 `rpc_gen/rpc/{svc}/` 出现；`git status` 里只有这两个目录和 `rpc_gen/go.sum` 变化。

`make gen-svc` 用仓库自带的 `templates/cwgo/kitex-server` 生成骨架，`main.go`、`conf/`、`biz/dal/`、`infra/rpc/client.go` 都是 `skip`（首次生成后归项目维护），`handler.go` 与 `biz/service/{method}.go` 是追加。

### 3. 生成服务骨架

`make gen-svc` 通常已包含这一步。否则：

```bash
cd app/{svc} && cwgo server --type RPC --service {svc} --module <mod>/app/{svc} \
  --template ../../templates/cwgo/kitex-server --pass "-use <mod>/rpc_gen/kitex_gen" \
  -I ../../idl/rpc --idl ../../idl/rpc/{svc}/{svc}.proto && go mod tidy
```

**完成条件**：`app/{svc}/` 下有 `main.go`、`handler.go`、`biz/service/{method}.go`。

### 4. 接入工作区

把 `app/{svc}` 加进 `go.work`，`go.mod` 里补 `replace` 指向本地 `rpc_gen` 与 `common`；Makefile 的 `SERVICES` 列表加上它。

**完成条件**：`make build svc={svc}` 通过。

### 5. 集中装配依赖

写 `biz/service/init.go`。看仓库里同类服务用哪种形态，照着写：

- 依赖少、以 DAO 为主 → 包级变量（`var OrderDAO *dao.OrderDAO`），`InitDeps` 里逐个 `New`，首个依赖非空即视为重复初始化、直接 panic。
- 依赖多、要把下游 client 注入 → `dependencies` 结构体 + `d()`，下游 client 用窄接口。

两种都要有 `ShutdownDeps()`（有后台 worker 时停 worker）。

在 `main.go` 里按顺序调用 `dal.Init()`、`rpc.InitClient()`、`service.InitDeps()`（包级变量形态通常是 `InitDeps` 在 `InitClient` 之前；`dependencies` 形态必须 `InitClient` 在前），并把 `ShutdownDeps` 注册为 shutdown hook。`svr.Run()` 失败 `os.Exit(1)`。

**完成条件**：不调 `InitDeps` 直接调业务方法会 panic；调了之后正常。

### 6. 下游 client

写 `infra/rpc/client.go`，用统一 suite（服务发现、超时、trace）初始化下游。不要用 cwgo 默认的 `DefaultClient()`——它硬编码了 `127.0.0.1:8888`。

**完成条件**：服务能通过服务发现调通下游。

### 7. 实现 service 方法

每个方法只写编排，通过 `d()` 取依赖。参数校验用 `errno.NewBizError(errno.Err_Params, ...)`。

**完成条件**：`make test svc={svc}` 通过；`[logic]` 函数有测试。

### 8. 数据层

表归这个服务所有才在这里建 DAO；每张表一个 `{entity}_dao.go`。模型不定义索引。见 [data-and-cache.md](../references/data-and-cache.md)。

### 9. 部署

先部署本服务（callee），再部署调用方（caller）。

## 交付清单

- `idl/rpc/{svc}/{svc}.proto`
- `rpc_gen/` 生成物
- `app/{svc}/` 全套
- `go.work`、Makefile 更新
- migration（如有新表）
