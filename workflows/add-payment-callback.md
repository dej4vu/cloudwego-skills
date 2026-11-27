# 新增支付回调

第三方支付的回调入口，必须幂等。

## 步骤

### 1. 路由

回调不是给前端用的，不走 IDL。写在网关根目录 `router.go` 的 `customizedRegister`——这是 hz 预留的手写入口：

```go
func customizedRegister(r *server.Hertz) {
    r.POST("/v1/payments/channel/notify", paymenthandler.HandlePaymentCallback)
}
```

### 2. handler：验签 + 快速应答

第三方会重试直到收到成功应答，所以 handler 要**先**持久化再应答，不要在 handler 里同步做重业务：

```go
func HandlePaymentCallback(ctx context.Context, c *app.RequestContext) {
    notify, err := verifyAndParse(c)
    if err != nil {
        c.String(consts.StatusBadRequest, "fail")
        return
    }
    resp, err := rpc.PaymentClient.ApplyPaymentCallback(ctx, pack.ToRPCCallback(notify))
    if err != nil {
        hlog.CtxErrorf(ctx, "apply payment callback failed: %v", err)
        c.String(consts.StatusInternalServerError, "fail")
        return
    }
    c.String(consts.StatusOK, resp.GetResponse())
}
```

验签失败返回 400；业务处理失败返回 5xx 让第三方重试。

### 3. 领域 RPC：幂等入账

事务内三件事：锁订单行、查幂等键、状态机推进。见 [async-and-idempotency.md](../references/async-and-idempotency.md)。

```go
key := fmt.Sprintf("ORDER:%s:AMOUNT:AVAILABLE", order.PaymentNo)
```

**完成条件**：同一笔回调重复 N 次，账本只有一条记录；乱序到达（先"成功"后"处理中"）不会把状态改回去。

### 4. 幂等键的构成

`{业务}:{单号}:{资产}:{桶}`。同一笔业务的不同入账方向（可用、冻结）用不同的桶，这样退款和入账可以各自幂等。

### 5. 并发

行锁 + 状态机已经保证正确性；分布式锁是额外的保护，不是替代。

### 6. 验证

- 用测试环境重放同一笔回调，确认余额只加一次。
- 模拟乱序回调。
- 回调处理失败时，确认第三方会重试且第二次能成功。
