# 工具链与生成

选哪条生成路径、命令怎么写、生成之后还要做什么。

## 三种生成形态

先分清"生成器"和"骨架来源"。两个参考仓库都不是纯 cwgo 或纯 hz：

| 形态 | 谁在用 | 网关 handler | 响应函数来自 |
|---|---|---|---|
| A. `cwgo server --type HTTP` 生成整套 | 对照库 | 委托 `service.NewX(ctx, c).Run(&req)`；两个失败分支都传 `consts.StatusOK` | 生成的 `biz/utils/resp.go` 桩（`c.JSON`），由项目替换 |
| B. 手工骨架 + 原生 `hz update` 增量 | 主参考库 | 同样委托 service；业务分支传 `consts.StatusBadRequest` | 项目自己的 `hcommon/pkg/responsex` |
| C. 原生 `hz new` 生成整套 | 新仓库、本 skill 的 fixture | 直接 `c.JSON`，**不**生成 service 层和 `biz/utils` | 自己写 |

三种形态的 handler 都是同一套三分支结构（绑定失败 → 业务失败 → 成功），差别只在失败分支的状态码和响应函数的包路径。形态 A、B 的 service 文件要自己写；形态 C 连 service 都没有，要按 [hertz-gateway.md](hertz-gateway.md) 的形状自己加一层。

| | cwgo | 原生 hz / kitex |
|---|---|---|
| 适用 | 仓库已有 `templates/cwgo/` 或 Makefile 里调用 `cwgo` | 仓库没有 cwgo 痕迹，或明确要脱离 cwgo |
| 生成器版本 | 内置：Kitex `v0.16.3`、hz `v0.9.7` | 取决于本机安装版本 |
| 占位 pb 包 | 真实生成到 `hertz_gen/{api,github.com,google.golang.org}` | `--no_recurse` 下不生成，留下悬空 blank import |

仓库里已有一种形态时**不要**换成另一种重新生成：handler 形状不同，混用会得到无法编译或形状漂移的代码。判断仓库是哪种：看 `Makefile` 的 `gen-*gw*` 目标调的是 `cwgo` 还是 `hz`，以及 `app/{gw}/biz/service/` 是否存在。

## 版本矩阵

本 skill 验证过的组合，升级任一项都要同步更新 [compatibility.md](compatibility.md) 并重跑 fixture：

```
cwgo            v0.1.3   (fork, 嵌入 Kitex v0.16.3 + hz v0.9.7 + thriftgo v0.4.5)
Kitex runtime   v0.16.4  (go mod tidy 解析结果，生成器 v0.16.3 与之互通)
Hertz runtime   v0.10.6
Go              1.25 (cwgo 自身) / 1.27 (生成项目)
```

不要用 PATH 里的 `kitex` 二进制生成 cwgo 仓库的代码。本机 `kitex` 若是 `v0.14.1`，其输出与 cwgo 内置的 `v0.16.3` 不同，混用会让 `kitex_gen` 出现无关 diff。

## 命令对照

参数名在 cwgo 和 hz 之间**不同**，这是最常见的报错来源：

| 含义 | cwgo | hz |
|---|---|---|
| proto 搜索路径 | `--proto_search_path` / `-I` | `--proto_path` / `-I` |
| 透传给底层 | `--pass "..."` | 直接给 |
| 处理器目录 | 经 `--pass "--handler_dir ..."` | `--handler_dir` |

### Kitex RPC 服务与客户端（主参考库形状）

```bash
# 客户端 + kitex_gen：在 rpc_gen 模块内执行
cd rpc_gen && cwgo client --type RPC --service ${svc} --module <mod>/rpc_gen \
  --template ../templates/cwgo/kitex-client \
  -I ../idl/rpc --idl ../idl/rpc/${svc}/${svc}.proto && go mod tidy

# 服务骨架：在 app/${svc} 模块内执行，-use 指向已生成的 kitex_gen
cd app/${svc} && cwgo server --type RPC --service ${svc} --module <mod>/app/${svc} \
  --template ../../templates/cwgo/kitex-server \
  --pass "-use <mod>/rpc_gen/kitex_gen" \
  -I ../../idl/rpc --idl ../../idl/rpc/${svc}/${svc}.proto && go mod tidy
```

### Hertz 网关

