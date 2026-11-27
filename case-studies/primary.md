# 案例：主参考库

> 基于 2026-11 的代码快照归纳。只描述架构与模式，不含内部地址、凭据和业务数据；代码片段为最小化示意，不是可直接复制的私有实现。

## 是什么

一个带实时流式输出与在线支付的 C 端产品的服务端。Go 单仓多服务，Kitex RPC + Hertz HTTP，Protobuf IDL，MySQL / Redis / Kafka / Etcd。

它是本 skill 的**主规范来源**：相对对照库，它在依赖装配、流式、异步可靠性、网关 JSON 契约、数据访问约定五个方面做了演进。

## 服务划分

| 服务 | 类型 | 职责 |
|---|---|---|
| `apigw` | Hertz | 用户端入口：认证、路由、pack、调 RPC |
| `admingw` | Hertz | 管理端入口 |
| `user` | Kitex | 身份与准入 |
| `sms` | Kitex | 短信 |
| `file` | Kitex | 文件与上传 |
| `admin` | Kitex | 管理员 |
| `order` | Kitex | 订单：状态机、流式进度推送 |
| `inventory` | Kitex | 库存：预留、扣减、对账 |
| `product` | Kitex | 商品 |
| `payment` | Kitex | 支付：渠道、回调、幂等入账 |
| `risk` | Kitex | 内容风控 |
| `queue` | Consumer | 集中消费所有 topic |

拆分判据：实时链路内聚（状态和编排不外拆），非实时的走异步（对账、结算、审核），长耗时的落专用 worker。

仓库没有独立的定时任务服务。规范文档里提到的若干服务是规划中的，磁盘上还不存在，动手前先 `ls app/` 确认。

## 演进 1：集中依赖装配

**问题**：对照库在每个方法文件里各自构造 DAO 和 client。服务变大后，加一个下游要改多个文件；配置错误要到第一次请求才暴露。

**做法**：`biz/service/init.go` 里一个 `dependencies` 结构体装下所有依赖，`d()` 取用，`InitDeps()` 在 `main` 里服务启动前调一次。下游 client 用窄接口。

```go
type dependencies struct {
    inventoryClient  inventoryClient   // 窄接口
    orderStore     orderStore
    orderEventPublisher orderEventPublisher
}

func d() *dependencies {
    if deps == nil {
        panic("order service deps not initialized: call service.InitDeps before serving requests")
    }
    return deps
}
```

**收益**：启动即 fail-fast；方法文件只剩编排；窄接口让测试不必 mock 整个 client。

## 演进 2：SSE 流式返回

**问题**：处理结果是逐步产生的，unary 响应要么等完（延迟高）要么得分片轮询。

**做法**：RPC 侧 server-streaming，网关侧用登记例外的 handler 桥接把流转成 SSE。

三个关键决定：

1. `Run` 第一行 `s.ctx = stream.Context()`，因为生成的骨架硬编码了 `context.Background()`。
2. 业务错误编码为 `error` 事件而不是 RPC error，避免客户端只看到断流。
3. 增量事件用显式 DTO 序列化，避免零值字段被丢掉。

事件按 `seq` 单调递增，以 `done` 或 `error` 终止。

历史上曾把 SSE 路由手写在 `router.go` 的 `customizedRegister` 里，后来重构为交给生成器。这印证了 hz 的增量合并语义：只要方法名在 IDL 里，生成的 handler 就是稳定的落点。

> 这部分协议仍在调整，本 skill 暂不以其为准。

## 演进 3：事务性 outbox

**问题**："写订单状态"和"发下游处理事件"要一致，但 DB 和 Kafka 不是一个事务。

**做法**：业务行和 outbox 行在同一个本地事务里写，独立 worker 轮询投递。

```go
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(order).Error; err != nil { return err }
    return tx.Create(&outbox{EventID: "order-paid-" + order.RequestID, Status: "PENDING",
        NextRetryAt: time.Now().Add(time.Second)}).Error
})
```

worker：1s 轮询、100 条批、最多 10 次、30s × 次数退避。停机时在途消息不进 DLQ、不提交。

outbox 是已落库业务事件的唯一发布路径。

## 演进 4：支付幂等

**问题**：第三方支付回调至少一次投递，且可能乱序。

