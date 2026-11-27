#!/usr/bin/env bash
# 校验 skill 自身：结构规范、链接、过时表述、脚本依赖、脱敏。
# 只依赖 bash 与标准 unix 工具（grep / sed / awk / find）；不依赖 perl、python、jq、yq。
# frontmatter 约束与 Agent Skills 规范（https://agentskills.io/specification）一致，
# 可用官方校验器交叉验证：`agentskills validate <绝对路径>`（pip 包名 skills-ref；传 . 会误报）。
#
#   scripts/check-skill.sh              结构 + 守护 + fixture 编译（scripts/verify.sh --build-only）
#   scripts/check-skill.sh --structure  只做结构与守护，不编译

source "$(dirname "$0")/lib.sh"

cd "$REPO_ROOT"

SKILL_MD="SKILL.md"
ALLOWED_FIELDS=" name description license allowed-tools metadata compatibility "
MAX_NAME=64
MAX_DESCRIPTION=1024
MAX_LINES=500

# ---------- frontmatter ----------

# 取 frontmatter 两个 `---` 之间的内容。frontmatter 最多 32 行且必须以 `---` 闭合，
# 否则退出码非零——否则正文会被当成 frontmatter 吞进去，"未闭合"这个错误就查不出来。
frontmatter() {
  awk '
    NR == 1 && $0 != "---" { exit 1 }
    NR > 1 && $0 == "---" { closed = 1; exit }
    NR > 32 { exit 1 }
    NR > 1 { print }
    END { if (!closed) exit 1 }
  ' "$SKILL_MD"
}

# 顶层字段的值。只识别 `key: value` 单行形态；缩进行属于上一个字段，跳过。
fm_value() {
  frontmatter | awk -v key="$1" '
    /^[ \t]/ { next }
    index($0, key ":") == 1 { sub("^" key ":[ \t]*", ""); print; exit }
  '
}

fm_keys() {
  frontmatter | awk '/^[ \t]/ { next } /^[A-Za-z_-]+:/ { sub(/:.*/, ""); print }'
}

begin "SKILL.md frontmatter 结构"
FRONTMATTER_OK=0
if [ "$(sed -n '1p' "$SKILL_MD")" != "---" ]; then
  fail
  printf '    %s 第一行必须是 ---\n' "$SKILL_MD"
elif ! frontmatter >/dev/null; then
  fail
  printf '    frontmatter 没有用 --- 闭合\n'
else
  pass
  FRONTMATTER_OK=1
fi

if [ "$FRONTMATTER_OK" -eq 1 ]; then
  begin "frontmatter 只含规范允许的字段"
  frontmatter_fields="$FAILURES"
  while IFS= read -r key; do
    [ -n "$key" ] || continue
    case "$ALLOWED_FIELDS" in
      *" $key "*) ;;
      *) fail "unexpected field '$key'（允许：${ALLOWED_FIELDS}）" ;;
    esac
  done <<EOF
$(fm_keys)
EOF
  [ "$FAILURES" -ne "$frontmatter_fields" ] || pass

  begin "name 规则且与目录同名"
  name="$(fm_value name)"
  name_errors=""
  [ -n "$name" ] || name_errors="$name_errors 为空；"
  [ "${#name}" -le "$MAX_NAME" ] || name_errors="$name_errors 超过 $MAX_NAME 字符；"
  case "$name" in -*|*-) name_errors="$name_errors 不能以连字符开头或结尾；" ;; esac
  case "$name" in *--*) name_errors="$name_errors 不能含连续连字符；" ;; esac
  if printf '%s' "$name" | LC_ALL=C grep -q '[^a-z0-9-]'; then
    name_errors="$name_errors 只能含小写字母、数字、连字符；"
  fi
  [ "$name" = "$(basename "$REPO_ROOT")" ] || name_errors="$name_errors 与目录名 $(basename "$REPO_ROOT") 不一致；"
  if [ -z "$name_errors" ]; then pass; else fail "$name_errors"; fi

  begin "description 非空且不超过 $MAX_DESCRIPTION 字符"
  description="$(fm_value description)"
  if [ -z "$description" ]; then
    fail "为空"
  elif [ "${#description}" -gt "$MAX_DESCRIPTION" ]; then
    fail "${#description} 字符"
  else
    pass
  fi

  begin "未完成的脚手架占位符"
  if grep -n -E '\[TODO' "$SKILL_MD" >/dev/null; then
    fail "${SKILL_MD} 含 [TODO"
  else
    pass
  fi
