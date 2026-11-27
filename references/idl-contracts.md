# IDL 契约

Protobuf 怎么写、网关注解怎么加、改动如何保持兼容。

## 分层 `[主]`

| 目录 | 内容 | 消费者 |
|---|---|---|
| `idl/rpc/{svc}/{svc}.proto` | 领域 RPC 服务，`service {Svc}Service` | Kitex 服务端 + 调用方 client |
| `idl/apigw/{pkg}/{pkg}.proto` | 用户端 HTTP，`service {Pkg}Service` | hz → 网关 handler |
| `idl/admingw/{pkg}/admin_{pkg}.proto` | 管理端 HTTP，`service {Domain}AdminService`，包名 `gw.admin{pkg}` | hz → 网关 handler |
| `idl/errno/errno.proto` | 错误码枚举 | 所有服务 |
| `idl/apigw/api.proto` | hz 的 HTTP 注解扩展 | 仅 hz 解析 |

网关 IDL 与 RPC IDL **分开定义**。网关消息是 HTTP 对外的形状，RPC 消息是领域内部的形状；两者之间用 `pack` 显式转换。不要让网关直接复用 RPC 消息，那会把内部字段暴露给 HTTP，并让任一侧的改动被迫同步到另一侧。

## 命名 `[主]`

| 对象 | 规则 | 例 |
|---|---|---|
| 服务 | PascalCase + `Service` | `UserService` |
| RPC 方法 | PascalCase，**单数** | `ListBanner`，不是 `ListBanners` |
| 消息 | PascalCase + 用途后缀，**单数** | `ListBannerReq`、`UserInfo` |
| 字段 | camelCase | `userId`、`nickName` |
| 文件 | 小写 | `user.proto` |
| URL 路径 | 可用复数，与方法名解耦 | `v1/admin/banners` |
| 管理端 | 与用户端重名时加 `Admin` | `ListBannerAdmin` / `ListBannerAdminReq` |

管理端命名的完整规则是两条：与用户端同名同义的方法加 `Admin` 后缀区分（`ListProductAdmin`、`ListUserAdmin`）；`admingw` 独有、用户端没有对应方法的精简命名，不加冗余后缀（`SaveProductDraft`、`CreateUpload`、`AdminLogin`）。后缀加在方法名**末尾**，服务名则是 `{Domain}AdminService`。新加管理端方法先看 `idl/apigw/` 里有没有同名同义的方法：有就加 `Admin`，没有就不加。

文件头固定写法：

```protobuf
syntax = "proto3";

package user;                    // 英文包名
option go_package = "/user";     // Go 包路径
```

## 注释 `[主]`

服务、方法、字段都要有中文注释；字段注释写在**上一行**，不写行尾注释：

```protobuf
// 用户服务
service UserService {
  // 用户登录
  rpc Login(LoginReq) returns (LoginResp) {}
}

message LoginReq {
  // 手机号
  string mobile = 1;
  // 验证码
  string code = 2;
}
```

注释会进入生成的 OpenAPI 文档，是前端的唯一字段说明来源。

## 网关方法必须带的三个注解 `[主]`

```protobuf
rpc GetOrderDetail(GetOrderDetailReq) returns (GetOrderDetailResp) {
  option (api.post) = "v1/order/detail/get";
  option (google.api.http) = { post: "/v1/order/detail/get" body: "*" };
  option (openapi.v3.operation) = { summary: "查询订单详情" };
}
```

- `api.post` 决定 hz 生成的路由。
- `google.api.http` 供 OpenAPI 生成器识别。
- `openapi.v3.operation` 提供文档摘要。

缺少任一项，要么路由不生成，要么文档缺描述。所有网关端点用 **POST**，请求体用 `body: "*"`：网关是 RPC 风格的入口，不是 REST 资源 API，统一 POST 让认证、日志、重试策略只有一套。

## 认证字段 `[主]`

网关请求消息**不声明** `accessToken` / `Authorization` 字段。token 由中间件从 `Authorization: Bearer` 头解析校验，业务层通过 `mw.UserID(c)` 取已验证的身份，再以 `userId` 传给 RPC。

