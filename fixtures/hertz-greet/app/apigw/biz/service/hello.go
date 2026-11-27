package service

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/errno"
	greet "github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/hertz_gen/gw/greet"
)

type HelloService struct {
	RequestContext *app.RequestContext
	Context        context.Context
}

func NewHelloService(Context context.Context, RequestContext *app.RequestContext) *HelloService {
	return &HelloService{RequestContext: RequestContext, Context: Context}
}

// Run 网关只做参数校验和转发；fixture 里没有真实 RPC，直接组响应。
func (h *HelloService) Run(req *greet.HelloReq) (*greet.HelloResp, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "error" {
		// 模拟领域 RPC 返回的业务错误，用于验证错误映射
		return nil, errno.Err_Params
	}
	if name == "" {
		name = "world"
	}
	return &greet.HelloResp{Message: "Hello, " + name + "!"}, nil
}
