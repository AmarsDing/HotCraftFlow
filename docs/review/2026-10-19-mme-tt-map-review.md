# Review：MME/TT/MAP 工作区在飞改动（2026-10-19）

> 评审范围：`apps/server` 在飞改动（MME 委托渲染、TT 生命周期评估、MAP 调度/分析 worker、doctor 探针、配置）。评审基线：`go build ./...` 全绿。
>
> **状态标注**：`[已入库]` = 已在 HEAD（8345701）；`[工作区]` = 仍在工作区未提交；`[未跟踪]` = 新文件尚未 add。
>
> 归属（2026-10-19 对账闭环）：H1–H7 / M1 / M2 / M4 / M5 / M6 / S1 = **TT 线 hotcraftflow-6e** 统一修复（随 TT commit 收敛；S1=提交前回退+env 覆盖，勿裸回退）；S2 = **MME 线 hotcraftflow-72**（已落盘并验证全绿）；M3 = 核验驳回，维持现状（gyan ffmpeg 9.x 宿主怪癖）。M2/M4/M5 主体 + mmesub 单测作者已确认为 6e。由 hotcraftflow-59 代发并归档。

---

## 高危：逻辑/语义错误（会静默产生错误行为）

### H1. dead-letter 审计行永远写不进 `[已入库]` `worker/map/schedule_worker.go:247`
`recordSchedulerAudit(..., "deadletter", ...)` 传入的 Result 值是 `"deadletter"`，但 ent schema `map_approval_records.result` 是 `field.Enum("result").Values("allow","deny")`——`ResultValidator` 直接拒绝，Record 返回 ErrInternal 后被 Warnf 吞掉。
**后果**：重试全部耗尽、草稿保留等人工重发的场景，**恰恰留不下任何审计行**，操作员只有一条 ephemeral 日志。
**修法**：枚举加 `"deadletter"`（需要 ent 重新生成），或复用现有枚举值 + 在备注字段标注死信。

### H2. asynq TaskID 去重受 24h Retention 影响，失败重排被静默跳过 `[已入库]` `schedule_worker.go:184`
去重依赖 `asynq.ErrTaskIDConflict`，但 asynq 的冲突检查是"task key 在 Redis 中存在"——已完成任务的 key 因 `Retention`(24h) 仍在。同一单元上午失败、下午重排到 14:00，14:00 时 key 还在 → ErrTaskIDConflict → Debug 日志一跳而过。
**后果**：单元永不在新时间触发，无人被告警，直到 24h 后 key 过期。
**修法**：TaskID 纳入 scheduled_for 时间戳（`map-due:du_X:1697641200`），或失败时主动 `Inspector.DeleteTask`。

### H3. 终态发布失败被吞 → pending 永挂 + 30s 无限静默重试 `[已入库]` `schedule_worker.go:239`
`executeDue` 对 `ErrQuotaExceeded`/visibility/bad request 分类为 terminal 后仅 Warn 即 return nil——任务被归档为成功，单元 publish_state 停在 pending/failed，scheduled_for 在过去，下一轮 dueScan 又入队重试，跑完整 preflight 再失败。
**后果**：卡死单元每 30s 一次全量重试，dashboard 永远显示 pending，无 deadletter 行、无告警。
**修法**：terminal 分支里把 publish_state 显式置 failed 并写审计（同 H1 的枚举问题需先解决）。

### H4. AgeDays 语义错误：用了"最新样本陈旧度"而非 A-8 定义的"主题年龄" `[已入库+工作区仍在改]` `biz/tt/lifecycle_eval.go:125`
`StageInput.AgeDays = now - latest.TRaw`（最新样本的陈旧度）；但 `EvalStage` 的 long_tail 规则 1 把 AgeDays 定义为主题年龄（`sample_time - first_seen_at`）。评估器没有 first_seen_at。
**后果**：20 天的老主题只要 feed 在刷新，AgeDays≈0，long_tail 分支永不触发；反之新主题 feed 停 5 天会被误判"老主题"。long_tail 迁移对每个 topic 都与 A-8 规格发散。
**修法**：评估器补 first_seen_at 来源（topic 主表创建时间或首个样本时间）。

### H5. Velocity 哨兵 -1 被当真衰减信号 `[已入库+工作区仍在改]` `lifecycle_eval.go:83`
M1 阶段 normalize 未回填时 SnapshotPoint.Velocity=-1 是"未计算"哨兵；streak 派生循环把 `-1 < 0` 计入 decayStreak。
**后果**：3 个哨兵点 + 0 热 pivot 即可凭空触发 decaying/long_tail，evidence JSON 里 `"v":-1.00` 被当成实测衰减记入审计。
**修法**：streak 循环跳过 `v == -1`（或在进入循环前过滤哨兵点）。

### H6. 按桶时间 `_time` 排序，被当样本时间用 `[已入库]` `data/tt/influx_client.go:118`
`ReadRecentPoints` ORDER BY `_time`（bucket 时间 TBucket），但评估器把 points[0] 当最新样本、用 `p.TRaw` 算 AgeDays/Updated24h/StageSince。样本乱序到达或跨桶边界时桶序≠原始时序。
**后果**：10:00 样本落桶 10:15、10:04 样本落桶 10:00 → 排序后 10:00 在前，AgeDays 用的是较旧样本；streak 也按桶序而非样本序累计，上升序列可被读成中断。
**修法**：查询按 TRaw（原始采样时间字段）排序，或读出后在内存里按 TRaw 重排。

