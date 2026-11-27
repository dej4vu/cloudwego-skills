# Kitex RPC 服务

业务方法怎么写、依赖怎么拿、上下文和错误怎么处理。

## 方法形状 `[主]`

生成桩固定为 `New{Method}Service(ctx)` + `Run`。只填 `Run` 的方法体，不改签名：

```go
type ListOrderService struct{ ctx context.Context }

func NewListOrderService(ctx context.Context) *ListOrderService {
    return &ListOrderService{ctx: ctx}
}

func (s *ListOrderService) Run(req *order.ListOrderReq) (resp *order.ListOrderResp, err error) {
    if req.GetUserId() == 0 {
        return nil, errno.NewBizError(errno.Err_Params, "请求参数不完整")
    }
    items, err := d().orders.ListByUser(s.ctx, req.GetUserId())
    if err != nil {
        return nil, fmt.Errorf("list orders: %w", err)
    }
    return pack.Orders(items), nil
}
```

`handler.go` 由生成器产出，只做一行委托，不要动它。

## 流式方法：第一行必须是 stream.Context() `[主]`

生成的流式 handler 骨架是：

```go
func (s *OrderServiceImpl) WatchStream(req *order.WatchReq, stream order.OrderService_WatchStreamServer) (err error) {
    ctx := context.Background()          // ← 骨架硬编码，不透传请求 ctx
    err = service.NewWatchStreamService(ctx).Run(req, stream)
    return
}
```

所以 `Run` 的第一行必须用 stream 的上下文覆盖它，否则超时、取消、trace 全部与连接脱钩：

```go
func (s *WatchStreamService) Run(req *order.WatchReq, stream order.OrderService_WatchStreamServer) error {
    // 流式 handler 的生成骨架不透传请求 ctx；连接生命周期以 stream.Context() 为准。
    s.ctx = stream.Context()
    ...
}
```

这是骨架的设计缺陷，不是可选项。完整链路见 [streaming-sse.md](streaming-sse.md)。

## 取依赖

通过 `d()` 取，不在方法内构造：

```go
// ✅
resp, err := d().inventoryClient.ReserveStock(s.ctx, req)

// ❌ 每个方法各自初始化，无法在启动时发现配置错误，也无法替换
client := inventory.NewInventoryClient(...)
```

## 下游 client 用窄接口 `[主]`

`init.go` 里为每个下游声明只含本服务用到方法的接口，把生成的 client 赋进去：

```go
type inventoryClient interface {
    ReserveStock(ctx context.Context, req *inventory.ReserveStockReq, callOptions ...callopt.Option) (*inventory.ReserveStockResp, error)
}

type riskClient interface {
    CheckText(ctx context.Context, req *risk.CheckTextReq, callOptions ...callopt.Option) (*risk.CheckTextResp, error)
}
```

接口签名与生成 client 的方法签名一致，`rpc.InventoryClient` 可以直接赋值。好处是 service 只依赖它用到的面，换实现或写测试时不必 mock 整个 client。

## 下游 client 装配 `[主]`

`infra/rpc/client.go` 集中初始化，用统一 suite 接入服务发现、链路追踪、超时：

```go
var (
    InventoryClient inventoryservice.Client
    RiskClient   riskservice.Client
)

func InitClient() {
    suite := client.WithSuite(clientsuite.CommonGrpcClientSuite{
        CurrentServiceName: consts.OrderServiceName,
        RegistryAddr:       strings.Join(conf.GetConf().Registry.RegistryAddress, ","),
    })
    var err error
    if InventoryClient, err = inventoryservice.NewClient(consts.InventoryServiceName, suite); err != nil {
        panic(err)
    }
    if RiskClient, err = riskservice.NewClient(consts.RiskServiceName, suite); err != nil {
        panic(err)
    }
}
```

`main` 里在 `service.InitDeps()` 之前调用。client 创建失败直接 panic，服务不应该带着残缺的下游启动。

## 错误 `[主]`

业务错误用 `errno`，带稳定码和面向用户的文案：

```go
return nil, errno.NewBizError(errno.Err_NotFound, "用户不存在")
return nil, errno.NewBizError(errno.Err_ServiceErr, "内部服务错误")
```

内部错误（DAO、下游 RPC）用 `%w` 包装后向上返回，由最外层决定映射；不要在 service 里把底层错误吞成 `nil` 或换成无信息的字符串：

```go
if err != nil {
    return nil, fmt.Errorf("failed to create user: %w", err)
}
```

错误码定义与网关侧的映射见 [errors-auth-observability.md](errors-auth-observability.md)。

## 上下文与并发 `[主]`

- 请求范围内的工作用 `s.ctx`。后台任务（outbox worker、缓存预热）才用独立的 `context.Background()`，并在 `ShutdownDeps` 里停止。
- 业务代码里起 goroutine 用 `gopool.Go`，不用裸 `go`：gopool 自带 panic recover，裸 `go` 的 panic 会拖垮进程。

```go
gopool.Go(worker.Run)
```

## 启动与关闭顺序 `[主]`

```go
func main() {
    opts := kitexInit()
    dal.Init()
    rpc.InitClient()
    service.InitDeps()
    svr := orderservice.NewServer(new(OrderServiceImpl), opts...)
    server.RegisterShutdownHook(func() { service.ShutdownDeps() })
    if err := svr.Run(); err != nil { klog.Error(err.Error()) }
}
```

`InitDeps` 在接收请求前完成；`ShutdownDeps` 注册为 shutdown hook，先停 worker 再关连接。
