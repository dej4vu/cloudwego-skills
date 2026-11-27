# 新增 RPC 方法

给已有 Kitex 服务加一个方法。

## 步骤

### 1. 改 IDL

在 `idl/rpc/{svc}/{svc}.proto` 的 `service` 里加方法。请求和响应各一个消息，命名 `{Method}Req` / `{Method}Resp`。

```protobuf
// 查询订单详情
rpc GetOrderDetail(GetOrderDetailReq) returns (GetOrderDetailResp) {}
```

**完成条件**：方法名单数；所有字段有上一行注释。

### 2. 重新生成

```bash
make gen-svc svc={svc}
```

**完成条件**：`app/{svc}/biz/service/{method}.go` 和 `{method}_test.go` 桩出现；`rpc_gen/kitex_gen/{svc}/` 更新；`git diff --stat` 里没有预期外文件。

### 3. 实现方法

填 `Run` 的方法体。通过 `d()` 取依赖，不在方法里初始化客户端。

```go
func (s *GetOrderDetailService) Run(req *order.GetOrderDetailReq) (*order.GetOrderDetailResp, error) {
    if req.GetUserId() == 0 {
        return nil, errno.NewBizError(errno.Err_Params, "请求参数不完整")
    }
    setting, err := d().orderSettings.Get(s.ctx, req.GetUserId(), req.GetProductId())
    if err != nil {
        return nil, fmt.Errorf("get order detail: %w", err)
    }
    return pack.OrderDetail(setting), nil
}
```

**完成条件**：`make build svc={svc}` 通过。

### 4. 需要新依赖时

新下游或新 DAO 加进 `init.go` 的 `dependencies` 和 `InitDeps`，不要在方法里临时构造。

### 5. 测试

给函数打标签：纯逻辑抽出来标 `[logic]` 写 TDD；编排按 `[glue]` 只保证编译。

**完成条件**：`make test svc={svc}` 通过。

### 6. 调用方

调用方的 `rpc_gen` 需要同步更新（同一个 `rpc_gen` 模块的话已经更新）。部署顺序 callee 先。

## 流式方法

方法返回 `stream X` 时，额外做：`Run` 第一行 `s.ctx = stream.Context()`，业务错误编码为事件。见 [streaming-sse.md](../references/streaming-sse.md)。
