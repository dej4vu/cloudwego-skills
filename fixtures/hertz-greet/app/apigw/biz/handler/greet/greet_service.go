package greet

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/service"
	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/utils"
	greet "github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/hertz_gen/gw/greet"
)

// Hello .
// @router v1/greet/hello [POST]
func Hello(ctx context.Context, c *app.RequestContext) {
	var err error
	var req greet.HelloReq
	err = c.BindAndValidate(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	resp := &greet.HelloResp{}
	resp, err = service.NewHelloService(ctx, c).Run(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	utils.SendSuccessResponse(ctx, c, consts.StatusOK, resp)
}

// HelloMe .
// @router v1/greet/me [POST]
func HelloMe(ctx context.Context, c *app.RequestContext) {
	var err error
	var req greet.HelloMeReq
	err = c.BindAndValidate(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	resp := &greet.HelloMeResp{}
	resp, err = service.NewHelloMeService(ctx, c).Run(&req)
	if err != nil {
		utils.SendErrResponse(ctx, c, consts.StatusOK, err)
		return
	}

	utils.SendSuccessResponse(ctx, c, consts.StatusOK, resp)
}
