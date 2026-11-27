# templates/

从 [fixtures/](../fixtures/) 提炼的最小片段。`{{...}}` 是占位符，复制后替换。每个文件头部的注释写明来源 fixture 与要点。

| 目录 | 内容 | 来源 fixture |
|---|---|---|
| `cwgo/kitex-client/` | 修好的 cwgo client 模板，可直接作为 `--template` 使用 | `kitex-greet`、`stream-sse` |
| `kitex/` | 依赖装配、unary service、流式 service | `kitex-greet`、`stream-sse` |
| `hertz/` | 响应信封、SSE 桥接 | `hertz-greet`、`stream-sse` |
| `protobuf/` | 网关端点与流式端点的 IDL 注解 | `hertz-greet`、`stream-sse` |
| `async/` | outbox worker、幂等入账 | `outbox-idempotency` |

`cwgo/kitex-client/` 不是 Go 模板，是 cwgo 的 yaml 模板，三个文件原样使用：

```bash
cd rpc_gen && cwgo client --type RPC --service ${svc} --module <mod>/rpc_gen \
  --template "$PWD/<path-to>/templates/cwgo/kitex-client" \
  -I ../idl/rpc --idl ../idl/rpc/${svc}/${svc}.proto
```

其余 `.tmpl` 文件是给人看的参考片段，不经工具渲染；要完整可运行的实现，直接看对应的 fixture。
