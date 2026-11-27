package echo

import (
	"context"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"
)

// RPCClient 是下游 EchoService 的 Kitex 客户端封装。
type RPCClient interface {
	KitexClient() echoservice.Client
	Service() string
	EchoStream(ctx context.Context, Req *echo.EchoReq, callOptions ...callopt.Option) (stream echoservice.EchoService_EchoStreamClient, err error)
}

func NewRPCClient(dstService string, opts ...client.Option) (RPCClient, error) {
	kitexClient, err := echoservice.NewClient(dstService, opts...)
	if err != nil {
		return nil, err
	}
	cli := &clientImpl{
		service:     dstService,
		kitexClient: kitexClient,
	}

	return cli, nil
}

type clientImpl struct {
	service     string
	kitexClient echoservice.Client
}

func (c *clientImpl) Service() string {
	return c.service
}

func (c *clientImpl) KitexClient() echoservice.Client {
	return c.kitexClient
}

func (c *clientImpl) EchoStream(ctx context.Context, Req *echo.EchoReq, callOptions ...callopt.Option) (stream echoservice.EchoService_EchoStreamClient, err error) {
	return c.kitexClient.EchoStream(ctx, Req, callOptions...)
}
