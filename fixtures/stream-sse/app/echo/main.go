package main

import (
	"net"
	"os"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	echoservice "github.com/dej4vu/cloudwego-skills/fixtures/stream-sse/rpc_gen/kitex_gen/echo/echoservice"
)

func main() {
	opts := kitexInit()
	svr := echoservice.NewServer(new(EchoServiceImpl), opts...)
	if err := svr.Run(); err != nil {
		klog.Error(err.Error())
	}
}

// kitexInit 监听一个本地端口；测试用 0 让系统分配。
func kitexInit() []server.Option {
	addr, err := net.ResolveTCPAddr("tcp", listenAddr())
	if err != nil {
		panic(err)
	}
	return []server.Option{
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "echo"}),
	}
}

func listenAddr() string {
	if v := os.Getenv("KITEX_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:0"
}
