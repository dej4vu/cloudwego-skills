package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/dej4vu/cloudwego-skills/fixtures/hertz-greet/app/apigw/biz/router"
)

// envelope 是断言用的响应信封。
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func post(t *testing.T, path, token, body string) (*ut.ResponseRecorder, envelope) {
	t.Helper()
	h := server.New()
	router.GeneratedRegister(h)
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	resp := ut.PerformRequest(h.Engine, "POST", path, &ut.Body{Body: strings.NewReader(body), Len: len(body)}, headers...)
	var env envelope
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", resp.Body.String(), err)
	}
	return resp, env
}

// TestHelloSuccess 成功：HTTP 200，code=0，data 是业务响应。
func TestHelloSuccess(t *testing.T) {
	resp, env := post(t, "/v1/greet/hello", "", `{"name":"Ada","lang":"en"}`)
	assert.DeepEqual(t, resp.Code, 200)
	assert.DeepEqual(t, env.Code, 0)
	assert.DeepEqual(t, env.Msg, "OK")
	if !bytes.Contains(env.Data, []byte(`"message":"Hello, Ada!"`)) {
		t.Fatalf("want greeting in data, got %s", env.Data)
	}
}

// TestHelloBizError 业务错误：HTTP 仍是 200，code 非零，msg 来自 errno。
func TestHelloBizError(t *testing.T) {
	resp, env := post(t, "/v1/greet/hello", "", `{"name":"error"}`)
	assert.DeepEqual(t, resp.Code, 200)
	assert.DeepEqual(t, env.Code, 10001)
	assert.DeepEqual(t, env.Msg, "请求参数不合法")
}

// TestHelloBindError 参数绑定失败：形状与业务错误一致，仍是 200 + code。
func TestHelloBindError(t *testing.T) {
	resp, env := post(t, "/v1/greet/hello", "", `{not json`)
	assert.DeepEqual(t, resp.Code, 200)
	assert.DeepEqual(t, env.Code, 10001)
}

// TestHelloMeRequiresLogin 未登录：中间件拦截，返回登录无效错误码。
func TestHelloMeRequiresLogin(t *testing.T) {
	resp, env := post(t, "/v1/greet/me", "", `{}`)
	assert.DeepEqual(t, resp.Code, 200)
	assert.DeepEqual(t, env.Code, 10002)
	assert.DeepEqual(t, env.Msg, "登录状态无效")
}

// TestHelloMeSuccess 已登录：service 拿到中间件验证过的 userId。
func TestHelloMeSuccess(t *testing.T) {
	resp, env := post(t, "/v1/greet/me", "tok-42", `{}`)
	assert.DeepEqual(t, resp.Code, 200)
	assert.DeepEqual(t, env.Code, 0)
	if !bytes.Contains(env.Data, []byte(`"userId":42`)) &&
		!bytes.Contains(env.Data, []byte(`"userId":"42"`)) {
		t.Fatalf("want userId 42, got %s", env.Data)
	}
}
