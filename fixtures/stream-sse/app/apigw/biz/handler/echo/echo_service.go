package echo

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/biz/service"
	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/biz/utils"
	echo "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/hertz_gen/gw/echo"
)

// EchoStream .
// @router v1/echo/stream [POST]
func EchoStream(ctx context.Context, c *app.RequestContext) {
	var err error
	var req echo.EchoReq
	err = c.BindAndValidate(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	stream, err := service.NewEchoStreamService(ctx, c).Run(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	writeSSEStream(c, stream)
}