例外只有两处：令牌校验入口本身（`user.ValidateSession`）和登录/刷新的**响应**。

SSE 这类无法设置请求头的链路，才用业务字段传 token，并在注释里说明原因。

## 字段编号与兼容

- 编号从 1 顺序递增，相关字段用连续段；`repeated` 从 20 起是 `[主]` 的习惯。
- **不复用已删除字段的编号**，也不改已有字段的类型或编号。删除字段用 `reserved` 标出编号和名字。
- 新增字段是兼容的；改名在 protobuf 里等价于删除 + 新增，老客户端会丢字段。
- 新增 RPC 方法兼容；删除方法前先确认没有调用方。

## 整型字段宽度 `[主]`

proto 只用 `int32` / `int64` 两种整型。宽度同时决定 JSON 形态：**`int32` 输出数字，`int64` 输出字符串**（protojson 规范，避免 JS 精度丢失）。所以宽度既是存储选择，也是前端契约。

禁止 `uint32` / `uint64` / `sint*` / `fixed*` / `sfixed*`；DB 的 `bigint unsigned` 用 `int64` 承接。禁止用 `int32` 承载 ID 或时间戳。

判定顺序，从上到下命中即停：

| 顺序 | 语义 | 类型 |
|---|---|---|
| 1 | 主键 / 外键 / 业务 ID（`*Id`） | `int64` |
| 2 | 时间点，秒（`*At`） | `int64` |
| 3 | 金额 / 余额 / 数量（`*Minor` / `*Amount` / `*Balance` / `*Diamond`） | `int64` |
| 4 | 字节数（`size` / `*Bytes`） | `int64` |
| 5 | 聚合总量（`total`） | `int64` |
| 6 | 耗时，毫秒（`*Ms`） | `int32` |
| 7 | 等待时长，秒（`*Seconds`） | `int32` |
| 8 | 单轮 / 单条计数（`*Count` / `*Tokens` / `*Attempts` / `*Chars`） | `int32` |
| 9 | 版本号 / 乐观锁（`*Version` / `*Revision`） | `int32` |
| 10 | 分页 / 下标 / 序号（`page` / `pageSize` / `limit` / `index` / `seq`） | `int32` |
| 11 | 百分比与属性（`importance` / `confidence` / `age` / `heightCm`） | `int32` |
| 12 | 错误码（`code`） | `int32` |

优先级与例外：

- **DB 列宽是权威**：列为 `bigint` 时用 `int64`，即使字段名以 `*Count` / `*Version` 结尾；列为 `int` / `mediumint` / `smallint` / `tinyint` 时用 `int32`。
- ID 一律 `int64`（主键均为 `bigint unsigned`），时间戳一律 `int64`（避免 2038，并预留毫秒级）。
- 同名同义字段在 `idl/apigw`、`idl/admingw`、`idl/rpc` 中必须同宽，跨层直接赋值，不做隐式收窄。
- 有存储依据的例外登记到 `scripts/check_proto_int.py` 的 `WHITELIST` 并写明理由。

改完 `.proto` 跑 `make check-proto`，它按上表检查所有整型字段的宽度。

## 流式方法的声明 `[主]`

RPC 侧用 server-streaming：

```protobuf
rpc WatchStream(WatchReq) returns (stream WatchStreamResp) {}
```

网关侧的 IDL 仍是普通 unary 声明，流式语义由 handler 的桥接实现（见 [streaming-sse.md](streaming-sse.md)）：

```protobuf
rpc WatchStream(WatchReq) returns (WatchStreamResp) {
  option (api.post) = "v1/order/order-watch";
  ...
}
```

流式响应消息里带一个 `type` 字段区分事件类型，用 `seq` 做单调递增序号。

## IDL 改动流程

1. 改 `.proto`。
2. 整型字段跑 `make check-proto`。
3. 按 [toolchain-and-generation.md](toolchain-and-generation.md) 重新生成对应层。
4. `git diff` 确认生成物变化与 IDL 改动一一对应。
5. 实现 service 桩；网关侧补 pack。
6. 部署顺序 callee 先于 caller。
