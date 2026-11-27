package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---- fakes：只实现窄接口，不 mock 整个 DAO ----

type fakeStore struct {
	mu     sync.Mutex
	items  map[int64]*Item
	nextID int64
	failAt map[int64]error // 发布失败次数达到后成功
	fails  map[int64]int
}

func newFakeStore() *fakeStore {
	return &fakeStore{items: map[int64]*Item{}, nextID: 1, failAt: map[int64]error{}, fails: map[int64]int{}}
}

func (s *fakeStore) add(it *Item) *Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	it.ID = s.nextID
	s.nextID++
	s.items[it.ID] = it
	return it
}

func (s *fakeStore) get(id int64) *Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[id]
}

func (s *fakeStore) ListDue(_ context.Context, now time.Time, limit int) ([]*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Item
	for _, it := range s.items {
		if it.Status == StatusPending && !it.NextRetry.After(now) {
			out = append(out, it)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (s *fakeStore) MarkSent(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id].Status = StatusSent
	return nil
}

func (s *fakeStore) MarkFailed(_ context.Context, id int64, retry int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id].Status = StatusFailed
	s.items[id].RetryCount = retry
	return nil
}

func (s *fakeStore) ScheduleRetry(_ context.Context, id int64, retry int, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id].RetryCount = retry
	s.items[id].NextRetry = next
	return nil
}

// fakePublisher 可按 item 配置"前 n 次失败"。
type fakePublisher struct {
	mu        sync.Mutex
	published []string
	failN     map[int64]int
	closed    bool
}

func (p *fakePublisher) Publish(_ context.Context, it *Item) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failN[it.ID] > 0 {
		p.failN[it.ID]--
		return errors.New("broker unavailable")
	}
	p.published = append(p.published, it.EventID)
	return nil
}

func (p *fakePublisher) Close() error { p.closed = true; return nil }

func (p *fakePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

// ---- worker ----

func dueItem(eventID string) *Item {
	return &Item{EventID: eventID, Aggregate: "turn", Topic: "t", Payload: "{}", Status: StatusPending}
}

func TestWorkerPublishesOnceAndMarksSent(t *testing.T) {
	store := newFakeStore()
	it := store.add(dueItem("evt-1"))
	pub := &fakePublisher{failN: map[int64]int{}}
	w := NewWorker(store, pub, Config{})

	if n := w.ProcessDue(t.Context()); n != 1 {
		t.Fatalf("processed = %d, want 1", n)
	}
	if pub.count() != 1 {
		t.Fatalf("published = %d, want 1", pub.count())
	}
	if got := store.get(it.ID).Status; got != StatusSent {
		t.Fatalf("status = %s, want SENT", got)
	}
	// 再跑一轮：已 SENT，不应重复投递
	w.ProcessDue(t.Context())
	if pub.count() != 1 {
		t.Fatalf("published = %d after second pass, want still 1", pub.count())
	}
}

func TestWorkerRetriesWithBackoff(t *testing.T) {
	store := newFakeStore()
	it := store.add(dueItem("evt-2"))
	pub := &fakePublisher{failN: map[int64]int{it.ID: 1}}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := base
	w := NewWorker(store, pub, Config{Now: func() time.Time { return clock }})

	w.ProcessDue(t.Context())
	if pub.count() != 0 {
		t.Fatalf("published = %d, want 0 on failure", pub.count())
	}
	got := store.get(it.ID)
	if got.Status != StatusPending || got.RetryCount != 1 {
		t.Fatalf("after 1st fail: status=%s retry=%d, want PENDING/1", got.Status, got.RetryCount)
	}
	if want := base.Add(30 * time.Second); !got.NextRetry.Equal(want) {
		t.Fatalf("next retry = %v, want %v (1 × step)", got.NextRetry, want)
	}

	// 时间推进到退避到期之后，且本次仍失败：退避 2 × step
	clock = base.Add(31 * time.Second)
	// 第二次失败：退避 2 × step
	pub.failN[it.ID] = 1
	w.ProcessDue(t.Context())
	got = store.get(it.ID)
	if got.RetryCount != 2 {
		t.Fatalf("retry = %d, want 2", got.RetryCount)
	}
	if want := clock.Add(60 * time.Second); !got.NextRetry.Equal(want) {
		t.Fatalf("next retry = %v, want %v (2 × step)", got.NextRetry, want)
	}
}

func TestWorkerMarksFailedAfterMaxRetry(t *testing.T) {
	store := newFakeStore()
	it := store.add(dueItem("evt-3"))
	pub := &fakePublisher{failN: map[int64]int{}}
	w := NewWorker(store, pub, Config{MaxRetry: 3, RetryStep: time.Millisecond})
	pub.failN[it.ID] = 100 // 永远失败

	// 每次 ProcessDue 后把 NextRetry 拨回"现在"，模拟时间推进
	for i := 0; i < 3; i++ {
		w.ProcessDue(t.Context())
		row := store.get(it.ID)
		row.NextRetry = time.Now().Add(-time.Second)
	}
	got := store.get(it.ID)
	if got.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED after max retry", got.Status)
	}
	if got.RetryCount != 3 {
		t.Fatalf("retry = %d, want 3", got.RetryCount)
	}
}

