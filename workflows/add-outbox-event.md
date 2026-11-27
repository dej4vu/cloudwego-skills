# 新增 outbox 事件

让一个业务事件在数据库事务提交后可靠地发到 Kafka。

## 适用

事件必须与业务数据一致：丢了会导致下游状态错误，或者业务回滚了但事件已发出。不满足这个条件的（埋点、非关键通知）直接同步发即可。

## 步骤

### 1. 定义事件

在 `common/mq` 定义消息结构（`{Event}Msg`）和 topic 基名常量。规则：

- 字段 json tag 用 snake_case；手机号等 PII 不进消息。
- 带一个 `request_id` 字段作为消费端幂等键。
- 代码里只有 topic 基名，环境后缀（`_test` / `_gray`）由各环境配置注入，不要写死在常量里。

在 `app/queue/biz/handler/handler.go` 的 `handlerFactories` 登记 topic → handler 工厂。漏登记时 queue 启动直接报 `unknown topic`。

### 2. outbox 表

通常复用已有的 `outbox` 表。字段：`event_id`、`aggregate_type`、`aggregate_id`、`topic`、`payload`、`status`、`retry_count`、`next_retry_at`、`published_at`。`event_id` 唯一索引。

### 3. 写事务

业务行和 outbox 行在**同一个** `Transaction` 里写：

```go
err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&entity).Error; err != nil {
        return err
    }
    payload, _ := json.Marshal(event)
    return tx.Create(&Outbox{EventID: "evt-" + entity.ID, AggregateID: entity.ID,
        Topic: topic, Payload: string(payload), Status: "PENDING",
        NextRetryAt: time.Now().Add(time.Second)}).Error
})
```

**完成条件**：事务回滚时 outbox 行也不存在。

### 4. store 接口

在 `init.go` 声明窄接口：

```go
type outboxStore interface {
    ListDueOutbox(ctx context.Context, limit int) ([]*Outbox, error)
    MarkOutboxSentByID(ctx context.Context, id int64) error
    ScheduleOutboxRetry(ctx context.Context, id int64, retry int, next time.Time) error
    MarkOutboxFailed(ctx context.Context, id int64, retry int) error
}
```

### 5. worker

参考 [async-and-idempotency.md](../references/async-and-idempotency.md)。用 `gopool.Go` 启动，`ShutdownDeps` 里 `Stop()`。

worker 往 Kafka 投递用 `kq.Pusher`（同步模式，默认 `RequiredAcks=all`），不要直接用 kafka-go。pusher 在窄接口后面注入，方便用 fake 测 worker 的四条路径。

**完成条件**：worker 发布成功后 `status=SENT`；失败按退避重试；超上限 `FAILED`；`Stop()` 能在处理完当前批次后返回。

### 6. 消费端幂等

在 `app/queue/biz/handler/` 加 handler，只做三件事：反序列化 → 调领域 RPC → 返回 error。业务逻辑在领域服务里实现。

```go
func (h *xHandler) Handle(ctx context.Context, msg message.Message) error {
    var bizMsg mq.XMsg
    if err := sonic.Unmarshal(msg.Value(), &bizMsg); err != nil {
        // 解析错误文本可能带原始 JSON 片段，不要原样返回/打印。
        return invalidMessageError("x", msg.Value(), err)
    }
    _, err := rpc.XClient.Do(ctx, &rpcx.DoReq{RequestId: bizMsg.RequestId /* ... */})
    return err
}
```

消费端用 `request_id` 去重，或让操作本身幂等。进程内重试与 rebalance 都会造成重复投递。

失败语义见 [async-and-idempotency.md](../references/async-and-idempotency.md)：必然失败的错误用 `kq.Permanent` 包装；可重试的直接返回 err；重试耗尽进统一 DLQ。

### 7. 测试

用 fake store + fake publisher 测 worker 的四条路径：成功一次、失败重试、超限标记失败、停机退出。

**完成条件**：`make test` 通过。
