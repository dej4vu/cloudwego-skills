#!/usr/bin/env bash
# 对一次 eval 运行产出的工作区做机械断言。只依赖 bash 与标准 unix 工具。
#
#   evals/assertions.sh <workspace> <eval-id> [method] [requires-login]
#
#   workspace  agent 改完后的目标仓库根目录（不是本 skill 目录）
#   eval-id    evals/evals.json 里的 id
#   method          网关端点用例要检查的方法名，如 GetOrderDetail
#   requires-login  yes | no。该端点是否应挂登录校验，由用例语义决定；
#                   省略时不检查中间件（发验证码这类登录前接口本来就不挂）
#
# 每条断言输出一行 `<check> PASS|FAIL|SKIP  <证据>`，退出码 = FAIL 的条数。
# SKIP 表示工作区里找不到对应文件，说明这条断言无法在该工作区判定，不计入失败。

set -uo pipefail

WS="$(cd "${1:?usage: assertions.sh <workspace> <eval-id> [method]}" && pwd)"
EVAL_ID="${2:?missing eval-id}"
METHOD="${3:-}"
REQUIRES_LOGIN="${4:-}"

cd "$WS"

FAILS=0

result() {
  # result <check> <PASS|FAIL|SKIP> <evidence>
  printf '%-36s %s  %s\n' "$1" "$2" "$3"
  [ "$2" != "FAIL" ] || FAILS=$((FAILS + 1))
}

first_match() {
  grep -rn -E "$1" ${2:-.} --include='*.go' --include='*.proto' 2>/dev/null | head -1
}

# ---------- 网关端点 ----------