func TestWorkerStopReturnsAfterCurrentBatch(t *testing.T) {
	store := newFakeStore()
	it := store.add(dueItem("evt-4"))
	pub := &fakePublisher{failN: map[int64]int{}}
	w := NewWorker(store, pub, Config{Interval: time.Hour}) // 永不 tick，只跑首批

	done := make(chan struct{})
	go func() { w.Start(); close(done) }()

	// 等首批处理完
	deadline := time.After(2 * time.Second)
	for pub.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("worker did not process first batch")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	w.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
	if got := store.get(it.ID).Status; got != StatusSent {
		t.Fatalf("status = %s, want SENT (in-flight batch completed)", got)
	}
	// Stop 可重复调用
	w.Stop()
}

// ---- idempotency ----

func TestApplyOnceIsIdempotent(t *testing.T) {
	ledger := map[string]LedgerEntry{}
	e := LedgerEntry{IdempotencyKey: "RECHARGE:o1:DIAMOND:AVAILABLE", Amount: 100}
	if ok, err := ApplyOnce(ledger, e); err != nil || !ok {
		t.Fatalf("first apply: ok=%v err=%v, want true/nil", ok, err)
	}
	if ok, err := ApplyOnce(ledger, e); err != nil || ok {
		t.Fatalf("second apply: ok=%v err=%v, want false/nil", ok, err)
	}
	if len(ledger) != 1 {
		t.Fatalf("ledger has %d entries, want 1", len(ledger))
	}
}

func TestApplyOnceDetectsConflict(t *testing.T) {
	ledger := map[string]LedgerEntry{}
	_, _ = ApplyOnce(ledger, LedgerEntry{IdempotencyKey: "k", Amount: 100})
	_, err := ApplyOnce(ledger, LedgerEntry{IdempotencyKey: "k", Amount: 200})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestApplyOnceRequiresKey(t *testing.T) {
	if _, err := ApplyOnce(map[string]LedgerEntry{}, LedgerEntry{Amount: 1}); err == nil {
		t.Fatal("want error for empty idempotency key")
	}
}

// TestPaymentCallbackDuplicateAndOutOfOrder 覆盖：重复回调不重复入账，乱序回调不回退状态。
func TestPaymentCallbackDuplicateAndOutOfOrder(t *testing.T) {
	order := OrderPending
	ledger := map[string]LedgerEntry{}
	key := "RECHARGE:o1:DIAMOND:AVAILABLE"

	if ok, _ := ApplyPaymentCallback(&order, ledger, key, 100); !ok {
		t.Fatal("first callback should apply")
	}
	if order != OrderPaid {
		t.Fatalf("order = %s, want PAID", order)
	}
	// 重复回调
	if ok, _ := ApplyPaymentCallback(&order, ledger, key, 100); ok {
		t.Fatal("duplicate callback must not apply again")
	}
	if len(ledger) != 1 {
		t.Fatalf("ledger has %d entries after duplicate, want 1", len(ledger))
	}
	// 乱序：已支付后又收到"关闭"
	if CanTransition(OrderPaid, OrderClosed) {
		t.Fatal("PAID → CLOSED must not be allowed")
	}
	if order != OrderPaid {
		t.Fatalf("order = %s after out-of-order, want PAID", order)
	}
}

func TestStateMachineOnlyMovesForward(t *testing.T) {
	cases := []struct {
		from, to OrderStatus
		want     bool
	}{
		{OrderPending, OrderPaid, true},
		{OrderPending, OrderClosed, true},
		{OrderPaid, OrderClosed, false},
		{OrderPaid, OrderPending, false},
		{OrderClosed, OrderPaid, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("CanTransition(%s→%s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	type evt struct {
		TurnID int64 `json:"turnId"`
	}
	s, err := Encode(evt{TurnID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if s != `{"turnId":7}` {
		t.Fatalf("payload = %s", s)
	}
}
