# 新增流式端点

一个 SSE 端点从 RPC 到网关的全链路。

## 步骤

### 1. RPC IDL：server-streaming

```protobuf
rpc ChatStream(ChatReq) returns (stream ChatStreamResp) {}
```

响应消息带 `type`、`seq`、`replyId`：

```protobuf
message ChatStreamResp {
  // 事件类型：transcript / delta / audio / done / error
  string type = 1;
  // 本轮回复 ID
  string replyId = 2;
  // 分片序号，从 1 开始
  int64 seq = 3;
  ...
}
```

### 2. 网关 IDL：普通 unary

```protobuf
rpc ChatStream(ChatReq) returns (ChatStreamResp) {
  option (api.post) = "v1/dialogue/chat-stream";
  option (google.api.http) = { post: "/v1/dialogue/chat-stream" body: "*" };
  option (openapi.v3.operation) = { summary: "多模态对话流式返回" };
}
```

### 3. 生成

`make gen-svc` + `make gen-gw`。

### 4. 实现 RPC 侧

`Run` 第一行必须是 `s.ctx = stream.Context()`。业务错误用 `sendErrorEvent` 编码为事件后 `return nil`。先查幂等重放。见 [streaming-sse.md](../references/streaming-sse.md)。

**完成条件**：客户端中途断开时，模型调用能被取消。

### 5. 实现网关 service

`Run` 返回 stream 对象，设置独立超时：

```go
return rpc.DialogueClient.ChatStream(s.Context, req, callopt.WithRPCTimeout(5*time.Minute))
```

### 6. 写桥接

在 `biz/handler/{pkg}/sse.go` 写 `writeSSEStream`，函数上方加标记：

```go
// cloudwego-skills: sse-bridge ChatStream — registered exception, see references/generated-code-boundary.md
func writeSSEStream(c *app.RequestContext, stream dialogueservice.DialogueService_ChatStreamClient) {
```

检查零值有意义的字段（如 `partIndex`），用显式 DTO 包装。

### 7. 改 handler（登记例外）

只改一处：把 JSON 收尾换成 `writeSSEStream(c, stream)`。

在 Makefile 注释或 docs 里登记：方法名、文件、恢复步骤。

### 8. 挂中间件

SSE 通常无法设置 `Authorization` 头，用业务字段传 token 时，在 IDL 注释里说明原因，并在中间件里兼容两种来源。

### 9. 验证

```bash
curl -N -X POST http://localhost:8888/v1/dialogue/chat-stream \
  -H 'Content-Type: application/json' -d '{...}'
```

**完成条件**：
- 响应头 `Content-Type: text/event-stream`
- 事件顺序正确，`seq` 单调递增
- 正常结束有 `done`
- 制造一个业务错误，收到 `error` 事件而不是连接直接断开

## 恢复桥接

`hz new --force` 或删掉 handler 重建后，桥接会丢。恢复步骤：

1. 确认 `sse.go` 还在（它不是生成物，通常不受影响）。
2. 在生成的 handler 里找到流式方法，把 `resp, err := service...` 之后的 JSON 收尾替换成 `writeSSEStream(c, stream)`。
3. `grep -n 'cloudwego-skills: sse-bridge' biz/handler/{pkg}/sse.go` 确认标记在。
4. `make build svc=apigw`。
