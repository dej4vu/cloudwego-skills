# 故障排查

按现象查原因。

## 生成阶段

| 现象 | 原因 | 处理 |
|---|---|---|
| `can not load ".hz"` | 目录没有 `.hz`，直接跑了 `hz update` | 首次用 `hz new`，之后把 `.hz` 提交 |
| `flag provided but not defined: -proto_path` | 在 cwgo 里用了 hz 的参数名 | cwgo 用 `--proto_search_path` / `-I` |
| `Could not make proto path relative` | IDL 路径相对当前目录写错 | 确认是在 `app/apigw/` 里执行，路径是 `../../idl/...` |
| 生成物出现大量无关 diff | 生成器版本与仓库基线不一致 | 对齐版本；不要用 PATH 里的 `kitex` 生成 cwgo 仓库 |
| `go.mod` 多了一行 thrift `replace` | cwgo 自动追加 | 正常现象，确认是唯一变化即可 |
| blank import 指向不存在的目录 | 原生 hz 用了 `--no_recurse` | 去掉 `--no_recurse`，或手改那一行 |
| 换了 `--template` 但输出没变 | 模板目录路径写错，cwgo 静默回退默认模板 | `--template` 传绝对路径；生成后 `grep` 一个只有你的模板才有的标记 |
| `undefined: context`，出在 `rpc_gen/rpc/{svc}/*_client.go` | 服务只有流式方法，默认模板没导入 `context` | 用 [templates/cwgo/kitex-client](../templates/cwgo/kitex-client) |
| 生成物每次 `gofmt -l` 都脏 | 模板 `range .Imports`，map 顺序随机 | 同上，用修好的模板 |
| `main.go` 引用了不存在的 `kitex-contrib/obs-opentelemetry/logging/logrus` | cwgo 默认 server 模板的占位 | `main.go` 由项目维护，替换成自己的启动代码 |
| struct tag 里仍有 `omitempty`，但期望零值字段保留 | `--unset_omitempty=true` 没经 `--pass` 透传给 hz（cwgo 自身没有这个参数） | 写成 `cwgo server --pass --unset_omitempty=true ...` |
| 新生成的 handler 里没有 service 调用，直接 `c.JSON` | `hz update` 追加的新方法用的是 hz 原生模板 | 按 [add-http-endpoint.md](../workflows/add-http-endpoint.md) 步骤 3 改成委托形状并手写 service |
| 生成后运行期 `proto: file google/api/http.proto is already registered` | cwgo 生成的占位 pb 与其他链路带入的真实 `genproto` 重复注册 | 删掉 `.pb.go` 里指向 `hertz_gen/{github.com,google.golang.org}` 的 blank import，再删这两个占位目录 |
| 网关生成后 `biz/utils/resp.go` 又出现了 | cwgo 路径每次生成都产出这个桩 | 网关已用 `hcommon/pkg/responsex` 时删掉它，否则两套响应函数并存 |
| `check-proto` 报某字段宽度不符 | `int32` / `int64` 选错 | 按 [idl-contracts.md](idl-contracts.md) 的判定表改；有存储依据的登记到 `WHITELIST` |

## 编译阶段

| 现象 | 原因 | 处理 |
|---|---|---|
| `no required module provides package .../biz/router` | 生成后没 `go mod tidy` | 在对应 module 里 tidy |
| `undefined: utils.SendSuccessResponse`（网关） | handler 还引用 `biz/utils`，但该目录已迁到 `hcommon/pkg/responsex` | 把 `utils.` 改成 `responsex.` 并改 import；绑定分支用 `SendBindErrorResponse` |
| `undefined: SendBindErrorResponse` | 仓库还没有 `hcommon`，或 handler 调的是 `utils.` 包 | 确认 `go.work` 里有 `hcommon`，网关 `go.mod` 有对应 `replace` |
| `undefined: context`，出在 `rpc_gen/rpc/{svc}/*_client.go` | 服务只有流式方法 | 用 [templates/cwgo/kitex-client](../templates/cwgo/kitex-client) |
| 改了网关 service 但 `make build` 报 hcommon 的符号找不到 | `hcommon` 是独立 module，改了公开签名没重新 tidy | 在 `hcommon` 与两个网关里都 `go mod tidy`，两个网关都要编译 |
| `xxx is not used` 但那是生成物 | IDL 删了方法，handler 留下孤儿 | 删掉孤儿函数和 import |
| pack 函数缺字段 | 加了 IDL 字段没补 pack | 手写转换不会报错，编译通过也要补 |
| `make check-dao` 失败 | DAO 持有 `db *gorm.DB` 字段，或用了已删除的 `gormx.Interface` / `gormx.G` | 执行器统一为 `g gorm.Interface[*model.X]`，见 [data-and-cache.md](data-and-cache.md) |

