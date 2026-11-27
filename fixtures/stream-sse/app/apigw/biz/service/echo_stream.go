package service

import (
	"context"
	"errors"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/kitex/client/callopt"

	echo "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/hertz_gen/gw/echo"
	rpcecho "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"
)

// streamRPCTimeout 流式 RPC 的耗时不属于普通 unary 的量级，必须单独设。
const streamRPCTimeout = 5 * time.Minute

// ErrStreamUnavailable 下游流式 RPC 建立失败。此时 SSE 还没开始写，网关仍返回 JSON 信封。
var ErrStreamUnavailable = errors.New("stream rpc unavailable")

// StreamCaller 是网关依赖的下游流式调用面。抽成变量便于测试替换，不建连、不依赖服务发现。
var StreamCaller func(ctx context.Context, req *rpcecho.EchoReq, opts ...callopt.Option) (echoservice.EchoService_EchoStreamClient, error)

type EchoStreamService struct {
	RequestContext *app.RequestContext
	Context        context.Context
}

func NewEchoStreamService(Context context.Context, RequestContext *app.RequestContext) *EchoStreamService {
	return &EchoStreamService{RequestContext: RequestContext, Context: Context}
}

// Run 建立到 echo 的流式 RPC，返回事件流；SSE 编码由 handler 完成。
func (s *EchoStreamService) Run(req *echo.EchoReq) (echoservice.EchoService_EchoStreamClient, error) {
	if StreamCaller == nil {
		return nil, ErrStreamUnavailable
	}
	return StreamCaller(s.Context, &rpcecho.EchoReq{Text: req.GetText(), Fault: req.GetFault()},
		callopt.WithRPCTimeout(streamRPCTimeout))
}