fi

begin "SKILL.md 不超过 $MAX_LINES 行（渐进披露）"
lines="$(wc -l < "$SKILL_MD" | tr -d ' ')"
if [ "$lines" -gt "$MAX_LINES" ]; then fail "$lines 行"; else pass; fi

# ---------- 文档链接 ----------

# 去掉围栏代码块与行内代码后，提取 markdown 相对链接的目标。
link_targets() {
  awk '
    /^[ \t]*(```|~~~)/ { fenced = !fenced; next }
    !fenced { print }
  ' "$1" | sed -E 's/`[^`]*`//g' \
    | grep -oE '\]\([^)]+\)' \
    | sed -E 's/^\]\(//; s/\)$//; s/[[:space:]]+"[^"]*"$//' \
    | grep -v -E '^(https?:|mailto:|#)' \
    || true
}

begin "markdown 相对链接都指向存在的文件"
broken="$(for doc in "$SKILL_MD" references/*.md workflows/*.md case-studies/*.md templates/README.md; do
  dir="$(dirname "$doc")"
  link_targets "$doc" | while IFS= read -r target; do
    path="${target%%#*}"
    [ -n "$path" ] || continue
    [ -e "$dir/$path" ] || printf '%s -> %s\n' "$doc" "$target"
  done
done)"
if [ -n "$broken" ]; then
  fail
  printf '%s\n' "$broken" | sed 's/^/    /'
else
  pass
fi

begin "references/ 与 workflows/ 的每篇文档都被引用到"
orphans="$(for doc in references/*.md workflows/*.md; do
  if ! grep -rqF "$(basename "$doc")" --include='*.md' . --exclude="$(basename "$doc")"; then
    printf '%s\n' "$doc"
  fi
done)"
if [ -n "$orphans" ]; then
  fail
  printf '%s\n' "$orphans" | sed 's/^/    未被引用： /'
else
  pass
fi

# ---------- 过时表述守护 ----------

