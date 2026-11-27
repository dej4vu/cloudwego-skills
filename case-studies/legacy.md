# 案例：对照库

> 基于 2026-11 的代码快照归纳，已脱敏。保留它是为了对照"同一套技术栈下，哪些约定是稳的、哪些地方主参考库走得更远"。

## 是什么

Go 单仓多服务，Kitex RPC + Hertz HTTP，Protobuf IDL。服务数量与主参考库同量级，业务形态不同（以请求响应式接口为主，没有流式链路）。

## 与主参考库一致的部分

这些是两个代码库共有的基础约定，也是本 skill 的底线：

- 业务逻辑只在 `biz/service/{method}.go`，形状是 `New{Method}Service(ctx)` + `Run(req)`。
- `handler.go`、`rpc_gen/`、`hertz_gen/` 是生成物。
- 网关与 RPC 分开部署；服务间只走 RPC；每张表一个 owner。
- DAO 用 GORM 官方泛型 `gorm.Interface[*model.X]`，缓存内嵌 DAO，没有 `biz/repo/`。
- 都有独立 module `hcommon/`，`common/` 不引入 Hertz。
- Kafka 都走 `common/queue/kq` 封装。
- 构建和生成以 Makefile 为权威；用 cwgo fork + `templates/cwgo/` 自定义模板。

## 与主参考库不同的部分

| 方面 | 对照库 | 主参考库 | 新代码按 |
|---|---|---|---|
| 依赖装配 | 各方法文件自行构造，没有 `init.go` | `init.go` 集中装配 + 窄接口 | 主参考库 |
| 流式 | 无 SSE | server-streaming + SSE | 主参考库 |
| 异步一致性 | 直接发 Kafka | outbox | 主参考库 |
| 响应信封位置 | 各网关 `biz/utils/resp.go` | `hcommon/pkg/responsex` | 主参考库 |
| 响应编码 | `c.JSON`（`encoding/json`） | protojson（`gwjson`） | 主参考库 |
| `timestamp` | 数字 | 字符串 | 跟仓库，不要中途改 |
| 错误分类 | 类型化错误 + 错误文案关键词匹配（`timeout`、`connection`…） | 按分支选入口，不看文案 | 主参考库 |
| 请求绑定 | Hertz 默认解码器 | protojson（`BindConfig`） | 主参考库 |
| DAO 方法命名 | `Find*` / `Get*` 都可能走缓存 | `GetBy*` = 缓存，`FindBy*` = 直查 | 主参考库 |
| 守护脚本 | 无 | `make check-dao`、`make check-proto` | 主参考库 |
| 定时任务 | 有 `app/job` | 无独立服务 | 按需设计 |
| Kitex / Hertz 版本 | `v0.15.4` / `v0.10.4` | `v0.16.4` / `v0.10.6` | 跟仓库 |

## 为什么保留对照库作为案例

它证明了两件事：

1. **基础约定是稳的。** 两个业务形态不同的代码库，在"生成物边界、service 形状、表归属、官方泛型 DAO、缓存内嵌 DAO、`hcommon` 边界、测试分级"上完全一致——这些不是某一项目的偏好，是这套技术栈下的合理默认。
2. **演进是有动因的。** 主参考库的每一项演进都对应对照库里的一个具体的痛：方法文件膨胀、流式无法表达、事件丢失风险、靠错误文案猜分类、看方法名不知道走不走缓存。知道动因，才知道什么时候该用、什么时候不必。

## 不该从对照库照搬的

- 在方法文件里 `dao.NewXxxDAO()` 或直接构造 client。
- 用 `strings.Contains(err.Error(), "timeout")` 这类文案匹配判断错误类别。
- 用原生 `hz update --no_recurse` 后手工修补 blank import（cwgo 路径不需要）。
- 没有 outbox 的业务事件直接发 Kafka。