形态 B（主参考库）的命令。`hz` 的参数名与 cwgo 不同，不能直接把 cwgo 的参数搬过来：

```bash
# 首次：目录里没有 .hz 时必须用 new；update 会报 "can not load .hz"
cd app/apigw && hz new --idl ../../idl/apigw/<pkg>/<pkg>.proto --module <mod>/app/apigw \
  --out_dir . --handler_dir biz/handler --model_dir hertz_gen \
  --proto_path ../../idl/apigw --proto_path "$GOOGLEAPIS_DIR" --proto_path "$GNOSTIC_DIR" \
  --no_recurse --sort_router --unset_omitempty

# 之后：增量
hz update <同上参数>
```

主参考库的 `make gen-gw` / `gen-admingw` 就是对每个 IDL 循环跑这条 `hz update`（admingw 多一个 `--proto_path ../../idl/apigw`）。

形态 A（对照库）的命令：

```bash
cd app/${gw} && cwgo server --pass --unset_omitempty=true \
  -I "../../idl/${gw}" -I "$GOOGLEAPIS_DIR" -I "$GNOSTIC_DIR" \
  --type HTTP --service "${gw}" --module "<mod>/app/${gw}" --idl "$proto"
```

`--unset_omitempty=true` 是 hz 的参数，**必须**跟在 `--pass` 后面透传；cwgo 自己没有这个参数，单独给它是静默无效的（实测：不带 `--pass` 时生成的 struct tag 仍是 `json:"name,omitempty"`）。这个参数决定网关响应里零值字段是否保留，与 protojson 的 `EmitUnpopulated` 是两件事，不要漏。

对照库的 `scripts/gen-gateway.sh` 就是这条命令，之后用 `sed` 删 blank import、`rm` 占位目录（原因见下文"处理占位 pb 包"）。

`cwgo server --type HTTP` 还会顺带生成 `biz/handler/{pkg}/{pkg}_test.go` 的 `ut.PerformRequest` 测试桩（保留、可不填充）。

`GOOGLEAPIS_DIR` 与 `GNOSTIC_DIR` 的来源：

```bash
go mod download github.com/google/gnostic@v0.7.0 github.com/grpc-ecosystem/grpc-gateway@v1.9.5
GNOSTIC_DIR="$(go env GOPATH)/pkg/mod/github.com/google/gnostic@v0.7.0"
GOOGLEAPIS_DIR="$(go env GOPATH)/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.9.5/third_party/googleapis"
```

`api.proto`（hz 的 HTTP 注解扩展）由仓库自带，放在 `idl/apigw/api.proto`，不要从别处拷贝不同版本。

## 生成之后必做

1. **`go mod tidy`。** cwgo 会往已存在的 `go.mod` 追加 `replace github.com/apache/thrift => ... v0.13.0`，即使是纯 protobuf 项目。留着它通常无害，但要在 diff 里确认这是唯一一处 go.mod 变化。
2. **处理占位 pb 包。** hz 会把 IDL 里 import 的 `google.api`、`openapiv3` 依赖也生成到 `hertz_gen/` 下，并在 `.pb.go` 顶部留 blank import。两条路径的问题不同：
   - **cwgo 路径**：占位包被真实生成。但只要仓库里还有别的链路（如 gRPC）带入真实的 `genproto`，运行期会 `proto: file google/api/http.proto is already registered` panic。所以对照库的做法是**两步都做**：先删掉 `.pb.go` 里指向 `hertz_gen/{github.com,google.golang.org}` 的 blank import 行，再删掉这两个占位目录。
   - **原生 hz + `--no_recurse`**：占位包不生成，blank import 悬空，编译失败。修法见 [compatibility.md](compatibility.md)：去掉 `--no_recurse`，或把那一行改指官方包。

   本 skill 不提供 sed / perl 脚本做这件事。新增一个 IDL 包时手改一次 blank import（就一两行），比维护一段正则可靠。
3. **`git diff --stat`。** 预期只变 `idl/`、生成物、`{method}.go` 桩。出现其他文件先停下。
4. **补齐 `.hz`。** 用 `hz new` 初始化后 `.hz` 要提交，否则别人无法 `hz update`。
5. **网关 handler 的绑定分支。** 形态 A、B 生成的 handler 把绑定错误和业务错误都发给 `SendErrResponse`。按 [generated-code-boundary.md](generated-code-boundary.md) 的登记约定，把绑定分支改成 `SendBindErrorResponse`——这是允许手改生成物的第二个例外。`hz update` 和 `cwgo server` 重生成都会逐字节保留已有方法体，所以只需改一次。

