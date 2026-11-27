---
name: cloudwego-skills
description: Build, review, or debug business services on CloudWeGo Hertz (HTTP) and Kitex (RPC) when the repo uses cwgo or hz/kitex code generation, Protobuf/Thrift IDL, generated handler/service stubs, gateway-to-RPC call chains, SSE streaming, transactional outbox, or idempotent billing. Trigger on .proto under idl/, rpc_gen/, hertz_gen/, biz/service, cwgo, hz, kitex, or go.mod importing cloudwego/hertz or cloudwego/kitex.
license: Apache-2.0
metadata:
  version: "1.0.0"
---

# CloudWeGo Skills

规范 Hertz 网关与 Kitex RPC 的业务开发：哪些代码是生成物、业务逻辑写在哪、IDL 怎么改、改完怎么验证。

规范来源的优先级：**目标仓库的 `AGENTS.md` / `CLAUDE.md` / `.claude/rules` / Makefile > 本 skill**。本 skill 的基线来自主参考库与对照库两个生产代码库，标注 `[主]` 的规则是最新实践；目标仓库有不同约定时以仓库为准，并在交付说明里指出差异。

## 先分类，再读参考

动手前先判断任务属于哪一类，只读该类的文档。一个任务跨多类时逐类读取。

| 任务特征 | 读 |
|---|---|
| 新建服务、网关与 RPC 的边界、目录放哪 | [architecture.md](references/architecture.md) |
| 运行 `cwgo` / `hz` / `kitex`、选生成路径、生成后怎么补 | [toolchain-and-generation.md](references/toolchain-and-generation.md) |
| 不确定某文件能否手改；流式 handler 需要桥接 | [generated-code-boundary.md](references/generated-code-boundary.md) |
| 改 `.proto`、加 RPC 方法、加网关注解、字段兼容 | [idl-contracts.md](references/idl-contracts.md) |
| 写 `biz/service/*.go`、依赖装配、RPC 错误与上下文 | [kitex-service.md](references/kitex-service.md) |
| 写网关 service、pack、认证中间件、响应信封（`responsex` / `gwjson`）、protojson 绑定 | [hertz-gateway.md](references/hertz-gateway.md) |
| DAO、GORM 官方泛型、`cachex` 缓存协作、not-found 语义、表归属 | [data-and-cache.md](references/data-and-cache.md) |
| Kafka（`kq` 生产者 / 消费者 / DLQ）、outbox、消费幂等、支付回调幂等、锁与限流 | [async-and-idempotency.md](references/async-and-idempotency.md) |
| Kitex server-streaming 到 Hertz SSE 的全链路 | [streaming-sse.md](references/streaming-sse.md) |
| errno 错误码、网关错误映射的两个入口、日志选型、request id、身份传递 | [errors-auth-observability.md](references/errors-auth-observability.md) |
| 该不该写测试、用什么命令验证 | [testing-and-verification.md](references/testing-and-verification.md) |
| 生成失败、版本混用、响应形状不对、缓存不一致、消费堆积 | [troubleshooting.md](references/troubleshooting.md) |
| 版本矩阵、cwgo 与原生工具的差异 | [compatibility.md](references/compatibility.md) |
| 需要两个真实代码库的对照来理解"为什么这样定" | [case-studies/primary.md](case-studies/primary.md)、[case-studies/legacy.md](case-studies/legacy.md) |

端到端的操作步骤按场景放在 [workflows/](workflows/)：
[新服务](workflows/add-service.md) · [新 RPC 方法](workflows/add-rpc-method.md) · [新 HTTP 端点](workflows/add-http-endpoint.md) · [流式端点](workflows/add-streaming-endpoint.md) · [outbox 事件](workflows/add-outbox-event.md) · [支付回调](workflows/add-payment-callback.md)。

需要直接复用代码时，从 [templates/](templates/) 取，不要凭记忆写：

| 需要 | 取 |
|---|---|
| 修好的 cwgo client 模板（纯流式服务也能编译、gofmt 干净） | [templates/cwgo/kitex-client/](templates/cwgo/kitex-client/) |
| Kitex 服务的依赖装配与 service 桩 | [templates/kitex/](templates/kitex/) |
| 网关响应信封（待同步到 `responsex` 形态）、SSE 桥接 | [templates/hertz/](templates/hertz/) |
| 网关端点的 IDL 注解片段 | [templates/protobuf/](templates/protobuf/) |
| outbox worker 与幂等消费 | [templates/async/](templates/async/) |