gateway_endpoint() {
  local handler service_file mw_file
  handler="$(grep -rl -E "^func ${METHOD}\(" app/*/biz/handler --include='*_service.go' 2>/dev/null | head -1)"
  if [ -z "$handler" ]; then
    result handler_three_branches SKIP "apigw handler 里没有方法 ${METHOD}"
    return
  fi
  local pkg
  pkg="$(basename "$(dirname "$handler")")"
  # service 文件名按仓库惯例是 snake_case（SendLoginCode → send_login_code.go）；
  # 找不到就退化为 grep 构造函数，避免因命名差异误判为 SKIP。
  service_file="$(grep -rl -E "^func New${METHOD}Service\(" app/apigw/biz/service --include='*.go' 2>/dev/null | head -1)"
  mw_file="app/apigw/biz/router/${pkg}/middleware.go"

  # 取出该方法的函数体。
  body="$(awk -v m="$METHOD" '
    $0 ~ "^func " m "\\(" { in_fn = 1 }
    in_fn { print }
    in_fn && /^}/ { exit }
  ' "$handler")"
  service_body=""
  [ -f "$service_file" ] && service_body="$(cat "$service_file")"

  # idl_three_annotations
  local idl
  idl="$(grep -rl -E "rpc ${METHOD}\(" idl 2>/dev/null | head -1)"
  if [ -z "$idl" ]; then
    result idl_three_annotations SKIP "idl/ 里没有 rpc ${METHOD}"
  else
    local rpc_block
    rpc_block="$(awk -v m="$METHOD" '
      $0 ~ "rpc " m "\\(" { in_rpc = 1 }
      in_rpc { print }
      in_rpc && /^  }/ { exit }
    ' "$idl")"
    local missing=""
    printf '%s' "$rpc_block" | grep -q 'option (api.post)' || missing="$missing api.post"
    printf '%s' "$rpc_block" | grep -q 'option (google.api.http)' || missing="$missing google.api.http"
    printf '%s' "$rpc_block" | grep -q 'option (openapi.v3.operation)' || missing="$missing openapi.v3.operation"
    if [ -z "$missing" ]; then
      result idl_three_annotations PASS "$idl"
    else
      result idl_three_annotations FAIL "$idl 缺:$missing"
    fi
  fi

  # no_access_token_in_idl
  local req_msg
  req_msg="$(printf '%s\n' "$rpc_block" | sed -n -E 's/^[[:space:]]*rpc [A-Za-z0-9_]+\(([A-Za-z0-9_]+)\).*/\1/p' | head -1)"
  if [ -z "$req_msg" ]; then
    result no_access_token_in_idl SKIP "取不到 ${METHOD} 的请求消息名"
  elif awk -v m="$req_msg" '
    $0 ~ "^message " m "[[:space:]]*\\{" { in_msg = 1 }
    in_msg && /accessToken|access_token/ { found = 1 }
    in_msg && /^}/ { exit }
    END { exit found ? 0 : 1 }
  ' "$idl"; then
    result no_access_token_in_idl FAIL "$idl 的 ${req_msg} 里声明了 accessToken"
  else
    result no_access_token_in_idl PASS "${req_msg}"
  fi

  # handler_three_branches
  local bind_ok run_err_ok success_ok
  bind_ok="$(printf '%s\n' "$body" | awk '/BindAndValidate/ { bind = 1 } bind && /SendBindErrorResponse/ { print "yes"; exit }')"
  run_err_ok="$(printf '%s\n' "$body" | awk '/\.Run\(/ { run = 1 } run && /SendErrResponse/ { print "yes"; exit }')"
  success_ok="$(printf '%s\n' "$body" | grep -c 'SendSuccessResponse')"
  if [ -n "$bind_ok" ] && [ -n "$run_err_ok" ] && [ "$success_ok" -ge 1 ]; then
    result handler_three_branches PASS "bind→SendBindErrorResponse，Run 错误→SendErrResponse，成功→SendSuccessResponse"
  else
    result handler_three_branches FAIL "bind=${bind_ok:-no} run_err=${run_err_ok:-no} success=$success_ok；见 $handler"
  fi

  # no_native_hz_shape
  if printf '%s\n' "$body" | grep -q -E 'c\.String\((consts\.)?Status(BadRequest|OK)|c\.JSON\((consts\.)?StatusOK'; then
    result no_native_hz_shape FAIL "$handler 的 ${METHOD} 仍是 hz 原生形状"
  else
    result no_native_hz_shape PASS "$handler"
  fi

  # service_uses_mw_userid
  if [ -z "$service_body" ]; then
    result service_uses_mw_userid SKIP "没有 $service_file"
    result service_returns_error_only SKIP "没有 $service_file"
  else
    if printf '%s\n' "$service_body" | grep -q -E '\bmw\.UserID\('; then
      result service_uses_mw_userid PASS "$service_file"
    else
      result service_uses_mw_userid FAIL "$service_file 没有用 mw.UserID(s.RequestContext)"
    fi
    if printf '%s\n' "$service_body" | grep -q -E 'c\.JSON\(|RequestContext\)\.JSON\(|consts\.Status[A-Z]'; then
      result service_returns_error_only FAIL "$service_file 自己构造了响应"
    else
      result service_returns_error_only PASS "$service_file"
    fi
  fi

  # middleware_wired：仅当用例声明该端点需要登录时才检查。
  local mw_fn
  mw_fn="_$(echo "$METHOD" | tr 'A-Z' 'a-z')Mw()"
  if [ "$REQUIRES_LOGIN" != "yes" ]; then
    result middleware_wired SKIP "用例未声明需要登录"
  elif [ ! -f "$mw_file" ]; then
    result middleware_wired SKIP "没有 $mw_file"
  elif awk -v fn="${mw_fn%(}" '
    $0 ~ "^func " fn "\\(" { in_fn = 1 }
    in_fn && /MustLoginMw\(\)|MustAdminLoginMw\(\)/ { wired = 1 }
    in_fn && /^}/ { exit }
    END { exit wired ? 0 : 1 }
  ' "$mw_file"; then
    result middleware_wired PASS "$mw_file 的 $mw_fn"
  else
    result middleware_wired FAIL "$mw_file 的 $mw_fn 没有挂登录校验"
  fi

  # build
  local gw_dir
  gw_dir="$(dirname "$(dirname "$(dirname "$(dirname "$handler")")")")"
  if (cd "$gw_dir" && go build ./... >/dev/null 2>&1); then
    result build PASS "$gw_dir"
  else
    result build FAIL "$gw_dir 编译失败"
  fi
}

# ---------- DAO ----------

