package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/pkg/streaming"
	echo "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
)

var errSend = errors.New("send: broken pipe")

// fakeStream 记录 Run 发出的全部事件，并按需模拟发送失败。
type fakeStream struct {
	ctx    context.Context
	sent   []*echo.EchoResp
	failAt int // 从 1 开始；第 failAt 次 Send 返回 errSend。0 表示不失败。
}

func (f *fakeStream) Send(resp *echo.EchoResp) error {
	if f.failAt > 0 && len(f.sent)+1 == f.failAt {
		return errSend
	}
	f.sent = append(f.sent, resp)
	return nil
}

func (f *fakeStream) Context() context.Context { return f.ctx }

// 以下是 streaming.Stream 的其余方法；本 fixture 只用到 Send / Context。
func (f *fakeStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeStream) SetTrailer(metadata.MD)       {}
func (f *fakeStream) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeStream) Trailer() metadata.MD         { return nil }
func (f *fakeStream) RecvMsg(interface{}) error    { return nil }
func (f *fakeStream) SendMsg(interface{}) error    { return nil }
func (f *fakeStream) Close() error                 { return nil }

var _ streaming.Stream = (*fakeStream)(nil)

func newFakeStream(ctx context.Context) *fakeStream {
	if ctx == nil {
		ctx = context.Background()
	}
	return &fakeStream{ctx: ctx}
}

func types(events []*echo.EchoResp) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.GetType())
	}
	return out
}

func texts(events []*echo.EchoResp) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.GetText())
	}
	return out
}

// assertSeqContiguous 断言 seq 从 1 起每次加 1。
func assertSeqContiguous(t *testing.T, events []*echo.EchoResp) {
	t.Helper()
	for i, event := range events {
		if event.GetSeq() != int64(i+1) {
			t.Fatalf("event %d: seq must be %d, got %d", i, i+1, event.GetSeq())
		}
	}
}

// TestRunEmitsDeltasThenDone 正常路径：delta* → done，seq 从 1 连续递增，分片的 partIndex=0 显式存在。
func TestRunEmitsDeltasThenDone(t *testing.T) {
	stream := newFakeStream(nil)
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "helloworld", ChunkSize: 4}, stream)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := types(stream.sent), []string{"delta", "delta", "delta", "done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types: got %v want %v", got, want)
	}
	if got, want := texts(stream.sent), []string{"hell", "owor", "ld", "helloworld"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts: got %v want %v", got, want)
	}
	assertSeqContiguous(t, stream.sent)
	for i := 0; i < 3; i++ {
		if got := stream.sent[i].GetPartIndex(); got != 0 {
			t.Fatalf("delta %d: partIndex must be 0, got %d", i, got)
		}
	}
}

// TestRunSplitsByRune 多字节字符不能被切成半个。
func TestRunSplitsByRune(t *testing.T) {
	stream := newFakeStream(nil)
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "你好世界", ChunkSize: 2}, stream)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := texts(stream.sent[:2]), []string{"你好", "世界"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts: got %v want %v", got, want)
	}
	if last := stream.sent[len(stream.sent)-1]; last.GetType() != "done" {
		t.Fatalf("last event must be done, got %q", last.GetType())
	}
}

// TestRunUsesStreamContext Run 第一行必须用 stream.Context() 覆盖 ctx：
// 客户端已断开时，不能继续发送任何分片。
func TestRunUsesStreamContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := newFakeStream(ctx)
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "abcdefgh", ChunkSize: 2}, stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if len(stream.sent) != 0 {
		t.Fatalf("no event may be sent after the client disconnects, got %v", types(stream.sent))
	}
}

// TestRunBusinessErrorBecomesErrorEventAndReturnsNil 业务错误：发 error 事件后 Run 返回 nil，
// 客户端看到的是结构化的 error 事件，而不是传输层断流。
func TestRunBusinessErrorBecomesErrorEventAndReturnsNil(t *testing.T) {
	stream := newFakeStream(nil)
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "hello", Fault: "fail_arg"}, stream)
	if err != nil {
		t.Fatalf("business error must return nil, got %v", err)
	}
	if got, want := types(stream.sent), []string{"error"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types: got %v want %v", got, want)
	}
	event := stream.sent[0]
	if event.GetCode() != codeInvalidArgument || event.GetSeq() != 1 {
		t.Fatalf("error event: code=%d seq=%d", event.GetCode(), event.GetSeq())
	}
}

// TestRunMidStreamFailureEndsWithErrorEvent 流中途失败：已发出的分片保留，
// 补一个 error 事件作为终止事件，Run 返回 nil。
func TestRunMidStreamFailureEndsWithErrorEvent(t *testing.T) {
	stream := newFakeStream(nil)
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "hello", ChunkSize: 2, Fault: "fail_send"}, stream)
	if err != nil {
		t.Fatalf("business error must return nil, got %v", err)
	}
	if got, want := types(stream.sent), []string{"delta", "error"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types: got %v want %v", got, want)
	}
	assertSeqContiguous(t, stream.sent)
	if got := stream.sent[1].GetCode(); got != codeStreamBroken {
		t.Fatalf("error event code: got %d want %d", got, codeStreamBroken)
	}
}

// TestRunSendFailureReturnsError 只有 stream.Send 本身失败（对端已断开）才返回非 nil error。
func TestRunSendFailureReturnsError(t *testing.T) {
	stream := newFakeStream(nil)
	stream.failAt = 2 // 第 2 次 Send 失败
	err := NewEchoStreamService(context.Background()).
		Run(&echo.EchoReq{Text: "hello", ChunkSize: 2}, stream)
	if !errors.Is(err, errSend) {
		t.Fatalf("want errSend, got %v", err)
	}
	if got, want := types(stream.sent), []string{"delta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types: got %v want %v", got, want)
	}
}

// TestRunEmptyTextSendsOnlyDone 空文本没有分片，直接发 done（seq=1）。
func TestRunEmptyTextSendsOnlyDone(t *testing.T) {
	stream := newFakeStream(nil)
	err := NewEchoStreamService(context.Background()).Run(&echo.EchoReq{}, stream)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := types(stream.sent), []string{"done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("types: got %v want %v", got, want)
	}
	assertSeqContiguous(t, stream.sent)
}
