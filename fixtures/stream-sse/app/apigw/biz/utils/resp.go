package utils

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 网关本地错误码。真实仓库里它来自 idl/errno/errno.proto 生成的枚举。
const (
	CodeParams            = 10001
	CodeStreamUnavailable = 50002
)

// Response 是网关统一的响应信封。HTTP 状态恒为 200，业务结果在 Code 里。
type Response struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Data      any    `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

// SendSuccessResponse 把 data 包成 code=0 的信封。
func SendSuccessResponse(_ context.Context, c *app.RequestContext, _ int, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Msg: "OK", Data: data, Timestamp: time.Now().Unix()})
}

// SendErrResponse 按"RPC 业务错误 → 兜底参数错误"的顺序映射；状态码由本函数统一决定。
func SendErrResponse(ctx context.Context, c *app.RequestContext, _ int, err error) {
	if bizErr, ok := kerrors.FromBizStatusError(err); ok {
		hlog.CtxErrorf(ctx, "rpc business error: code=%d message=%s", bizErr.BizStatusCode(), bizErr.BizMessage())
		sendError(c, int(bizErr.BizStatusCode()), bizErr.BizMessage())
		return
	}
	hlog.CtxErrorf(ctx, "gateway error: %v", err)
	sendError(c, CodeParams, "请求参数不合法")
}

func sendError(c *app.RequestContext, code int, message string) {
	c.JSON(http.StatusOK, Response{Code: code, Msg: message, Timestamp: time.Now().Unix()})
}
