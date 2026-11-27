package main

import (
	"os"

	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/app/apigw/biz/router"
)

func main() {
	h := server.New(server.WithHostPorts(listenAddr()))
	router.GeneratedRegister(h)
	if err := h.Run(); err != nil {
		panic(err)
	}
}

func listenAddr() string {
	if v := os.Getenv("HERTZ_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:8888"
}
