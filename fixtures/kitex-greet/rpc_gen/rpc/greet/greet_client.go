package greet

import (
	"context"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet"
	"github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet/greetservice"
)

// RPCClient 是下游 GreetService 的 Kitex 客户端封装。
type RPCClient interface {
	KitexClient() greetservice.Client
	Service() string
	Hello(ctx context.Context, Req *greet.HelloReq, callOptions ...callopt.Option) (r *greet.HelloResp, err error)
}

func NewRPCClient(dstService string, opts ...client.Option) (RPCClient, error) {
	kitexClient, err := greetservice.NewClient(dstService, opts...)
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
	kitexClient greetservice.Client
}

func (c *clientImpl) Service() string {
	return c.service
}

func (c *clientImpl) KitexClient() greetservice.Client {
	return c.kitexClient
}

func (c *clientImpl) Hello(ctx context.Context, Req *greet.HelloReq, callOptions ...callopt.Option) (r *greet.HelloResp, err error) {
	return c.kitexClient.Hello(ctx, Req, callOptions...)
}
