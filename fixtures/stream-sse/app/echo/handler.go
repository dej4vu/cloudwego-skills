package main

import (
	"context"

	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/echo/biz/service"
	echo "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo"
)

// EchoServiceImpl implements the last service interface defined in the IDL.
type EchoServiceImpl struct{}

// EchoStream implements the EchoServiceImpl interface.
func (s *EchoServiceImpl) EchoStream(req *echo.EchoReq, stream echo.EchoService_EchoStreamServer) (err error) {
	ctx := context.Background()
	err = service.NewEchoStreamService(ctx).Run(req, stream)
	return
}
