package mw

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/errno"
	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/utils"
)

const ctxKeyUserID = "userId"

// session 模拟 user.ValidateSession 的结果。真实仓库里这一步是一次 RPC。
type session struct{ userID int64 }

// validateSession 校验 token。fixture 里 token 形如 "tok-<userId>"。
func validateSession(token string) (*session, error) {
	if !strings.HasPrefix(token, "tok-") {
		return nil, errno.Err_UserTokenInvalid
	}
	var id int64
	if _, err := fmt.Sscanf(strings.TrimPrefix(token, "tok-"), "%d", &id); err != nil || id == 0 {
		return nil, errno.Err_UserTokenInvalid
	}
	return &session{userID: id}, nil
}

// MustLoginMw 解析 Bearer token，校验后把已验证的 userId 放入 RequestContext。
func MustLoginMw() []app.HandlerFunc {
	return []app.HandlerFunc{func(ctx context.Context, c *app.RequestContext) {
		token := strings.TrimPrefix(string(c.GetHeader("Authorization")), "Bearer ")
		sess, err := validateSession(token)
		if err != nil {
			utils.SendErrResponse(ctx, c, http.StatusOK, err)
			c.Abort()
			return
		}
		c.Set(ctxKeyUserID, sess.userID)
		c.Next(ctx)
	}}
}

// UserID 返回中间件已验证的用户 ID；未登录时为 0。
func UserID(c *app.RequestContext) int64 {
	v, _ := c.Get(ctxKeyUserID)
	id, _ := v.(int64)
	return id
}
