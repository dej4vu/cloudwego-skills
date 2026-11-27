package service

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/mw"
	greet "github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/hertz_gen/gw/greet"
)

type HelloMeService struct {
	RequestContext *app.RequestContext
	Context        context.Context
}

func NewHelloMeService(Context context.Context, RequestContext *app.RequestContext) *HelloMeService {
	return &HelloMeService{RequestContext: RequestContext, Context: Context}
}

// Run 使用中间件已验证的身份。网关请求体里没有 token 字段。
func (h *HelloMeService) Run(_ *greet.HelloMeReq) (*greet.HelloMeResp, error) {
	uid := mw.UserID(h.RequestContext)
	return &greet.HelloMeResp{UserId: uid, Message: "Hello, me!"}, nil
}
