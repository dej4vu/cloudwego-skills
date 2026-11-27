package service

import (
	"context"

	greet "github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet"
)

type HelloService struct {
	ctx context.Context
}

// NewHelloService new HelloService
func NewHelloService(ctx context.Context) *HelloService {
	return &HelloService{ctx: ctx}
}

// Run 按语言返回问候语。
func (s *HelloService) Run(req *greet.HelloReq) (resp *greet.HelloResp, err error) {
	lang := normalizeLang(req.GetLang())
	return &greet.HelloResp{
		Message: d().greeter.Greet(lang, req.GetName()),
		Lang:    lang,
	}, nil
}
