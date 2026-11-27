package echo

import (
	"github.com/cloudwego/kitex/client"
)

var (
	defaultClient     RPCClient
	defaultDstService = "echo"
)

// DefaultClient 返回显式初始化过的客户端。
// 不在包初始化时偷偷建连，也不硬编码下游地址：地址由调用方通过 InitClient 注入。
func DefaultClient() RPCClient {
	if defaultClient == nil {
		panic("rpc client for " + defaultDstService + " is not initialized; call InitClient first")
	}
	return defaultClient
}

func InitClient(dstService string, opts ...client.Option) {
	defaultClient = newClient(dstService, opts...)
}

func newClient(dstService string, opts ...client.Option) RPCClient {
	c, err := NewRPCClient(dstService, opts...)
	if err != nil {
		panic("failed to init client: " + err.Error())
	}
	return c
}
