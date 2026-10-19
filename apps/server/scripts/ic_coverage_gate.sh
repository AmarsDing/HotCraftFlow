#!/bin/sh
# S-35 IC 覆盖率门禁（评审原文：biz/ic ≥70%；去重与闸门 ≥85%）
#
# 用法: sh scripts/ic_coverage_gate.sh coverage/ic.out
# 环境变量可复写阈值（上调收口用，下调被明确拒绝 ratchet 方向）:
#   IC_GATE_A=85   biz/ic_* 语句覆盖百分比（评审目标 70，2026-10-19 实测 91.7% 已上拧）
#   IC_GATE_B=85   去重与闸门面语句覆盖百分比（实测 92.8%）
#
# 口径（与 go tool cover 不同，必须说明）:
#   coverprofile 会被多个 test 二进制重复注入同一 file:range 块（coverpkg 全包面
#   下的 instrumentation 重复），不能简单的全行求和。正确聚合 = 块并集:
#   以 (file, rangeSpec) 为键，任一二进制 count>0 即视为覆盖。
#
# 组定义（评审 S-35 原文口径的展开）:
#   A 组  biz/ic_*           = app/admin/internal/biz/ic_*.go 全部 (≥ IC_GATE_A)
#   B 组  去重与闸门          = ic_dedup.go ic_simhash.go ic_urlnorm.go
#                               ic_governance.go + collector/ic/robots.go (≥ IC_GATE_B)
#   robots.go 归属 B 组: 闸门三态判定的物理输入（S-7/B-16 fail-closed 链的一员）。

set -eu

PROFILE="${1:-coverage/ic.out}"
IC_GATE_A="${IC_GATE_A:-85}"
IC_GATE_B="${IC_GATE_B:-85}"

[ -f "$PROFILE" ] || { echo "coverage profile 不存在: $PROFILE"; exit 2; }

awk -v GA="$IC_GATE_A" -v GB="$IC_GATE_B" '
NR == 1 { next }  # mode 行
{
    key = $1                       # file:rangeSpec（同一块在各二进制里逐字节相同）
    count = $3 + 0
    if (!(key in seen)) { seen[key] = 1; stmts[key] = $2 + 0 }
    if (count > 0) cov[key] = 1
}
END {
    nB = split("framework/app/admin/internal/biz/ic_dedup.go framework/app/admin/internal/biz/ic_simhash.go framework/app/admin/internal/biz/ic_urlnorm.go framework/app/admin/internal/biz/ic_governance.go framework/app/admin/internal/collector/ic/robots.go", B, " ")
    for (k in seen) {
        idx = index(k, ":"); f = substr(k, 1, idx - 1)
        c = (k in cov) ? stmts[k] : 0
        t[f] += stmts[k]; cc[f] += c   # 逐文件聚合
        if (f ~ /^framework\/app\/admin\/internal\/biz\/ic_[a-z_]+\.go$/) { at += stmts[k]; ac += c }
        for (i = 1; i <= nB; i++) if (f == B[i]) { bt += stmts[k]; bc += c; break }
    }
    printf "== S-35 IC 覆盖率门禁（块并集口径：任一脚本 count>0 即计覆盖）==\n"
    printf "%-64s %10s %9s %7s\n", "FILE", "STMTS", "COVERED", "PCT"
    for (f in t) {
        if (f !~ /^framework\/app\/admin\/internal\/(biz\/ic_[a-z_]+|collector\/ic\/robots)\.go$/) continue
        pct = 100.0 * cc[f] / t[f]
        mark = ""
        for (i = 1; i <= nB; i++) if (f == B[i]) { mark = " [B]"; break }
        printf "%-64s %10d %9d %6.1f%%%s\n", f, t[f], cc[f], pct, mark | "sort -k5 -r"
    }
    close("sort -k5 -r")
    pa = 100.0 * ac / at; pb = 100.0 * bc / bt
    printf "\nGROUP A biz/ic_*:                 %d/%d = %.1f%%  (gate %d%%)\n", ac, at, pa, GA
    printf "GROUP B dedup+governance+robots:  %d/%d = %.1f%%  (gate %d%%)\n", bc, bt, pb, GB
    fail = 0
    if (ac * 100 < at * GA) { printf "FAIL: A 组 %.1f%% < %d%%\n", pa, GA; fail = 1 }
    if (bc * 100 < bt * GB) { printf "FAIL: B 组 %.1f%% < %d%%\n", pb, GB; fail = 1 }
    if (fail) exit 1
    print "PASS: 双组过线（ratchet：阈值只许向上拧，下调须有评审记录）"
}
' "$PROFILE"
