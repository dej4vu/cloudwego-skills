package main

import (
	"context"
	"github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/app/greet/biz/service"
	greet "github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet"
)

// GreetServiceImpl implements the last service interface defined in the IDL.
type GreetServiceImpl struct{}

// Hello implements the GreetServiceImpl interface.
func (s *GreetServiceImpl) Hello(ctx context.Context, req *greet.HelloReq) (resp *greet.HelloResp, err error) {
	resp, err = service.NewHelloService(ctx).Run(req)

	return resp, err
}
