# 对抗核验附录：MME/TT/MAP 评审逐条裁决（2026-10-19）

> 配套 `2026-10-19-mme-tt-map-review.md`（hotcraftflow-59 产出）。本附录由 apps/server 线
> 独立核验：每条结论均对照当前工作区代码 + 供应商库源码，不盲信评审原文。
> 状态时点：HEAD=3d71e17，TT 线正在并行落修复（lifecycle_eval/influx/doctor/ent schema 已动）。

## 裁决总表

| # | 发现 | 裁决 | 现状 |
|---|------|------|------|
| H1 | dead-letter 审计枚举拒收 | ✅ 确认 | **已修**（schema 加 `deadletter` 值，待 regen+提交） |
| H2 | TaskID 去重被 24h Retention 拖累 | ✅ 确认（机制级） | 未修 |
| H3 | 终态失败吞掉 → pending 永挂 | ✅ 确认（机制修正） | 未修 |
| H4 | AgeDays 语义与 A-8 发散 | ✅ 确认 | **已修**（FirstSeenAt 逐平台按时间加权） |
| H5 | Velocity=-1 哨兵当衰减 | ⚠️ 半确认（诊断偏差） | 等价问题已修 |
| H6 | 按桶时间 _time 排序 | ✅ 确认 | **已修**（t_raw 落列+排序） |
| H7 | pivot 后缺 group | ✅ 确认 | **已修**（group(topic_id) 后 sort/limit） |
| M1 | 每 topic 全量扫 24h 聚合 | ✅ 确认 | 未修 |
| M2 | 幂等键只含 VariantID | ✅ 确认 | **已修**（载荷指纹）+本会话补回归闸 |
| M3 | doctor 探针不看退出码 | ❌ 驳回（有实测依据） | 无需修 |
| M4 | CRF=0 被当合法值 | ✅ 确认 | **已修**（normalize 回填23）+本会话补闸 |
| M5 | 字幕样式双份副本 | ✅ 确认 | **已修**（唯一真相迁 pkg/mmesub） |
| M6 | 256 条上限无游标 | ✅ 确认 | 未修 |
| S1 | data.yaml 真实密码 123456 | ✅ 确认 | 工作区本地值，**提交纪律项** |
| S2 | mme.yaml E:/ 绝对路径 | ✅ 确认 | 工作区本地值，**提交纪律项** |

## 高危逐条核验

### H1 确认 ✅（且已被并行修复）
`ent/schema/map_approval_record.go` 原为 `Values("allow","deny")`，worker 传 `"deadletter"`
→ ResultValidator 拒收 → Record 返 ErrInternal → Warnf 吞掉。后果如评审所述。
修复方向（schema 加第三值）与并行会话落地方案一致，已见其改入。

### H2 确认 ✅（机制级证据）
供应商源码 `repos/asynq@v0.25.1/internal/asynqtest/queue.go`：`claimTask` 以 task.UniqueKey 做
`SET NX` 占位；`DeleteExpiredCompletedTask` 每 8s 只清**已过期**任务——完成任务在 Retention(24h)
内 key 恒在。同库测试 `TestCompleteTaskConflict` 实证：任务完成后 24h 内再 Enqueue 同 TaskID
→ `ErrTaskIDConflict`（注释明言 "info on completed task is preserved during retention period
to avoid duplicate task being enqueued"）。
**联动后果**：终态失败 return nil → asynq 归档为成功 → 24h 内 dueScan 再入队被静默跳过
（Debug 一跳）。评审"每 30s 全量重试"的机制描述不准（30s 只是**尝试入队**频率，实际被冲突拒），
但"静默、无人告警、当日改期失效"的用户态后果完全成立。
**建议修法**（择一）：
- `TaskID = fmt.Sprintf("map-due:%s:%d", unitID, scheduledFor.Unix())` —— 改期即新 key；
- 或失败分支主动 `Inspector.DeleteTask`。

### H3 确认 ✅（机制修正 + 落法约束）
机制修正见 H2。修法有先例可抄：`publish_usecase.go:208-215` 适配器失败时
`UpdateStates(PublishState="failed")` + `auditDeny`（Result="deny"）。worker 终态分支（quota/
visibility/bad-request 等**门拒绝**）应镜像此惯例——**Result 用 `"deny"`**（语义=门裁决），
**不要**用 "deadletter"（后者只留给重试耗尽）。注意补 Reason（auditDeny 不留 reason，worker
侧应留 errBrief）+ ToState="failed"。顺序：先记 fromState 再 UpdateStates，否则审计 FromState 失真。

### H4 确认 ✅（已修）
`lifecycle.go:127` 注释明确定义 AgeDays=样本时间−first_seen_at；旧实现 `now−latest.TRaw`。
数据源现成：`heat_enricher.go` 已在用 `meta.FirstSeenAt`（PG 主存）。并行会话已按
"逐平台 FirstSeenAt 按时间加权"修复——方向正确（跨平台 merge 场景下单一 first_seen 不适用）。

