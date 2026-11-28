#!/usr/bin/env bash
# 校验器的故障注入：在临时副本里逐项注入已知缺陷，确认 check-skill.sh 每个分支都真的会报错。
# 只看"全绿"不能证明校验器有效——它可能只是提前退出了。改了 check-skill.sh 之后跑这个。
#
#   scripts/test-check-skill.sh [--structure-only]
#
# 每个用例：复制一份 skill 到临时目录 → 注入一处缺陷 → 跑 check-skill.sh →
# 期望退出码非零且输出含指定关键字。全程只在临时目录里写，不碰本仓库。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/skill-selftest.XXXXXX")"
trap 'find "$WORK" -depth -delete' EXIT

PASS=0
MISS=0
SCRIPT_ARGS="--structure"
[ "$MODE" = "--structure-only" ] || SCRIPT_ARGS=""

# 每个变异函数都在副本根目录里执行，且只改相对路径。
m_unexpected_field()     { sed -i '' '4i\
version: 1.0.0' SKILL.md; }
m_uppercase_name()       { sed -i '' '2s/.*/name: Cloudwego-skills/' SKILL.md; }
m_leading_hyphen()       { sed -i '' '2s/.*/name: -cloudwego-skills/' SKILL.md; }
m_double_hyphen()        { sed -i '' '2s/.*/name: cloudwego--skills/' SKILL.md; }
m_name_dir_mismatch()    { sed -i '' '2s/.*/name: other-skill/' SKILL.md; }
m_no_frontmatter()       { sed -i '' '1d' SKILL.md; }
m_unclosed_frontmatter() { sed -i '' '7d' SKILL.md; }
m_empty_description()    { sed -i '' '3s/.*/description:/' SKILL.md; }
m_long_description() {
  awk 'NR == 3 { printf "description: "; for (i = 0; i < 1100; i++) printf "x"; print ""; next } { print }' SKILL.md > SKILL.tmp
  mv SKILL.tmp SKILL.md
}
m_todo() {
  sed -i '' '12a\
[TODO: finish this section]' SKILL.md
}
m_too_long()             { for i in $(seq 1 520); do echo "line $i"; done >> SKILL.md; }
m_broken_link()          { sed -i '' 's#(references/architecture.md)#(references/nope.md)#' SKILL.md; }
m_broken_workflow_link() { sed -i '' 's#(workflows/add-rpc-method.md)#(workflows/missing.md)#' SKILL.md; }
m_orphan_doc()           { grep -v 'troubleshooting.md' SKILL.md > SKILL.tmp && mv SKILL.tmp SKILL.md; }
# 过时表述守护的变异要打在"否定语境"的那一行上。行号会随文档编辑漂移，
# 所以用 line_of 按内容定位：line_of <文件> <grep -F 片段>，找不到时让 sed 报错（run 会记为 SETUP FAILED）。
# mutate_line <文件> <定位片段(grep -F)> <sed 替换表达式(不含行号)>
# 只替换定位片段所在的那一行；找不到片段时返回非零，run 会记为 SETUP FAILED。
mutate_line() {
  local file="$1" anchor="$2" expr="$3" n
  n="$(grep -n -F -- "$anchor" "$file" | head -1 | cut -d: -f1)"
  if [ -z "$n" ]; then
    printf 'mutate_line: %s 里找不到 %s\n' "$file" "$anchor" >&2
    return 1
  fi
  sed -i '' "${n}${expr}" "$file"
}
m_stale_gormx()          { mutate_line references/data-and-cache.md '已删除，仓库用' 's#已删除，仓库用#推荐使用，仓库用#'; }
m_stale_validate_session() {
  mutate_line references/hertz-gateway.md '这是未迁移完的遗留，新端点按' 's#这是未迁移完的遗留，新端点按#这是推荐写法，新端点按#'
}
m_stale_json() {
  mutate_line workflows/add-http-endpoint.md '不带信封' 's#不带信封#不带信封。建议保留 c.JSON(consts.StatusOK, resp)#'
}
m_stale_utils_send()    { mutate_line references/troubleshooting.md '已迁到' 's#已迁到#仍在，推荐用#'; }
m_perl_call()           { echo 'perl -pi -e s/a/b/' >> scripts/check-toolchain.sh; }
m_python_call()         { echo 'python3 scripts/x.py' >> scripts/check-toolchain.sh; }
# 下面三条的样本在运行时拼出来：完整的字面量留在源码里，会被 check-skill.sh 的脱敏检查命中。
m_intranet_ip()         { echo "见 http://10.0.$((10 + 2)).34:8080/docs" >> references/architecture.md; }
m_private_key()         { echo "-----BEGIN RSA $(printf 'PRI%s KEY' VATE)-----" >> references/architecture.md; }
m_hardcoded_password()  { echo "password: $(printf 'H%s2' unter)Hunter2" >> references/architecture.md; }
# 代号样本同样在运行时拼出来，避免自检脚本自己被 check-skill.sh 的代号检查命中。
m_internal_codename()   { echo "参考 $(printf 'tu%s' ban) 的做法" >> references/architecture.md; }
m_internal_brandname()  { echo "参考 $(printf '兔%s' 伴) 的做法" >> references/architecture.md; }