### H7. pivot 后缺 `group(columns:["topic_id"])`：limit 按平台各算一份 `[已入库]` `influx_client.go:126`
sort→limit 直接跟在 pivot 后，每个平台的序列是独立 table，`limit(n)` 对每个 table 生效。topic 级评估 `ReadRecentPoints(topicID,"",6)` 在 3 平台时返回最多 18 行。
**后果**：评估器把 weibo/bilibili/youtube 的 velocity 混在一个降序流里算 streak——微博涨(v=8)、油管崩(v=-3)交替时 streak 抵消为 0；或 3 个平台的负值各一条凑出 DecayStreak=3（一小时内的 decaying 误判）。
**修法**：pivot 后 `group(columns:["topic_id"])` 把多平台序列合并成单表再 sort/limit。

---

## 中危：资源/可用性

### M1. 每个 topic 评估都全量扫 24h 聚合 `[已入库]` `lifecycle_eval.go:106`
每次 `EvaluateOne` 都发一次 `ReadActiveHeatRange`（group+aggregateWindow over 24h）。500 活跃 topic × 15min ≈ 4.8 万重 Flux 查询/天；整轮 sweep 共享同一 (from,to)，P25/P99 本该每轮算一次。
**修法**：sweep 入口算一次分位数，注入每个 EvaluateOne。

### M2. 幂等键只含 VariantID，重渲染永远命中旧 job `[未跟踪]` `data/mme_renderer_media.go:288`
`"mme_render_"+VariantID` + media 侧 jobstore 无限期保留 → QC 失败编辑后重渲、取消后重试都返回旧（done）job 的 stale result_json。
**修法**：幂等键纳入内容版本/渲染序号（`mme_render_{variant}_{contentHash}` 或递增 render_seq）。

### M3. doctor 版本探针只看非空首行不看退出码 `[工作区]` `media/internal/doctor/doctor.go:92`
3 个候选（`-version`/`--version`/`-V`）任一首行非空即 OK=true；坏二进制/包装脚本退非零但吐 usage 到 stderr 时照样绿。
**后果**：CI/新宿主上 doctor 全绿、`Available()=true`，实际渲染任务逐个失败——探针失去存在意义（仅对那台 Windows gyan 怪癖机成立）。
**修法**：保留非空首行宽松度的同时，至少要求退出码为 0（或记录 stderr 首行做二次判别）。

### M4. CRF=0（无损、~10x 码率）被当合法值接受 `[未跟踪]` `media/internal/jobrun/mme_render.go:119`
handler 只校验不钳制；调用方省略 crf 字段时 json 反序列化得 0，通过范围检查，ffmpeg 按 `-crf 0`（x264 无损）跑。300s 1080p 中间产物从几十 MB 变数 GB，与 mme.yaml 默认 23 静默不一致。
**修法**：CRF=0 时回填默认值 23（与本地渲染器对齐），或显式拒绝 0。

### M5. 字幕样式函数与 admin 侧手工复制，可独立漂移 `[未跟踪]` `mme_render.go:93`
`subtitleForceStyle`/`escapeSubFilterPath` 是 `bizmme.ForceStyleFor`/`escapeSubFilterPath` 的手抄副本（注释自认是债）。QC 改字号 14→16 后，delegated 路径仍按 14 烧字。
**修法**：抽到共享包（如 `apps/server/pkg/mmesub`），两侧 import。

### M6. 30 天窗口 256 条上限无分页 `[已入库]` `worker/map/analytics_worker.go:134`
拉取窗锚定 now，receipts 按 published_at≥now-30d 取 desc 前 256。一周发 300 单元的 campaign，最老 44 条永远进不了任何一轮（无游标、无高水位），REST/CSV 对它们报"尚无数据"。
**修法**：持久化高水位游标（last_fetched_published_at），每轮增量；或分页拉满窗口。

---

## 卫生：尚未入库，拦在提交前即可

### S1. 真实密码写进受跟踪配置 `[工作区]` `conf/data.yaml:7`
工作区把 `CHANGE_ME` 改成了 `password=123456`（主库 + 日志库两处）。**HEAD 未污染**——若按现状提交，凭证永久进 git 历史。
**修法**：提交前回退为 `CHANGE_ME`；本地凭证走 gitignore 的覆盖文件或 env。

### S2. ffmpeg 绝对路径硬编码进受跟踪配置 `[工作区]` `app/admin/config/mme.yaml:11`
工作区把 `ffmpeg`/`ffprobe` 改成 `E:/project/HotCraftFlow/apps/server/tools/ffmpeg/bin/...`。注释自认"其他机器改回 PATH 名"——即入库默认对所有其他机器/CI 都是坏的。
**修法**：提交前回退为 `ffmpeg`/`ffprobe`；本机覆盖走 gitignore 或 env。

---

## 评审方法备注

- 评审由 hotcraftflow-59 的 code-review fork 产出，16 项原始发现按严重度归并为本表 15 条。
- 作者线正在活跃编辑上述部分文件（lifecycle_eval.go / doctor.go 等仍是脏工作区），本报告只做发现与建议，不动代码。
- 与本评审无关的 docs/UI + .claude/skills 修复（d7537e5 内容）已随 8345701 完整入库，无需处理。