[fixtures/](fixtures/) 是四个可运行的最小工程（`kitex-greet`、`hertz-greet`、`stream-sse`、`outbox-idempotency`），每个规则都有对应的位置可以对照；拿不准某段代码长什么样时直接看 fixture。

## 验证本 skill

改完本 skill 的任何文件后跑 `scripts/check-skill.sh`（结构校验 + 过时表述守护 + fixture 编译）。要验证它是否真的让 agent 做对了事，用 [evals/](evals/) 里的用例做 with/without 对照，方法见 [testing-and-verification.md](references/testing-and-verification.md) 的"验证本 skill"一节。

## 每次都要守的规则

这些规则跨越所有任务类型，不重复写进各参考文档。

1. **IDL 先行。** 协议改动先改 `.proto`，再生成，最后写业务。不要先写 Go 再补 IDL。
2. **生成物不手改。** `rpc_gen/kitex_gen/**`、`hertz_gen/**`、生成的 router、普通 `handler.go` 只能通过重新生成来变更。业务逻辑只写在 `biz/service/{method}.go`。唯一例外是流式 handler 的桥接，须按 [generated-code-boundary.md](references/generated-code-boundary.md) 登记标记。
3. **生成后看 diff。** 重新生成后先 `git diff --stat` 再继续：diff 里出现预期外的文件，说明工具版本或参数与仓库基线不一致，先停下查 [compatibility.md](references/compatibility.md)。
4. **优先走 Makefile。** 仓库有 `make gen-*` / `make build` / `make test` 时用它们；裸命令只在 Makefile 不存在或目标缺失时使用，且参数必须与 Makefile 里的保持一致。
5. **网关不碰数据。** `apigw` / `admingw` 只做接入、认证、路由、pack；不直连数据库，不承载领域事实。每张表只有一个 owner service。
6. **请求上下文一路透传。** 业务代码用请求 `ctx`；流式方法用 `stream.Context()`。不要用 `context.Background()` 承载请求生命周期的工作。
7. **错误带稳定码。** RPC 业务错误用 `errno.NewBizError`；网关 service 直接 `return nil, err`，由统一的响应函数决定 HTTP 表现。
8. **验证到能跑。** 每个逻辑单元结束时至少完成 `make build`；纯逻辑补 TDD。改了生成流程就重新生成并确认 diff 干净。
9. **网关错误按分支选入口。** handler 里 `BindAndValidate` 失败调 `SendBindErrorResponse`，`service.Run` 返回错误调 `SendErrResponse`，不按错误文案猜分类。
10. **公共代码放对地方。** import 了 Hertz 的公共代码放 `hcommon/`，`common/` 零 Hertz 依赖；两个网关共用的东西不要在各自 `biz/` 里复制。
11. **DAO 命名即缓存语义。** `GetBy*` 走缓存并标 `// cached`，`FindBy*` 直查；执行器用官方 `gorm.Interface[*model.X]`，不持有 `db *gorm.DB` 字段。

## 不要做的事

- 不要把对照库的做法当成默认标准。对照库保留的是演进前的形态，两者冲突时以 `[主]` 为准。
- 不要为了"看起来更规范"引入目标仓库没有的抽象层、接口层或 ORM。
- 不要在网关 service 里自行构造 HTTP 响应或决定状态码。
- 不要把业务事件在数据库事务外同步发 Kafka；要一致性的写路径走 outbox。
- 不要用 `c.JSON` + `encoding/json` 输出 proto 响应：`int64` 会丢精度、零值被 `omitempty` 吃掉，统一走 `responsex`。
- 不要靠 `strings.Contains(err.Error(), "timeout")` 这类文案匹配判断错误类别。
- 不要在 handler 里把原始消息体或解析错误文本打进日志（可能含用户正文）。

## 版本

本 skill 的规则和 fixture 在 `cwgo v0.1.3`（Kitex 生成器 `v0.16.3`，hz `v0.9.7`）下验证。版本矩阵、已知的路径差异和升级时的注意事项见 [compatibility.md](references/compatibility.md)。目标仓库的版本与之不同时，先读该文档再动手。

## 待同步

- SSE 部分的协议仍在调整，[streaming-sse.md](references/streaming-sse.md)、[add-streaming-endpoint.md](workflows/add-streaming-endpoint.md) 与 `stream-sse` fixture 暂不作为规范。
- `fixtures/hertz-greet` 与 [templates/hertz/resp.go.tmpl](templates/hertz/resp.go.tmpl) 仍是旧的 `biz/utils` 三层匹配实现，与 [hertz-gateway.md](references/hertz-gateway.md) 描述的 `responsex` 两入口形态不一致，待同步。
