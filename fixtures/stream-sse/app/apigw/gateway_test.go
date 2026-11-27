package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"

	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/biz/router"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/biz/service"
	rpcecho "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"
)

// fakeClientStream 实现 Kitex 生成的 client stream 接口，按脚本吐事件。
type fakeClientStream struct {
	events []*rpcecho.EchoResp
	err    error // 脚本走完后 Recv 返回的错误；nil 表示 io.EOF
	idx    int
	ctx    context.Context
}

func (f *fakeClientStream) Recv() (*rpcecho.EchoResp, error) {
	if f.idx >= len(f.events) {
		if f.err != nil {
			return nil, f.err
		}
		return nil, io.EOF
	}
	event := f.events[f.idx]
	f.idx++
	return event, nil
}

func (f *fakeClientStream) Context() context.Context { return f.ctx }

// 以下是 streaming.Stream 的其余方法；本 fixture 只用到 Recv / Context。
func (f *fakeClientStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeClientStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeClientStream) SetTrailer(metadata.MD)       {}
func (f *fakeClientStream) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeClientStream) Trailer() metadata.MD         { return nil }
func (f *fakeClientStream) RecvMsg(interface{}) error    { return nil }
func (f *fakeClientStream) SendMsg(interface{}) error    { return nil }
func (f *fakeClientStream) Close() error                 { return nil }

var _ echoservice.EchoService_EchoStreamClient = (*fakeClientStream)(nil)

// stubStream 让网关 service 的 Run 返回预设的下游事件流。
func stubStream(t *testing.T, events []*rpcecho.EchoResp, recvErr error) {
	t.Helper()
	service.StreamCaller = func(ctx context.Context, _ *rpcecho.EchoReq, _ ...callopt.Option) (echoservice.EchoService_EchoStreamClient, error) {
		return &fakeClientStream{events: events, err: recvErr, ctx: ctx}, nil
	}
	t.Cleanup(func() { service.StreamCaller = nil })
}

// gatewayAddr 是测试包共用的真实网关地址，由 TestMain 启停。
var gatewayAddr string

// TestMain 起一个真实监听的网关给全部用例共用。SSE 走 chunked writer，依赖真实网络连接，
// 所以不能用 ut.PerformRequest 的 recorder，必须走一次完整 HTTP 往返；
// 优雅关闭要等 keep-alive 连接过期，共用实例避免每个用例都付 5s。
func TestMain(m *testing.M) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	h := server.New(server.WithListener(listener))
	router.GeneratedRegister(h)
	go func() { _ = h.Run() }()
	gatewayAddr = "http://" + listener.Addr().String()
	code := m.Run()
	_ = h.Shutdown(context.Background())
	os.Exit(code)
}

// doPost 发起请求并读完整条响应。
func doPost(t *testing.T, body string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(gatewayAddr+"/v1/echo/stream", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(raw)
}

// postStream 发起流式请求并读完整条 SSE 响应。
func postStream(t *testing.T, body string) (*http.Response, []sseEvent, string) {
	t.Helper()
	resp, raw := doPost(t, body)
	return resp, parseSSE(t, string(raw)), string(raw)
}

// sseEvent 是解析后的一条 SSE 消息。
type sseEvent struct {
	Event string
	Data  map[string]any
}

// parseSSE 解析 hertz-contrib/sse 的输出：每条消息是 `event:<name>` + `data:<json>`，消息之间空一行。
func parseSSE(t *testing.T, raw string) []sseEvent {
	t.Helper()
	var frames []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := strings.Index(string(data), "\n\n"); i >= 0 {
			return i + 2, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		block := scanner.Text()
		if strings.TrimSpace(block) == "" {
			continue
		}
		var frame sseEvent
		var data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				frame.Event = strings.TrimPrefix(line, "event:")
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimPrefix(line, "data:")
			}
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			t.Fatalf("decode sse data %q: %v", data, err)
		}
		frame.Data = payload
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan sse: %v", err)
	}
	return frames
}

func headerOf(resp *http.Response, key string) string {
	return resp.Header.Get(key)
}

func events(frames []sseEvent) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		out = append(out, frame.Event)
	}
	return out
}

