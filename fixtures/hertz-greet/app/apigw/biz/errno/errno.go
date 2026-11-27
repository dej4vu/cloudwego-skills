package errno

// Err 是网关本地产生的业务错误码。真实仓库里它来自 idl/errno/errno.proto 生成的枚举，
// 这里内联一份最小版本，让 fixture 不依赖 rpc_gen。
type Err int

const (
	Err_Params           Err = 10001
	Err_UserTokenInvalid Err = 10002
)

func (e Err) Error() string { return e.String() }

func (e Err) String() string {
	switch e {
	case Err_Params:
		return "请求参数不合法"
	case Err_UserTokenInvalid:
		return "登录状态无效"
	}
	return "服务繁忙，请稍后再试"
}
