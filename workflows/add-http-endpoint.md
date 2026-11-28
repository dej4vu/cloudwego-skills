# 新增 HTTP 端点

给网关加一个对外接口。主参考库形态（`hz update` 维护 cwgo 形状的 handler）为例，其他形态的差异见步骤 3。

## 步骤

### 1. 改网关 IDL

在 `idl/apigw/{pkg}/{pkg}.proto`（管理端是 `idl/admingw/`）的 `service` 里加方法，**三个注解都要**：

```protobuf
// 查询订单详情
rpc GetOrderDetail(GetOrderDetailReq) returns (GetOrderDetailResp) {
  option (api.post) = "v1/order/detail/get";
  option (google.api.http) = { post: "/v1/order/detail/get" body: "*" };
  option (openapi.v3.operation) = { summary: "查询订单详情" };
}
```

网关消息不复用 RPC 消息。请求体不声明 `accessToken`。整型字段宽度按 [idl-contracts.md](../references/idl-contracts.md) 的判定表选。

**完成条件**：三个注解齐全；URL 用 POST；`make check-proto` 通过。

### 2. 领域 RPC 先就位

网关端点通常要调一个新的 RPC 方法。先按 [add-rpc-method.md](add-rpc-method.md) 把 RPC 做完并部署，再做网关。部署顺序 callee 先于 caller。

### 3. 重新生成

```bash
make gen-gw          # 或 make gen-admingw
```

`hz update` 追加的 handler 是 hz 原生形状——`c.String(StatusBadRequest, err.Error())` 加 `c.JSON(StatusOK, resp)`，**不委托 service、不带信封、不生成 service 桩**。生成后要做两件事：

**a. 把新方法的 handler 改成三分支委托形状**（只改这一个方法，同文件其他方法不动）：

```go
func GetOrderDetail(ctx context.Context, c *app.RequestContext) {
    var req order.GetOrderDetailReq
    if err := c.BindAndValidate(&req); err != nil {
        responsex.SendBindErrorResponse(ctx, c, err)
        return
    }
    resp, err := service.NewGetOrderDetailService(ctx, c).Run(&req)
    if err != nil {
        responsex.SendErrResponse(ctx, c, consts.StatusBadRequest, err)
        return
    }
    responsex.SendSuccessResponse(ctx, c, consts.StatusOK, resp)
}
```

确认 import 里有 `service` 与 `responsex`。这是允许手改生成物的例外之一，之后 `hz update` 不会再碰它，见 [generated-code-boundary.md](../references/generated-code-boundary.md)。

**b. 确认没有漏改**（两条都应无输出，流式桥接的方法除外）：

```bash
grep -n 'c.String(consts.StatusBadRequest' app/apigw/biz/handler/*/*_service.go
grep -n 'c.JSON(consts.StatusOK' app/apigw/biz/handler/*/*_service.go
```

**c. 确认绑定分支**：`hz update` 追加的新方法把绑定错误也发给 `SendErrResponse`，仓库 Makefile 里的 perl 正则只匹配旧的 `utils.` 前缀，对 `responsex.` 不生效。新方法的手改要同时完成"委托形状"和"`BindAndValidate` 失败调 `SendBindErrorResponse`"两件事（上面的代码块已是最终形状）。

> 形态差异：cwgo 路径（对照库）生成的 handler 本来就委托 service，并自带空桩 `biz/service/{method}.go`，但两个失败分支都是 `StatusOK`、都调 `SendErrResponse`，同样要把绑定分支改成 `SendBindErrorResponse`；区分两个分支看 `Run` 是否已调用，不要按状态码找。原生 `hz new` 的仓库（形态 C）没有 service 层，整个 handler 按上面的形状手写。

**完成条件**：`biz/handler/{pkg}/{pkg}_service.go` 多了方法且已是委托形状；`biz/router/{pkg}/{pkg}.go` 多了路由；`biz/router/{pkg}/middleware.go` 多了 `_{method}Mw()` 桩；`hertz_gen/gw/{pkg}/` 更新；已有方法未被改动。

### 4. 手写网关 service

新建 `biz/service/{method}.go`（生成器不生成这个文件）：

```go
type GetOrderDetailService struct {
    RequestContext *app.RequestContext
    Context        context.Context
}

func NewGetOrderDetailService(ctx context.Context, c *app.RequestContext) *GetOrderDetailService {
    return &GetOrderDetailService{RequestContext: c, Context: ctx}
}

func (s *GetOrderDetailService) Run(req *gworder.GetOrderDetailReq) (*gworder.GetOrderDetailResp, error) {
    if req == nil {
        return nil, errno.NewBizError(errno.Err_Params, "请求参数不完整")
    }
    userId := mw.UserID(s.RequestContext)
    if userId <= 0 {
        return nil, errno.NewBizError(errno.Err_UserTokenInvalid, "登录状态无效")
    }
    resp, err := rpc.OrderClient.GetOrderDetail(s.Context, &rpcorder.GetOrderDetailReq{
        UserId: userId, ProductId: req.GetProductId(),
    })
    if err != nil {
        hlog.CtxErrorf(s.Context, "get order detail rpc failed: user_id=%d err=%v", userId, err)
        return nil, err
    }
    return pack.GetOrderDetailResp(resp), nil
}
```

service 只返回 error，不构造响应。身份从 `mw.UserID` 取，不在 `Run` 里解析 token。需要 request id 用 `hmw.RequestID(s.RequestContext)`。

`req == nil` 或必填字段为空时返回 `errno.NewBizError(errno.Err_Params, "请求参数不完整")`；`userId <= 0` 返回 `Err_UserTokenInvalid`。RPC 失败时 `hlog.CtxErrorf` 带上 request id 与关键参数，再把 `err` 原样返回，由 handler 的 `SendErrResponse` 决定响应。

把调用方环境传给下游时用 `support.go` 的 `clientInfo(c)`（IP、`deviceId`、`platform`、`appVersion`、`deviceModel`、`registerSource` 头）。

**完成条件**：`make build svc=apigw` 通过。

### 5. pack

在 `biz/pack/` 补双向转换函数。手写转换不会因新增字段报错，加字段时主动补。

### 6. 挂中间件

需要登录的端点，改 `biz/router/{pkg}/middleware.go` 里对应的 `_{method}Mw()`：

```go
func _getorderdetailMw() []app.HandlerFunc {
    return mw.MustLoginMw()
}
```

这个文件是生成器的用户扩展点，重新生成不覆盖。

### 7. 验证

起服务后用 curl 打一遍五种情况，HTTP 状态都是 200，`code` 不同：

| 情况 | 怎么触发 | 期望 |
|---|---|---|
| 成功 | 正常请求 | `code=0`，`data` 按 protojson（`int64` 是字符串） |
| 业务错误 | 让 RPC 返回 `errno.NewBizError` | 原码原文案 |
| 未登录 | 不带 `Authorization` | `Err_UserTokenInvalid` + "登录状态无效" |
| 参数错误 | 发非法 JSON 或类型不符的字段 | `Err_Params` + "请求参数不合法" |
| 下游故障 | 停掉 RPC 服务 | `Err_Params` + "服务繁忙，请稍后再试"，日志里有原始错误 |

响应头里应有 `X-Request-Id`。访问日志那一行应有 `ipLocation=`，且**不含** `xff=`。

**完成条件**：五种情况响应形状一致。
