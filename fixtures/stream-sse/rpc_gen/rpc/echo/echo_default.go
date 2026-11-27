package echo

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"
)

func EchoStream(ctx context.Context, Req *echo.EchoReq, callOptions ...callopt.Option) (stream echoservice.EchoService_EchoStreamClient, err error) {
	stream, err = defaultClient.EchoStream(ctx, Req, callOptions...)
	if err != nil {
		klog.CtxErrorf(ctx, "EchoStream call failed,err =%+v", err)
		return nil, err
	}
	return stream, nil
}