# run <用例名> <期望输出里出现的关键字> <变异函数>
run() {
  local name="$1" expect="$2" mutate="$3"
  local copy="$WORK/$name/cloudwego-skills"
  mkdir -p "$copy"
  cp -R "$REPO_ROOT/." "$copy/"
  if ! (cd "$copy" && exec 1>/dev/null 2>&1 && "$mutate"); then
    printf 'SETUP FAILED  %s\n' "$name"
    MISS=$((MISS + 1))
    return
  fi
  local out rc
  out="$(cd "$copy" && bash scripts/check-skill.sh $SCRIPT_ARGS 2>&1)"
  rc=$?
  if [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q -F -- "$expect"; then
    printf 'CAUGHT   %s\n' "$name"
    PASS=$((PASS + 1))
  else
    printf 'MISSED   %s (rc=%s)\n' "$name" "$rc"
    printf '%s\n' "$out" | grep -B1 -A2 -E 'FAIL|unbound|error' | sed 's/^/           /'
    MISS=$((MISS + 1))
  fi
}

# 阳性对照：不做任何变异，必须全绿。
control="$WORK/_control/cloudwego-skills"
mkdir -p "$control"
cp -R "$REPO_ROOT/." "$control/"
if out="$(cd "$control" && bash scripts/check-skill.sh $SCRIPT_ARGS 2>&1)"; then
  printf 'CONTROL  clean copy passes\n'
else
  printf 'CONTROL  clean copy FAILS — 校验器有误报，先修这个\n'
  printf '%s\n' "$out" | grep -A2 FAIL | sed 's/^/           /'
  MISS=$((MISS + 1))
fi

run unexpected-field       "unexpected field 'version'"                  m_unexpected_field
run uppercase-name         "只能含小写字母"                              m_uppercase_name
run leading-hyphen         "不能以连字符开头"                            m_leading_hyphen
run double-hyphen          "不能含连续连字符"                            m_double_hyphen
run name-dir-mismatch      "与目录名"                                    m_name_dir_mismatch
run no-frontmatter         "第一行必须是"                                m_no_frontmatter
run unclosed-frontmatter   "没有用 --- 闭合"                             m_unclosed_frontmatter
run empty-description      "description 非空"                            m_empty_description
run long-description       "1100 字符"                                   m_long_description
run todo                   "含 [TODO"                                    m_todo
run too-long               "SKILL.md 不超过 500 行"                      m_too_long
run broken-link            "references/nope.md"                         m_broken_link
run broken-workflow-link   "workflows/missing.md"                        m_broken_workflow_link
run orphan-doc             "未被引用： references/troubleshooting.md"   m_orphan_doc
run stale-gormx            "references/data-and-cache.md"                m_stale_gormx
run stale-validate-session "references/hertz-gateway.md"                 m_stale_validate_session
run stale-json             "workflows/add-http-endpoint.md"              m_stale_json
run stale-utils-send       "references/troubleshooting.md"               m_stale_utils_send
run perl-call              "scripts/check-toolchain.sh"                  m_perl_call
run python-call            "scripts/check-toolchain.sh"                  m_python_call
run intranet-ip            "references/architecture.md"                  m_intranet_ip
run private-key            "PRIVATE KEY"                                 m_private_key
run hardcoded-password     "references/architecture.md"                  m_hardcoded_password
run internal-codename      "references/architecture.md"                  m_internal_codename
run internal-brandname     "references/architecture.md"                  m_internal_brandname

printf '\ncaught=%d missed=%d\n' "$PASS" "$MISS"
[ "$MISS" -eq 0 ]