### H5 诊断偏差 ⚠️（等价问题已修）
评审称 "SnapshotPoint.Velocity=-1 是未计算哨兵"——**当前代码无此常量**：types.go 哨兵族是
`SentinelRank/SentinelEngRate/SentinelHeat(-1)/SentinelFactor`，**无 SentinelVelocity**；
`BuildSnapshotPoint` 未回填点 `Velocity: 0`（snapshot.go:114）。即评审引用的常量不存在
（可能源自更早版本）。真风险等价物：**HeatScore=-1** 会命中规则 1A（heat<20 → long_tail），
把未回填点误判长尾——并行会话已加 `latest.HeatScore==SentinelHeat` 守卫。streak 侧
v=0 走 else 分支 break，行为保守（宁可漏判不虚判），可接受。裁决：**关闭**，以热度守卫为准。

### H6/H7 确认 ✅（已修）
Flux 侧已按评审建议修：t_raw 落列、按 t_raw 排序、pivot 后 group(topic_id) 再 limit。
三处证据均与评审引用行一致。

## 中危核验

- **M1 ✅**：`EvaluateOne` 内每次 `ReadActiveHeatRange`（24h group+aggregateWindow 全扫）。
  500 topic×15min≈4.8 万次重查询/天。修法：sweep 入口算一次 P25/P99，注入 EvaluateOne
  （需 worker 签名加参或注入器，属接口演进，建议和 H2/H3 同批做）。
- **M2 ✅（已修+已补闸）**：原 `key := "mme_render_" + v.VariantID`，媒体侧 jobstore
  无限期保留 → QC 编辑后重渲/取消后重试返回旧 done job。**TT 线落地**：指纹键
  `mme_render_<variant>_<fnv64hex16>`（载荷 FNV-1a，见 data/mme_renderer_media.go）；
  **本会话补闸**：修正 stale 断言并加 `TestMediaRendererIdempotencyKey` 矩阵
  （同参重试同键 / 切点、画幅、源路径任一变化即换新键）。
- **M3 ❌ 驳回**：doctor.go 新增注释给出**实测宿主怪癖**——gyan/gcc16 系 ffmpeg 9.x 在 Go
  子进程管道重定向下拉版本探测时间歇性崩溃/挂死（~15ms 正常 | 卡死超时 | 6s 崩溃乱码，
  三种状态轮换），而真实渲染命令稳定退 0。探针语义=「可执行且吐出版本」，退出码是宿主噪音。
  评审的"至少要求退出码 0"修法会在真实生产宿主上把正确二进制打红。另候选参数改并发拉起
  解决了串行最坏 N×timeout。维持现状。
- **M4 ✅（已修+已补闸）**：原 `CRF<0||CRF>51` 放行 0（x264 无损，~10x 码率）；调用方
  省略字段时 json 得 0 通过校验，与 mme.yaml 默认 23 静默不一致。**TT 线落地**：
  `normalize()` 规整层回填 23 + preset 缺省（见 jobrun/mme_render.go 方法注释）；
  **本会话**把内联回填抽取为 `mmeRenderPayload.normalize()` 以便纯函数可测，
  并加 `TestMMERenderNormalize`（7 例：回填/不动/越界不遮盖）。
- **M5 ✅（已修）**：原注释自认与 admin `ForceStyleFor` 同源副本。**已迁唯一真相至
  `pkg/mmesub`**（双侧调用点编译期同步，不再手抄漂移）。
- **M6 ✅**：`analytics_worker.go:137` 窗口锚 now + desc 前 256，无游标/高水位；
  周发 300 单元时最老 44 条永不可达。修法：持久化高水位游标（增量拉取）或分页拉满。

## 卫生项（提交纪律，非代码修复）

- **S1** `conf/data.yaml:7` 工作区 `password=123456`；HEAD 未污染。**回退工作区值会破坏
  本地起服**——正确处理是提交纪律：明名单暂存、严禁 `git add -A`、提交前 `git diff --cached`
  核 HEAD。长期建议 gitignore 本地覆盖文件。
- **S2** `mme.yaml:11-12` E:/ 绝对路径同理（本机 ffmpeg 真实位置）。同上处理。

## 剩余清单（按建议批次）

1. **MAP worker 组（H2+H3）**：TaskID 纳入 scheduled_for；终态分支置 failed+deny 审计
   （Result 语义约束见 H3 节）。文件：worker/map/schedule_worker.go。
2. **M1**：sweep 级 P25/P99 注入。
3. **M6**：高水位游标（需 repo 演进，独立小设计）。

> M2/M4/M5 已随 TT 线落地（本会话逐项复核 + 补闸），不在余留项。

## 与本附录无关

d7537e5（docs/UI + skills）与 8345701 内容已入库，不在本核验范围。
