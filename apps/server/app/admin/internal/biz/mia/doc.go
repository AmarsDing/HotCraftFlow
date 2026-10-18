// Package mia 实现 ③ MIA 市场影响分析系统的业务层。
// 对应 docs/systems/03-MIA/{01-需求规格,02-概要设计,03-详细设计}.md。
//
// 合规红线（🔴 法务底线，不可降级、不参与砍单）：
//   1. disclaimer 强制非空——空卡拒绝落库与推送（NFR-MIA-01）
//   2. 行情依赖一律只读——绝不实现交易/下单/撤单能力（NFR-MIA-02）
//   3. affected[] 必须带有效 confidence；跨资产传导 ≤0.6（NFR-MIA-03）
//   4. affected[] 必须有非空 path（NFR-MIA-04）
//   5. 情感引擎全不可用时输出 unknown 而禁止猜测（NFR-MIA-08）
//
// 分层：biz 只做编排，纯算法在 internal/algo/mia，行情/情感引擎等外部依赖
// 走 data 层适配器（ports.go 定义的端口）。
package mia