dao_methods() {
  local dao_dir="${DAO_DIR:-app/file/biz/dao}"
  if [ ! -d "$dao_dir" ]; then
    dao_dir="$(find app -type d -path '*biz/dao' 2>/dev/null | head -1)"
  fi
  if [ -z "$dao_dir" ] || [ ! -d "$dao_dir" ]; then
    result cached_method_named_getby SKIP "工作区里没有 biz/dao"
    result cached_method_uses_cacherepo SKIP "工作区里没有 biz/dao"
    result list_method_direct SKIP "工作区里没有 biz/dao"
    result no_db_field_no_gormx SKIP "工作区里没有 biz/dao"
    return
  fi
  # DAO 文件按惯例叫 {entity}_dao.go，但仓库里有例外（file.go、*_store.go）。
  # 优先按 DAO 类型名定位（DAO_TYPE=FileDAO），否则取目录里第一个 *_dao.go。
  local target
  if [ -n "${DAO_TYPE:-}" ]; then
    target="$(grep -rl -E "^type ${DAO_TYPE} struct" "$dao_dir" --include='*.go' | grep -v _test.go | head -1)"
  else
    target="$(find "$dao_dir" -name '*_dao.go' | grep -v _test.go | head -1)"
  fi
  target="${DAO_FILE:-$target}"
  if [ -z "$target" ] || [ ! -f "$target" ]; then
    result cached_method_named_getby SKIP "$dao_dir 里找不到目标 DAO"
    result cached_method_uses_cacherepo SKIP "$dao_dir 里找不到目标 DAO"
    result list_method_direct SKIP "$dao_dir 里找不到目标 DAO"
    result no_db_field_no_gormx SKIP "$dao_dir 里找不到目标 DAO"
    return
  fi

  # 把文件切成 "签名前一行<US>方法名<US>方法体" 的记录，每个 receiver 方法一条。
  # 分隔符用 0x1F：方法体里有 tab 缩进，不能用 tab 分列。
  # Go 的函数结尾 `}` 一定在行首，嵌套块的 `}` 都有缩进，所以以行首 `}` 作为方法结束。
  records="$(awk '
    BEGIN { FS = "\x1f"; sep = "\x1f" }
    /^func \(d \*?[A-Za-z0-9_]+\) [A-Z][A-Za-z0-9_]*\(/ {
      name = $0; sub(/^func \([^)]*\) /, "", name); sub(/\(.*/, "", name)
      body = ""; prev = last; in_fn = 1; next
    }
    in_fn {
      body = body " " $0
      if ($0 ~ /^}/) { printf "%s%s%s%s%s\n", prev, sep, name, sep, body; in_fn = 0 }
    }
    { last = $0 }
  ' "$target")"
  # 上一行注释里带 cached 的方法名。
  cached="$(printf '%s\n' "$records" | awk 'BEGIN { FS = "\x1f" } $2 != "" && $1 ~ /cached/ { print $2 }')"
  # 缓存读的方法：方法体里调 cacheRepo 的读方法。写后失效（InvalidateCache）不算，
  # 否则 Create / Update 这类写方法会被误判。
  uses_cache="$(printf '%s\n' "$records" | awk 'BEGIN { FS = "\x1f" } $2 != "" && $3 ~ /cacheRepo\.(FindBy|GetBy|Find|Get)[A-Za-z]*\(/ { print $2 }')"

  # cached_method_named_getby：缓存读的方法必须以 GetBy 开头（标了 // cached 的同样要求）。
  bad_cached="$(printf '%s\n%s\n' "$uses_cache" "$cached" | grep -v -E '^(GetBy|$)' | sort -u | tr '\n' ' ')"
  if [ -n "$bad_cached" ]; then
    result cached_method_named_getby FAIL "$target 走缓存却不叫 GetBy*：$bad_cached"
  else
    result cached_method_named_getby PASS "走缓存的方法：$(printf '%s' "$uses_cache" | tr '\n' ' ')"
  fi

  # cached_method_uses_cacherepo：叫 GetBy* 的必须真的是缓存读，且上一行标了 // cached
  getby_bad="$(printf '%s\n' "$records" | awk -v cache="$(printf '%s' "$uses_cache" | tr '\n' ' ')" '
    BEGIN { FS = "\x1f"; n = split(cache, arr, " "); for (i = 1; i <= n; i++) if (arr[i] != "") is_cache[arr[i]] = 1 }
    $2 != "" && $2 ~ /^GetBy/ && (!is_cache[$2] || $1 !~ /cached/) { print $2 }' | tr '\n' ' ')"
  if [ -n "$getby_bad" ]; then
    result cached_method_uses_cacherepo FAIL "$target 的 GetBy* 没有走 cacheRepo 或没标 // cached：$getby_bad"
  else
    result cached_method_uses_cacherepo PASS "$target"
  fi

  # list_method_direct：ListBy* / List* 直查 d.g，不做缓存读
  list_bad="$(printf '%s\n' "$records" | awk 'BEGIN { FS = "\x1f" } $2 != "" && $2 ~ /^List/ && $3 ~ /cacheRepo\.(FindBy|GetBy|Find|Get)[A-Za-z]*\(/ { print $2 }' | tr '\n' ' ')"
  if [ -n "$list_bad" ]; then
    result list_method_direct FAIL "$target 的 List* 做了缓存读：$list_bad"
  else
    result list_method_direct PASS "$target"
  fi

  # no_db_field_no_gormx：整份 DAO 目录都不能有
  local hits
  hits="$(grep -rn -E '^\s+db\s+\*gorm\.DB\b|\b[ds]\.db\b|gormx\.(Interface|G\b)' "$dao_dir" --include='*.go' 2>/dev/null | grep -v '_test.go' || true)"
  if [ -n "$hits" ]; then
    result no_db_field_no_gormx FAIL "$(printf '%s\n' "$hits" | head -3 | tr '\n' ' ')"
  else
    result no_db_field_no_gormx PASS "$dao_dir"
  fi
}

