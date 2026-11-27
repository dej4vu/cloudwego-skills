package echo

import (
	"encoding/json"
	"errors"
	"io"

	rpcecho "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/hertz-contrib/sse"
)

// cloudwego-skills: sse-bridge EchoStream — registered exception, see references/generated-code-boundary.md
//
// writeSSEStream 把 echo 的流式事件编码为 SSE：event 名为 delta / done / error，data 为事件 JSON。
// 它不是生成物，放在 sse.go 里；handler 里只保留一处调用。
func writeSSEStream(c *app.RequestContext, stream echoservice.EchoService_EchoStreamClient) {
	c.SetStatusCode(consts.StatusOK)
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")
	sseStream := sse.NewStream(c)

	for {
		event, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			// 流中途的传输层错误也要变成 error 事件，客户端不能只看到连接被关闭。
			code, message := int32(codeStreamInterrupted), "stream interrupted"
			if bizErr, ok := kerrors.FromBizStatusError(recvErr); ok {
				code, message = bizErr.BizStatusCode(), bizErr.BizMessage()
			}
			publish(sseStream, &rpcecho.EchoResp{Type: "error", Code: code, Message: message})
			break
		}
		if event == nil {
			continue
		}
		if err := publish(sseStream, event); err != nil {
			break
		}
		if event.Type == "done" || event.Type == "error" {
			break
		}
	}
	c.Flush()
}

// codeStreamInterrupted 传输层中断时网关自己填的错误码。
const codeStreamInterrupted = 50003

func publish(stream *sse.Stream, event *rpcecho.EchoResp) error {
	data, err := marshalEvent(event)
	if err != nil {
		return err
	}
	return stream.Publish(&sse.Event{Event: event.Type, Data: data})
}

// deltaEvent 显式保留 partIndex=0：protobuf 生成的 omitempty 会丢掉首个分段下标。
type deltaEvent struct {
	Type      string `json:"type"`
	ReplyID   string `json:"replyId"`
	Seq       int64  `json:"seq"`
	Text      string `json:"text"`
	PartIndex int32  `json:"partIndex"`
}

func marshalEvent(event *rpcecho.EchoResp) ([]byte, error) {
	if event.GetType() != "delta" {
		return json.Marshal(event)
	}
	return json.Marshal(deltaEvent{
		Type: event.GetType(), ReplyID: event.GetReplyId(), Seq: event.GetSeq(),
		Text: event.GetText(), PartIndex: event.GetPartIndex(),
	})
}
