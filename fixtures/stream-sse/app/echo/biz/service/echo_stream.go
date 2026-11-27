package service

import (
	"context"
	"unicode/utf8"

	echo "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
)

// 业务错误码。真实仓库里它来自 idl/errno/errno.proto 生成的枚举。
const (
	codeInvalidArgument = 40001
	codeStreamBroken    = 50001
)

const defaultChunkSize = 4

// EchoStreamService 承接流式回显。
type EchoStreamService struct {
	ctx context.Context
}

// NewEchoStreamService new EchoStreamService
func NewEchoStreamService(ctx context.Context) *EchoStreamService {
	return &EchoStreamService{ctx: ctx}
}

// Run 把 text 切成若干片依次下发，最后发 done。
// 所有业务失败都编码为 error 事件后返回 nil，让传输层正常收束。
func (s *EchoStreamService) Run(req *echo.EchoReq, stream echo.EchoService_EchoStreamServer) error {
	// 生成的 handler 骨架不透传请求 ctx；连接生命周期以 stream.Context() 为准。
	// 客户端断开时 ctx 被 cancel，这里据此停止后续分片。
	s.ctx = stream.Context()

	if req.GetFault() == "fail_arg" {
		return s.sendErrorEvent(stream, "", 0, codeInvalidArgument, "invalid chunk size")
	}
	size := int(req.GetChunkSize())
	if size <= 0 {
		size = defaultChunkSize
	}
	text := req.GetText()
	replyID := "reply-1"
	seq := int64(0)
	for start := 0; start < len(text); {
		if err := s.ctx.Err(); err != nil {
			// 客户端已断开：不再发送，直接结束。
			return err
		}
		end := start
		for steps := 0; steps < size && end < len(text); steps++ {
			_, width := utf8.DecodeRuneInString(text[end:])
			end += width
		}
		seq++
		// PartIndex 显式写 0：它是合法值，不能因为零值被丢掉。
		if err := stream.Send(&echo.EchoResp{
			Type: "delta", ReplyId: replyID, Seq: seq,
			Text: text[start:end], PartIndex: 0,
		}); err != nil {
			// 只有 stream.Send 本身失败（对端已断开）才返回非 nil error。
			return err
		}
		start = end
		if req.GetFault() == "fail_send" {
			return s.sendErrorEvent(stream, replyID, seq, codeStreamBroken, "stream interrupted")
		}
	}
	seq++
	return stream.Send(&echo.EchoResp{Type: "done", ReplyId: replyID, Seq: seq, Text: text})
}

// sendErrorEvent 在流上发送业务错误事件；发送成功后返回 nil。
func (s *EchoStreamService) sendErrorEvent(
	stream echo.EchoService_EchoStreamServer, replyID string, seq int64, code int32, message string,
) error {
	if err := stream.Send(&echo.EchoResp{
		Type: "error", ReplyId: replyID, Seq: seq + 1, Code: code, Message: message,
	}); err != nil {
		return err
	}
	return nil
}