## cwgo 默认模板里要替换的占位

`cwgo client` 默认生成的 `rpc_gen/rpc/{svc}/{svc}_init.go` 硬编码了下游地址并在包初始化时建连：

```go
defaultClientOpts = []client.Option{ client.WithHostPorts("127.0.0.1:8888") }
func init() { DefaultClient() }
```

生产仓库用自定义模板去掉这些。若仓库没有自定义模板，不要直接使用 `DefaultClient()`，改为在 `infra/rpc/client.go` 用服务发现显式装配，见 [kitex-service.md](kitex-service.md)。

`cwgo server` 默认模板生成的 `app/{svc}/main.go` 引用了一个**不存在**的包：

```go
kitexlogrus "github.com/kitex-contrib/obs-opentelemetry/logging/logrus"
```

它让 `go mod tidy` 直接失败，连带把 `conf.GetConf()`、`dal.Init()` 一整套脚手架也拖进来。流式方法生成的 `handler.go` 还漏了 `"context"` 的 import。处理方式与主参考库一致：`main.go`、`handler.go` 由项目维护，首次生成后按 [fixtures/kitex-greet](../fixtures/kitex-greet/app/greet) 的形态替换成最小启动代码，`conf/`、`biz/dal/`、`build.sh`、`docker-compose.yaml`、`readme.md` 直接删掉。

## cwgo 默认模板的两个硬伤

默认模板（以及主参考库的 `templates/cwgo/kitex-client`）在**只有流式方法**的服务上生成的 `rpc_gen/rpc/{svc}/*_client.go`、`*_default.go` 编译不过：

```
rpc/echo/echo_client.go:14:17: undefined: context
rpc/echo/echo_default.go:11:21: undefined: context
```

原因：模板无条件写 `ctx context.Context`，但 `context` 只靠 `.Imports` 顺带带入，而 `.Imports` 是从方法的参数/返回类型收集的，纯流式服务收集不到它。同时有 unary 方法的服务能过，只是因为 unary 方法顺带把 `context` 带了进来。

第二个问题：模板用 `range .Imports` 输出 import，而它是 Go map，顺序随机，所以每次生成的 import 顺序可能不同，`gofmt -l` 会报脏，diff 里出现无关改动。Kitex 的模板函数表只有 9 个函数（没有 `keys`/`sort`），在模板里排序做不到。

[templates/cwgo/kitex-client](../templates/cwgo/kitex-client) 是修好的三件套，unary 与纯流式两种服务都验证过编译、`go vet`、`gofmt -l` 全部干净：

| 文件 | `update_behavior` | 修法 |
|---|---|---|
| `client_tpl.yaml` | `cover` | 显式 `"context"`；不再遍历 `.Imports`，改为从 `{{.ImportPath}}` 显式列出消息包与服务包 |
| `default_tpl.yaml` | `cover` | 同上；流式分支补上服务包的导入（返回类型 `{{ToLower .ServiceName}}.Xxx_YyyClient` 定义在那里） |
| `init_tpl.yaml` | `skip` | 不在 `init()` 建连、不硬编码地址；未初始化直接 panic，由 `InitClient` 显式注入 |

```bash
cd rpc_gen && cwgo client --type RPC --service ${svc} --module <mod>/rpc_gen \
  --template "$PWD/../templates/cwgo/kitex-client" \
  -I ../idl/rpc --idl ../idl/rpc/${svc}/${svc}.proto && go mod tidy
```

`--template` 一律传**绝对路径**。cwgo 对不存在的模板目录**静默回退到默认模板**，不报错：相对路径算错时你以为在用自己的模板，实际拿到的是带硬伤的默认输出。

## 纯 protobuf 项目可以去掉 thrift replace

cwgo 追加的 `replace github.com/apache/thrift => ... v0.13.0` 对只用了 protobuf 的模块没有作用。本仓库全部 fixture 都是 `go mod edit -dropreplace=github.com/apache/thrift` 后 `go mod tidy` 通过的。留着无害，去掉能让 `go.mod` 少一条无意义的指令；两种都行，但整个仓库要一致。
