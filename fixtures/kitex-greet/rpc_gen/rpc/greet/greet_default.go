package greet

import (
	"context"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet"
)

func Hello(ctx context.Context, req *greet.HelloReq, callOptions ...callopt.Option) (resp *greet.HelloResp, err error) {
	resp, err = defaultClient.Hello(ctx, req, callOptions...)
	if err != nil {
		klog.CtxErrorf(ctx, "Hello call failed,err =%+v", err)
		return nil, err
	}
	return resp, nil
}