// TestSSEHappyPath 事件顺序、seq 连续递增、done 终止、partIndex=0 显式存在、SSE 响应头。
func TestSSEHappyPath(t *testing.T) {
	stubStream(t, []*rpcecho.EchoResp{
		{Type: "delta", ReplyId: "r1", Seq: 1, Text: "hell", PartIndex: 0},
		{Type: "delta", ReplyId: "r1", Seq: 2, Text: "o", PartIndex: 0},
		{Type: "done", ReplyId: "r1", Seq: 3, Text: "hello"},
	}, nil)
	resp, frames, _ := postStream(t, `{"text":"hello"}`)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if got, want := headerOf(resp, "Content-Type"), "text/event-stream"; got != want {
		t.Fatalf("Content-Type: got %q want %q", got, want)
	}
	if got, want := headerOf(resp, "Cache-Control"), "no-cache"; got != want {
		t.Fatalf("Cache-Control: got %q want %q", got, want)
	}
	if got, want := headerOf(resp, "X-Accel-Buffering"), "no"; got != want {
		t.Fatalf("X-Accel-Buffering: got %q want %q", got, want)
	}
	if got, want := events(frames), []string{"delta", "delta", "done"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i, frame := range frames {
		if got := frame.Data["seq"]; got != float64(i+1) {
			t.Fatalf("frame %d: seq got %v want %d", i, got, i+1)
		}
	}
	// 零值字段不能丢：partIndex=0 必须出现在 delta 的 JSON 里。
	for i := 0; i < 2; i++ {
		if got, ok := frames[i].Data["partIndex"]; !ok || got != float64(0) {
			t.Fatalf("delta %d must keep partIndex=0, got %+v", i, frames[i].Data)
		}
	}
	if _, leaked := frames[2].Data["partIndex"]; leaked {
		t.Fatalf("done event must not carry partIndex, got %+v", frames[2].Data)
	}
}

// TestSSEBusinessErrorIsAnEvent 下游用 error 事件表达业务失败时，客户端收到 error 事件而不是断流。
func TestSSEBusinessErrorIsAnEvent(t *testing.T) {
	stubStream(t, []*rpcecho.EchoResp{
		{Type: "error", Seq: 1, Code: 40001, Message: "invalid chunk size"},
	}, nil)
	resp, frames, _ := postStream(t, `{"text":"hello","fault":"fail_arg"}`)
	if got, want := headerOf(resp, "Content-Type"), "text/event-stream"; got != want {
		t.Fatalf("Content-Type: got %q want %q", got, want)
	}
	if got, want := events(frames), []string{"error"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events: got %v want %v", got, want)
	}
	if got := frames[0].Data["code"]; got != float64(40001) {
		t.Fatalf("error code: got %v want 40001", got)
	}
	if got := frames[0].Data["message"]; got != "invalid chunk size" {
		t.Fatalf("error message: got %v", got)
	}
}

// TestSSEStopsAfterTerminalEvent done/error 之后的下游事件不再转发。
func TestSSEStopsAfterTerminalEvent(t *testing.T) {
	stubStream(t, []*rpcecho.EchoResp{
		{Type: "delta", Seq: 1, Text: "a"},
		{Type: "done", Seq: 2},
		{Type: "delta", Seq: 3, Text: "leak"},
	}, nil)
	_, frames, _ := postStream(t, `{"text":"abc"}`)
	if got, want := events(frames), []string{"delta", "done"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events after done must not be forwarded: got %v want %v", got, want)
	}
}

// TestSSETransportErrorBecomesErrorEvent 流中途传输层断开：网关补一个 error 事件，而不是直接关连接。
func TestSSETransportErrorBecomesErrorEvent(t *testing.T) {
	stubStream(t, []*rpcecho.EchoResp{{Type: "delta", Seq: 1, Text: "a"}},
		errors.New("connection reset by peer"))
	_, frames, _ := postStream(t, `{"text":"abc"}`)
	if got, want := events(frames), []string{"delta", "error"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events: got %v want %v", got, want)
	}
	if got := frames[1].Data["message"]; got != "stream interrupted" {
		t.Fatalf("transport error must be reported as a stream interruption: got %v", got)
	}
}

// TestSSETransportBizErrorKeepsCode 传输层返回的是 Kitex 业务错误时，错误码与消息沿用下游的。
func TestSSETransportBizErrorKeepsCode(t *testing.T) {
	stubStream(t, nil, kerrors.NewBizStatusError(40001, "invalid chunk size"))
	_, frames, _ := postStream(t, `{"text":"abc"}`)
	if got, want := events(frames), []string{"error"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events: got %v want %v", got, want)
	}
	if got := frames[0].Data["code"]; got != float64(40001) {
		t.Fatalf("error code: got %v want 40001", got)
	}
	if got := frames[0].Data["message"]; got != "invalid chunk size" {
		t.Fatalf("error message: got %v", got)
	}
}

// TestSSEStreamUnavailableReturnsEnvelope 下游建流失败发生在 SSE 开始之前，仍返回普通 JSON 信封。
func TestSSEStreamUnavailableReturnsEnvelope(t *testing.T) {
	service.StreamCaller = func(ctx context.Context, _ *rpcecho.EchoReq, _ ...callopt.Option) (echoservice.EchoService_EchoStreamClient, error) {
		return nil, errors.New("dial tcp: connection refused")
	}
	t.Cleanup(func() { service.StreamCaller = nil })
	resp, raw := doPost(t, `{"text":"hello"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if got, want := headerOf(resp, "Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Fatalf("Content-Type: got %q want %q", got, want)
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", raw, err)
	}
	if envelope.Code != 10001 {
		t.Fatalf("envelope code: got %d want 10001", envelope.Code)
	}
}
