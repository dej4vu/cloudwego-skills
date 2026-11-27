package main

import (
	"net"
	"os"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	greetservice "github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet/greetservice"

	"github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/app/greet/biz/service"
)

func main() {
	opts := kitexInit()
	service.InitDeps()
	svr := greetservice.NewServer(new(GreetServiceImpl), opts...)
	server.RegisterShutdownHook(service.ShutdownDeps)
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
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "greet"}),
	}
}

func listenAddr() string {
	if v := envOr("KITEX_ADDR", ""); v != "" {
		return v
	}
	return "127.0.0.1:0"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
