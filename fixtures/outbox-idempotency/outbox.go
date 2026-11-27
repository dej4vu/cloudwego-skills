// Package outbox 是事务性 outbox 的最小参考实现：业务数据与待发事件在同一事务落库，
// 由独立 worker 轮询投递。它用窄接口隔离存储与发布，便于用 fake 验证 worker 的每条路径。
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status 是 outbox 行的状态。
type Status string

const (
	StatusPending Status = "PENDING"
	StatusSent    Status = "SENT"
	StatusFailed  Status = "FAILED"
)

// Item 是一行待发事件。
type Item struct {
	ID         int64
	EventID    string
	Aggregate  string
	Topic      string
	Payload    string
	Status     Status
	RetryCount int
	NextRetry  time.Time
}

// Config 是 worker 参数。零值字段取默认。
type Config struct {
	Interval    time.Duration // 轮询间隔，默认 1s
	BatchSize   int           // 每批条数，默认 100
	InitialWait time.Duration // 首次投递前的等待，默认 1s
	MaxRetry    int           // 最大重试次数，默认 10
	RetryStep   time.Duration // 退避步长，默认 30s；第 n 次重试等 n*Step
	Now         func() time.Time
}

func (c Config) withDefaults() Config {
	if c.Interval <= 0 {
		c.Interval = time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.InitialWait <= 0 {
		c.InitialWait = time.Second
	}
	if c.MaxRetry <= 0 {
		c.MaxRetry = 10
	}
	if c.RetryStep <= 0 {
		c.RetryStep = 30 * time.Second
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Store 是 worker 对存储的窄接口：只声明 worker 用到的四个操作。
type Store interface {
	ListDue(ctx context.Context, now time.Time, limit int) ([]*Item, error)
	MarkSent(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, retry int) error
	ScheduleRetry(ctx context.Context, id int64, retry int, next time.Time) error
}

// Publisher 是 worker 对消息系统的窄接口。
type Publisher interface {
	Publish(ctx context.Context, item *Item) error
	Close() error
}

// Worker 轮询 due 事件并投递。
type Worker struct {
	store    Store
	pub      Publisher
	cfg      Config
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewWorker 构造 worker；调用 Start 才开始运行。
func NewWorker(store Store, pub Publisher, cfg Config) *Worker {
	return &Worker{store: store, pub: pub, cfg: cfg.withDefaults(),
		stop: make(chan struct{}), done: make(chan struct{})}
}

// Start 启动轮询；应在独立的 goroutine 中调用（生产里用 gopool.Go）。
func (w *Worker) Start() {
	defer close(w.done)
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	for {
		_ = w.ProcessDue(context.Background())
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

// Stop 通知退出并等待当前批次处理完。可重复调用。
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// ProcessDue 处理一批 due 事件，返回处理条数。
func (w *Worker) ProcessDue(ctx context.Context) int {
	items, err := w.store.ListDue(ctx, w.cfg.Now(), w.cfg.BatchSize)
	if err != nil {
		return 0
	}
	for _, it := range items {
		w.publishOne(ctx, it)
	}
	return len(items)
}

func (w *Worker) publishOne(ctx context.Context, it *Item) {
	if it == nil {
		return
	}
	if err := w.pub.Publish(ctx, it); err != nil {
		w.fail(ctx, it, err)
		return
	}
	_ = w.store.MarkSent(ctx, it.ID)
}

func (w *Worker) fail(ctx context.Context, it *Item, cause error) {
	retry := it.RetryCount + 1
	if retry >= w.cfg.MaxRetry {
		_ = w.store.MarkFailed(ctx, it.ID, retry)
		return
	}
	_ = w.store.ScheduleRetry(ctx, it.ID, retry, w.nextRetry(retry))
}

func (w *Worker) nextRetry(retry int) time.Time {
	if retry < 1 {
		retry = 1
	}
	return w.cfg.Now().Add(time.Duration(retry) * w.cfg.RetryStep)
}

// ErrConflict 表示同一幂等键已存在但内容不同。
var ErrConflict = errors.New("idempotency conflict")

// LedgerEntry 是一条账本记录。IdempotencyKey 唯一。
type LedgerEntry struct {
	IdempotencyKey string
	BizNo          string
	Amount         int64
}

// ApplyOnce 用幂等键保证同一笔业务只入账一次。
// 已存在且金额一致 → 视为重复，返回 (false, nil)；金额不一致 → ErrConflict。
func ApplyOnce(ledger map[string]LedgerEntry, e LedgerEntry) (bool, error) {
	if e.IdempotencyKey == "" {
		return false, errors.New("idempotency key is required")
	}
	if prev, ok := ledger[e.IdempotencyKey]; ok {
		if prev.Amount != e.Amount {
			return false, fmt.Errorf("%w: key=%s have=%d want=%d",
				ErrConflict, e.IdempotencyKey, prev.Amount, e.Amount)
		}
		return false, nil
	}
	ledger[e.IdempotencyKey] = e
	return true, nil
}

// OrderStatus 是订单状态机的状态。
type OrderStatus string

const (
	OrderPending OrderStatus = "PENDING"
	OrderPaid    OrderStatus = "PAID"
	OrderClosed  OrderStatus = "CLOSED"
)

// transitions 定义允许的状态推进；只允许前进。
var transitions = map[OrderStatus]map[OrderStatus]bool{
	OrderPending: {OrderPaid: true, OrderClosed: true},
	OrderPaid:    {},
	OrderClosed:  {},
}

// CanTransition 报告 from → to 是否合法。
func CanTransition(from, to OrderStatus) bool {
	return transitions[from][to]
}

// ApplyPaymentCallback 处理一次支付回调：状态机 + 幂等键。
// 返回 applied=true 表示本次调用真正推进了状态。
func ApplyPaymentCallback(order *OrderStatus, ledger map[string]LedgerEntry, key string, amount int64) (applied bool, err error) {
	if !CanTransition(*order, OrderPaid) {
		// 已支付或已关闭：重复/乱序回调，幂等返回
		return false, nil
	}
	applied, err = ApplyOnce(ledger, LedgerEntry{IdempotencyKey: key, Amount: amount})
	if err != nil {
		return false, err
	}
	if applied {
		*order = OrderPaid
	}
	return applied, nil
}

// Encode 把事件序列化为 outbox payload。
func Encode(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