**做法**：事务内锁订单行 → 查幂等键 → 状态机只允许前进 → 锁余额账户写账本。幂等键 `{业务}:{单号}:{资产}:{桶}`。

三要素缺一不可：只有幂等键没有行锁，并发回调同时通过检查；只有行锁没有状态机，乱序回调可能改回状态。

## 演进 5：网关 JSON 契约

**问题**：网关响应用 `c.JSON` + `encoding/json`，proto 的 `int64` 在 JS 里丢精度，零值字段被 `omitempty` 吃掉，两个网关的错误响应形状靠人肉对齐。对照库为了区分"参数错误"和"系统错误"，用错误文案里的关键词（`timeout`、`connection`）做字符串匹配。

**做法**：三层各管一件事。

1. **`common/gwjson`**：纯编码器。信封 wire 契约固定为 `{code, msg, data, timestamp}`，`data` 走 protojson（`int64` → 字符串，零值保留，`EmitUnpopulated`），`timestamp` 也输出字符串；`Encoder` 走 `sync.Pool`。
2. **`hcommon/pkg/responsex`**：三个入口按**分支**分类而不是按文案猜——`SendBindErrorResponse` 只给 `BindAndValidate` 失败，`SendErrResponse` 只给业务分支。业务错误原样透传，其余一律"服务繁忙"。
3. **`responsex.BindConfig`**：请求绑定也走 protojson，`int64` 同时接受字符串与数字，未知字段忽略。

```go
if err := c.BindAndValidate(&req); err != nil {
    responsex.SendBindErrorResponse(ctx, c, err)
    return
}
resp, err := service.NewXService(ctx, c).Run(&req)
if err != nil {
    responsex.SendErrResponse(ctx, c, consts.StatusBadRequest, err)
    return
}
responsex.SendSuccessResponse(ctx, c, consts.StatusOK, resp)
```

**收益**：前端拿到稳定的 JSON 形态；绑定错误不再被报成"服务繁忙"；不再有靠错误文案猜分类的脆弱逻辑。

## 演进 6：Kafka 封装 `kq`

**问题**：直接用 kafka-go 时，生产端默认重试 10 次 × 10s 超时能把请求路径卡住 100s；消费端失败处理、DLQ、停机语义每个消费者各写一遍。

**做法**：`common/queue/kq` 统一封装。生产端默认同步、`RequiredAcks=all`、重试 3 次 × 5s（最坏 15s）、hash 分区；消费端一个 Reader 订阅全部 topic 按 `msg.Topic` 分发，进程内指数退避重试，耗尽后进统一 DLQ，`kq.Permanent` 标记必然失败的错误不重试，停机时在途消息不进 DLQ 不提交。

两个仓库都用了 `kq`，主参考库把它写成了规范并补齐 DLQ 重放工具。

## 其他约定

- 网关不碰数据，只做接入、认证、路由、pack。
- 每张表一个 owner；整个项目共用一个库，但不跨服务读写表。
- DAO 用 GORM 官方泛型 `gorm.Interface[*model.X]`，缓存以 `cacheRepo` / `countCache` 协作者内嵌，没有独立 `repo` 层；方法名前缀表达缓存语义（`GetBy` 走缓存、`FindBy` 直查）。
- 网关公共代码在 `hcommon/`，`common/` 零 Hertz 依赖。
- 业务代码用 `gopool.Go` 而不是裸 `go`。
- 测试按 `[logic]` / `[glue]` / `[mixed]` 分级，不追求覆盖率。
- 所有网关端点 POST，IDL 带三个注解；整型字段宽度按判定表选，`make check-proto` 守护。

## 可借鉴的判断

| 场景 | 主参考库的选择 | 何时照搬 |
|---|---|---|
| 服务数量 > 3 | 集中装配 | 一旦有多个下游就值得 |
| 结果逐步产生 | server-streaming + SSE | 任何流式输出（协议待定） |
| DB 写 + 事件发 | outbox | 事件丢了会出错时 |
| 第三方回调 | 行锁 + 幂等键 + 状态机 | 任何至少一次投递的入口 |
| proto 响应直接出 HTTP | protojson 编解码 + 按分支分类的错误入口 | 任何用 protojson 的网关 |
| 热点读 | DAO 内嵌 `cacheRepo` | 读多写少的实体 |