# ---------- 消费幂等 ----------

kafka_consumer() {
  local queue="${QUEUE_DIR:-app/queue}"
  if [ ! -d "$queue" ]; then
    queue="$(find app -maxdepth 2 -type d -name 'queue' 2>/dev/null | head -1)"
  fi
  if [ -z "$queue" ] || [ ! -d "$queue" ]; then
    result request_id_idempotency SKIP "工作区里没有 app/queue"
    result handler_delegates_to_rpc SKIP "工作区里没有 app/queue"
    result no_error_string_matching SKIP "工作区里没有 app/queue"
    result topic_registered SKIP "工作区里没有 app/queue"
    result uses_permanent SKIP "工作区里没有 app/queue"
    result no_raw_payload_logging SKIP "工作区里没有 app/queue"
    return
  fi
  local handler_dir="$queue/biz/handler"

  # request_id_idempotency：common/mq 的消息结构里有 request_id json tag
  if grep -rn -E 'json:"request_id' common/mq >/dev/null 2>&1; then
    result request_id_idempotency PASS "common/mq"
  else
    result request_id_idempotency FAIL "common/mq 的消息结构里没有 request_id"
  fi

  # handler_delegates_to_rpc：handler 文件里不出现 gorm / mysql.DB
  if grep -rn -E 'gorm\.|mysql\.DB|\.Transaction\(' "$handler_dir" --include='*.go' >/dev/null 2>&1; then
    result handler_delegates_to_rpc FAIL "$handler_dir 里直接操作了数据库"
  else
    result handler_delegates_to_rpc PASS "$handler_dir"
  fi

  # no_error_string_matching
  if grep -rn -E 'strings\.(Contains|HasPrefix|HasSuffix)\((err|e)\.Error\(\)' "$queue" "$handler_dir" --include='*.go' 2>/dev/null | grep -v '_test.go:' | grep -q .; then
    result no_error_string_matching FAIL "$(grep -rn -E 'strings\.(Contains|HasPrefix|HasSuffix)\((err|e)\.Error\(\)' "$queue" --include='*.go' | grep -v '_test.go:' | head -1 | cut -c1-120)"
  else
    result no_error_string_matching PASS "$queue"
  fi

  # topic_registered
  if grep -rn 'handlerFactories' "$queue" --include='*.go' >/dev/null 2>&1; then
    result topic_registered PASS "$(first_match 'handlerFactories' "$queue" | cut -c1-110)"
  else
    result topic_registered FAIL "$queue 里没有 handlerFactories"
  fi

  # uses_permanent
  if grep -rn 'kq\.Permanent' "$queue" "$handler_dir" --include='*.go' >/dev/null 2>&1; then
    result uses_permanent PASS "$(first_match 'kq\.Permanent' "$queue" | cut -c1-110)"
  else
    result uses_permanent FAIL "没有用 kq.Permanent 包装必然失败的消息"
  fi

  # no_raw_payload_logging
  if grep -rn -E '(hlog|log)\.[A-Za-z]*f?\(.*msg\.Value\(\)' "$handler_dir" --include='*.go' >/dev/null 2>&1; then
    result no_raw_payload_logging FAIL "$(first_match '(hlog|log)\.[A-Za-z]*f?\(.*msg\.Value\(\)' "$handler_dir")"
  else
    result no_raw_payload_logging PASS "$handler_dir"
  fi
}

# ---------- 入口 ----------

case "$EVAL_ID" in
  1) gateway_endpoint ;;
  2) dao_methods ;;
  3) kafka_consumer ;;
  *)
    printf 'eval %s 没有机械断言（全部留给人工复查）\n' "$EVAL_ID"
    ;;
esac

printf 'eval=%s fails=%s\n' "$EVAL_ID" "$FAILS"
exit "$FAILS"
