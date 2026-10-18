// Package mme 实现 ④ MME 短视频矩阵混剪系统的业务层。
// 对应 docs/systems/04-MME/{01-需求规格,02-概要设计,03-详细设计}.md。
//
// 红线（🔴 不可降级、不参与砍单）：
//  1. QC 硬门禁——boundary_ok/subtitle_ok 任一为 false 的变体不得进入 approved（契约 §4）
//  2. 先展开后渲染、允许删减——渲染只针对 selected=true 的组合（ADR-MME-01）
//  3. 诚实声明——无 GPU 的 reframe 是中心/规则裁切，不得宣称「AI 智能重构」；
//     LLM 降级文案必须标 copy_degraded；依赖缺失报可读错误，禁止猜测产出
//  4. 差异化检查（FR-MIX-05）是风控措施：强提示不硬阻断（ADR-MME-04），
//     但审核台必须可见
//  5. 发布不属于本系统——Dispatch 只产出契约 VariantSet 并发 mm.mme.variant.ready.v1
//
// 分层：biz 只做编排与纯算法；外部依赖（yt-dlp/FFmpeg/whisper/Ollama/NATS）
// 全部走 ports.go 定义的端口，由 data/server 层适配器实现。
package mme