# 每行一条规则：`文件列表|正则`。这些写法在主参考库已不是范式，列出的文档里只能以
# "遗留 / 已删除 / 对照 / hz 原生" 的口吻出现，不能当作目标写法。
# streaming-sse.md 与 add-streaming-endpoint.md 是 SSE 旧稿、整体待重写，不纳入守护。
STALE_RULES='
references/architecture.md references/data-and-cache.md references/troubleshooting.md references/compatibility.md|gormx\.(Interface|G\b)
references/hertz-gateway.md references/generated-code-boundary.md|validateRequestSession
references/architecture.md|\bapp/job\b
references/hertz-gateway.md references/troubleshooting.md references/generated-code-boundary.md|utils\.Send(Success|Err|BindErr)Response
references/hertz-gateway.md workflows/add-http-endpoint.md|c\.JSON\(consts\.StatusOK
'
# 两段式判定：命中行先看是否在"肯定"这个写法，是则一律违规；否则带否定/对照语气才放行。
# 只靠豁免词会被"……这是推荐写法……不要复制这段"这种同行混写绕过。
# 这是启发式而不是证明：词表覆盖不到的近义表达会漏过，所以它只用来拦住"把过时写法写成目标写法"
# 这一类高置信度的回退；语义层面的正确性仍靠 evals 与人工复查。
STALE_AFFIRMING='推荐|应该|必须|直接保留|即可|建议|仍然可用|照此|按此写|沿用|采用'
STALE_EXEMPT='遗留|已删除|对照|不再|未迁移|旧|undefined|改成|不要|坑|存量|hz 原生|原生形状|grep -n'

# 注意：不要把循环放进管道，否则 fail 累加的 FAILURES 会随子 shell 丢失。
while IFS='|' read -r pattern regex; do
  [ -n "$pattern" ] || continue
  hits="$(grep -n -E "$regex" $pattern 2>/dev/null | grep -E "$STALE_AFFIRMING" || true)"
  hits="$hits
$(grep -n -E "$regex" $pattern 2>/dev/null | grep -v -E "$STALE_AFFIRMING" | grep -v -E "$STALE_EXEMPT" || true)"
  hits="$(printf '%s' "$hits" | sed '/^[[:space:]]*$/d')"
  label="$(printf '%s' "$regex" | sed -E 's/\\b//g; s/\(\*\)//g')"
  begin "过时表述守护：$label"
  if [ -n "$hits" ]; then
    fail
    printf '%s\n' "$hits" | sed 's/^/    /'
  else
    pass
  fi
done <<EOF
$STALE_RULES
EOF

# ---------- 脚本依赖与脱敏 ----------

begin "scripts/ 不调用 perl 或 python"
script_hits="$(grep -n -E '(^|[;&|(` ])(perl|python[0-9.]*|uvx|pipx)([ ]|$)' scripts/* | grep -v '^scripts/check-skill.sh:' || true)"
if [ -n "$script_hits" ]; then
  fail
  printf '%s\n' "$script_hits" | sed 's/^/    /'
else
  pass
fi

begin "文档不含内网地址与凭据"
leaks="$(grep -rn -i -E '\b(10\.[0-9]{1,3}|172\.(1[6-9]|2[0-9]|3[01])|192\.168)\.[0-9]{1,3}\.[0-9]{1,3}\b|-----BEGIN [A-Z ]*PRIVATE KEY-----|(password|passwd|api[_-]?key|access[_-]?key)[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9/+_-]{8,}' \
  --include='*.md' --include='*.sh' --include='*.env' --include='*.tmpl' --include='*.yaml' . || true)"
if [ -n "$leaks" ]; then
  fail
  printf '%s\n' "$leaks" | cut -c1-160 | sed 's/^/    /'
else
  pass
fi

begin "不含内部项目代号"
# 代号在运行时拼接：完整的字面量留在源码里会被本检查命中。
# 覆盖整个仓库（含 fixtures、模板、生成物），代号不允许以任何形式出现。
codenames="$(printf '%s %s %s' 'tu''ban' 'le''mon' 'fei''yin')"
brand_names="$(printf '%s %s' '绯''音' '兔''伴')"
codename_regex="$(printf '%s' "$codenames" | tr ' ' '|')|$(printf '%s' "$brand_names" | tr ' ' '|')"
codename_hits="$(grep -rn -i -E "$codename_regex" . --exclude-dir=.git 2>/dev/null || true)"
if [ -n "$codename_hits" ]; then
  fail
  printf '%s\n' "$codename_hits" | cut -c1-160 | sed 's/^/    /'
else
  pass
fi

# ---------- fixture 编译 ----------

if [ "${1:-}" != "--structure" ]; then
  # 与 verify.sh --build-only 相同的检查，但把结果并入本次汇总，不提前 summary。
  source "$REPO_ROOT/scripts/versions.env"
  for mod in "${FIXTURE_MODULES[@]}"; do
    begin "build $mod"
    if out="$(cd "$mod" && go build ./... 2>&1 && go vet ./... 2>&1)"; then
      pass
    else
      fail
      printf '%s\n' "$out" | sed 's/^/    /'
    fi
  done
fi

summary