## 运行阶段

| 现象 | 原因 | 处理 |
|---|---|---|
| `deps not initialized: call service.InitDeps` | `InitDeps` 没在 `main` 里调用，或在接收请求后才调 | 启动顺序：`dal.Init → rpc.InitClient → service.InitDeps` |
| `InitDeps` panic 重复初始化 | `InitDeps` 被调了两次 | 只在 `main` 调一次 |
| 传了非法 JSON，响应却是"服务繁忙，请稍后再试" | 绑定分支调的是 `SendErrResponse`，绑定错误是无类型错误，被当成非业务错误兜底 | handler 绑定分支改用 `SendBindErrorResponse`；检查 `grep -n 'SendErrResponse' app/apigw/biz/handler/*/*_service.go` 里紧跟 `BindAndValidate` 的那几处 |
| 明明是参数错误，返回的却是"服务繁忙"且 `code=10000` | 同上 | 同上 |
| 响应里 `int64` 变成了字符串 | protojson 规范，`gwjson` 有意为之 | 前端按字符串 / `BigInt` 处理；不要为了"看起来像数字"改回 `c.JSON` |
| 前端传 `"id": "123"`（字符串）报参数错误 | 请求绑定没走 protojson | `server.New` 加 `server.WithBindConfig(responsex.BindConfig())` |
| 前端多传了一个字段就报错 | 同上（protojson 默认 `DiscardUnknown`） | 同上 |
| 响应头里没有 `X-Request-Id` | 没挂 `mw.NewAccessLog()` | `h.Use(mw.NewAccessLog()...)`，且要在 `register` 之前 |
| 日志里访问日志没有请求体 | `logx.FormatJSONPayload` 只对 JSON 内容类型格式化，且超长截断（请求 2048、响应 512） | 正常现象；需要完整 body 时在业务日志里自己打，注意不要打用户正文 |
| 网关日志里出现重复的入口日志 | `mw.NewAccessLog()` 已经打了一行，业务里又手写了一份 | 删掉业务里手写的入口日志 |
| 读到旧数据 | 写后没失效缓存 | 写成功后 `InvalidateCache` |
| 缓存里没有，DB 里也没有，但接口返回"参数不合法" | DAO 用了 `IgnoreNotFound`，把"必须存在"吞成了 `nil, nil` | 必存在查询用 `gormx.WrapNotFound`，service 把 `Err_RecordNotFound` 映射成领域错误 |
| 支付重复入账 | 缺幂等键、行锁或状态机 | 三者都要，见 [async-and-idempotency.md](async-and-idempotency.md) |
| 消费重复处理 | 消费端没幂等 | 去重表或幂等操作 |
| 事件丢失 | 业务代码直接发 Kafka 而非 outbox | 一致性写路径走 outbox |
| queue 启动即退出，日志 `unknown topic` | 配置里的 topic 没在 `handlerFactories` 登记 | 在 `app/queue/biz/handler/handler.go` 加工厂 |
| queue 启动即退出，日志 `mixed environment suffixes` | 同一组消费者配了不同环境后缀的 topic | 统一后缀 |
| 一条坏消息反复重试、堵住分区 | 解析失败没用 `kq.Permanent` 包装 | 必然失败的错误 `return kq.Permanent(...)`，让它直接进 DLQ |
| 日志里出现 `LOST message` | 重试耗尽且未启用 DLQ（或 DLQ 转投失败），消息已丢 | 配置 `DlqTopic`；按告警处理 |
| 请求路径偶发卡 15 秒 | 同步 `Push` 遇到 broker 故障，`MaxAttempts × WriteTimeout` 是上界 | 业务事件仍用同步；埋点类用 `WithAsyncPush` |
| 日志里找不到关联请求 | 没用 `Ctx` 系列方法 | 用 `klog.CtxErrorf` / `hlog.CtxErrorf`；request id 由 `mw.NewAccessLog()` 自动回写响应头 |
| 服务起来了但下游调不通 | client 用了默认 `127.0.0.1:8888` | 在 `infra/rpc/client.go` 用服务发现显式装配 |
| 配置文件找不到，`panic` 读 `conf/<env>/conf.yaml` | `GO_ENV` 没设或设错 | `confx` 默认 `test`，按部署环境设 `GO_ENV` |

## 如何定位

1. 先看是生成、编译还是运行阶段。
2. 运行问题先查启动顺序和依赖装配，再查业务逻辑。
3. 网关问题先看访问日志那一行（request id、状态码、耗时、IP），再拿 request id 去串下游服务的日志。
4. 改了生成相关的东西，先 `git diff --stat` 看有没有预期外的文件变化。
5. 改 DAO 或 proto 后先跑 `make check-dao` / `make check-proto`，再看业务逻辑。
