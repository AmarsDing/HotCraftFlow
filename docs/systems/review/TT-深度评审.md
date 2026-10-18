已完成四份文档 + 共享契约 + 仓库现状的交叉核验。以下为评审结果。

---

## 一、阻塞级问题

`[B-1] 03-详细设计.md:全文（§1.2 / §2 / §3 无任何 cluster 算法）/ 02-概要设计.md:117 / 04-开发进度.md:82-88 | FR-TRK-03 话题聚类是 P0、占 10 人日、验收「归并正确率 ≥85%」，但三份文档对聚类算法的全部描述只有「按 cluster_id + 语义相似度」这一句话。相似度函数、召回方式、阈值、时间窗、跨语言归并、人工锁定、label 选取、复杂度控制全部缺失 | 无法编码、无法验收、无法估时；85% 的验收项没有标注集就没有判据 | 直接替换为下述可编码方案（补进 03 §2.0）：

```
输入 IntelItem it；候选集 = 近 7d 内、merge_lock=false 的活跃 Topic
Step1 硬键：it.dedup.cluster_id 命中 TopicSource.cluster_key → 直接归入（TopicSource 增列 cluster_key）
Step2 召回（禁止全扫 8000）：
  ① entities 中 confidence≥0.6 的 (type,value) → 倒排索引 topic_entities
  ② SimHash64(title) 分 4 段桶 + 段内 Hamming≤3
  ③ |collected_at - topic.last_seen_at| ≤ 7d
  召回上限 200
Step3 打分 s = 0.45·Jaccard(entities) + 0.25·(1 - Hamming/64) + 0.30·cos(emb)
  ⚠ 跨语言/同义只能靠 s_embed：用 paraphrase-multilingual-MiniLM-L12-v2（或云端 Embedding API），
    中英同事件要求 cos≥0.72；SimHash 与实体法对跨语言完全无效
Step4 s≥0.82 归入；0.70≤s<0.82 归入并置 needs_review=true；s<0.70 新建
Step5 merge_lock：人工合并/拆分后置 true，自动聚类不得再拆散（TopicSource 记 moved_by/moved_at）
Step6 label = TopicSource 中 heat 最高且 gate=pass 的 IntelItem 标题，每 6h 或 heat 变动>10 重选，变更入 TopicLabelHistory
标定：M3 必须先产出 200 条「同事件/不同事件」人工配对标注集做网格搜索（召回≥0.85 且精确≥0.90），
      该标注集是需求 §7 验收项 3 的唯一合法依据，且必须在 M3 第一周产出
```

`[B-2] 03-详细设计.md:325-429（§3 Proto 定义）| 契约 §0.4 强制要求 `TrendSeries` → `media.v1.tt.TrendSeries`，但 §3 只定义了 `Topic` / `StageEvidence` / `Candidate` / `ScoreBreakdown`，**没有任何 `TrendSeries` message**，也没有 `series` / `burst` / `origin` / `heat` 包装 | 契约 §0.4 是强制条款；`TrendSeries` 是 TT 对外第一契约（MIA 需求 §81 明确声明输入含 TrendSeries），缺失即无法提测、契约门禁 0 分 | 补完整 message（ID 前缀 `tp_`，枚举用 string 见 C-10）：

```proto
message TrendSeries {
  string api_version = 1;              // 恒 "1.0"
  string topic_id = 2;
  string label = 3;
  string category = 4;                 // A_hot | B_material | C_market | D_compliance
  string stage = 5;                    // latent|rising|peak|decaying|long_tail（小写，非枚举类型）
  StageEvidence stage_evidence = 6;
  Heat heat = 7;
  repeated SeriesPoint series = 8;     // ≤200 点，见 A-19
  Burst burst = 9;
  Origin origin = 10;
}
message Heat {
  double score = 1; double rank_pct = 2; double eng_pct = 3;   // ★ eng_pct 为新增，见 C-5
  double eng_rate = 4; double freshness = 5;                    // ★ freshness 为新增，见 C-5
  double velocity = 6; double accel = 7;
  Breadth breadth = 8;   HeatFactors factors = 9;
}
message Breadth { int32 platform_count = 1; repeated string platforms = 2; }
message SeriesPoint { string t = 1; string platform = 2; int32 rank = 3; double heat = 4; }
message Burst { bool is_burst = 1; repeated string methods_hit = 2; map<string,double> evidence = 3; }
message Origin { string first_seen_at = 1; string first_platform = 2; string first_url = 3; }
```

`[B-3] 03-详细设计.md:60-72（Topic 表）/ 26-40（topic_heat fields）/ 04-开发进度.md:186（可解释门禁）| 热度四因子的**取值**（rank_pct / eng_pct / freshness / breadth）在 Ent `Topic` 表、InfluxDB `topic_heat` fields 中**均无落点**，只有 `heat_score` 一个结果值。`HeatResult`（03:177-186）算完即弃 | 04 §8「可解释门禁：随机 20 个话题，全部能查到热度四因子取值与权重快照」在物理上不可达；NFR-TT-05「任何分数必须能追溯到具体因子数值」对历史点直接违约 | ① Ent `Topic` 增列 `rank_pct / eng_pct / freshness / breadth / w_rank_eff / w_eng_eff / w_fresh_eff / w_breadth_eff / missing_factors(jsonb) / weight_epoch`；② InfluxDB `topic_heat` 增 fields 同名的 8 个 float + `weight_epoch`；③ 每次 normalize 落盘，删除「只输出不落盘」的写法。

`[B-4] 03-详细设计.md:157（`freshness = exp(-Δt_hours/τ)`, `Δt = now - first_seen_at`）/ 147-149（eng_pct 滚动 7d 分位）| 两个因子都依赖**当前时刻**而非**样本时刻**：freshness 用 `now`，eng_pct 用「当前滚动窗口」。导致任何一个历史点的 heat 都**不可重现**——今天重算昨天的点会得出不同的 heat | 直接击穿 FR-TRK-12 热点档案（US-5「回看某一天的热点」）、FR-TRK-13 回流校正（需要「当时的热度」做偏差基准）、以及所有审计与误报复盘；同时违反 ADR-TT-05「历史数据可比」的立论前提 | 全量改为样本时间口径（详见 A-4 / A-3）：

```
freshness(t) = exp( -(t - first_seen_at)/τ )，t 为样本时间戳，禁止 now()
eng_pct(t)   = ECDF_{platform,category}(窗口 = (t-7d, t])，禁止用当前窗口
breadth(t)   = f(|distinct platform where contributed_at ∈ (t-24h, t]|)
落入 InfluxDB 的 heat_score/velocity/accel 一律为「基于 t 的可重现值」
若确需实时口径，另存字段 heat_score_live，明确仅用于实时看板、不参与历史查询与排序
```

`[B-5] 03-详细设计.md:255-283（三方法投票）/ 02-概要设计.md:233（ADR-TT-04）| 方法一（zscore 残差）与方法二（velocity）在数学上是**同一统计量的两个阈值**，相关系数恒为 1，投票机制完全失效 | ADR-TT-04 声称「投票可互相印证」，实际是「单统计量双阈值」，ADR 立论被自己的公式推翻；需求 §7 验收项 5「100% 附 ≥2 个方法的具体数值」会产出两条数值恒成比例、无独立信息量的伪证据 | 推导与修复见 A-12（结构性修复，非调参可解）。

`[B-6] 03-详细设计.md:192-198（判据表）/ 216-237（状态转移图）| 五条判据**没有覆盖状态空间**：`heat=50, velocity=+2(<6), n_points=5, platform_count=3` 时，非 latent（点数≥3）、非 rising（v<6）、非 peak（heat<60 且 v>0）、非 decaying（v>0）、非 long_tail（heat≥20）→ **无匹配阶段**；且新话题初始阶段未定义（latent 要求 heat<30，若首点 heat=45 则不属任何阶段） | 状态机求值函数无返回分支，实现时必然 panic 或静默留下脏值；`stage` 是契约必填枚举，脏值直接导致 JSON Schema 校验失败 | 加优先级求值 + default 分支 + dormant 阶段（详见 A-8）：

```
按序求值，第一个命中为目标阶段；全不命中 → default = 保持当前阶段（不转移、不重置 stage_since）
0) n_points < 3 → latent（强制，不动）
1) long_tail : heat<20 ∧ age>7d ∧ 近24h有更新
2) peak      : heat≥80 ∨ (v_ewma<0 ∧ heat≥60 ∧ 上阶段∈{rising,peak})
3) rising    : v_ewma>6.0 ∧ 持续≥2点
4) decaying  : v_ewma<0 ∧ 持续≥3点
5) default   : 保持当前阶段
新增 dormant：近48h无更新 → dormant（出看板、不产候选、7d 后 is_archived=true），与 long_tail 互斥
```

`[B-7] 03-详细设计.md:205-212（`min_dwell = {latent:0, rising:2 点, ...}` + `状态转移前检查：now - stage_since >= min_dwell(current)`）| 单位错配：`min_dwell` 定义为**采样点数**，检查式却用**墙钟时长**（`now - stage_since`）。采样间隔可配 2–60 分钟（FR-TRK-01），「2 点」在 30 分钟档是 1 小时、在 2 分钟档是 4 分钟，防抖强度差 15 倍 | 防抖参数失去意义；配置里 `min_dwell_points` 与代码里的时长判断无法对应，调参必然出错 | 统一为点数计数：

```
dwell_points(current) = 落在该阶段内的实际样本点数（由 stage_since 之后写入的点数统计，重启后从 InfluxDB count 恢复）
转移前置条件：dwell_points(current) >= min_dwell_points[current]
删除 "now - stage_since >= min_dwell(current)" 的时长写法
min_dwell_points = {latent:0, rising:2, peak:1, decaying:3, long_tail:6, dormant:0}
```

`[B-8] 03-详细设计.md:118（02 概要）/ 42（幂等注释）/ 全文 | 快照点的时间戳 `t` **来源从未定义**。若取「ingest 处理时刻 now」，则 JetStream 至少一次投递产生的新 `t` 会**追加**而非覆盖，NFR-TT-07 直接失效；且迟到数据（IC 回放、sidecar 补采）无落点 | 幂等（NFR-TT-07）、迟到回填、跨平台时间对齐、EWMA 的 Δt 四件事同时悬空 | 见 A-14 的 `t` 取值与时间桶对齐规则（含 `_as_of` 窗口校验与 late 标记）。

`[B-9] 03-详细设计.md:505-509 / 02-概要设计.md:157-162 / 01-需求规格.md:118 | InfluxDB 降级链路三个数字互相矛盾且不可执行：① 缓冲用「Redis TTL 1h」，而 NFR-TT-01 要求「不丢点」——宕机 >1h 必丢；② §6 告警阈值 `tt_influx_buffer_depth > 5000` 按 500 点/秒仅相当 **10 秒**缓冲，与 TTL 1h（180 万点）差 360 倍；③ 无容量上限、无死信、无重放失败处理、无背压 | 降级路径在真实故障下会先 OOM Redis 再静默丢数据，且「不静默返空」（NFR-TT-08）与「不丢点」（NFR-TT-01）同时违约 | 见 A-16（Redis Stream 死信队列 + XPENDING 重放 + 背压阈值重定 + 诚实降级提示）。

`[B-10] 01-需求规格.md:96（可静音）/ 03-详细设计.md:109-120（Alert 表）/ 02-概要设计.md:208（`POST /alerts/{id}/mute`）| 契约 §7 明确「`state=muted` 人工静音后**不再重复推送同一 topic**」——静音粒度是 **topic**；实现是「静音单条 Alert」，且无 `TopicMute` 表、无冷却窗口、无全局限流 | ① 契约语义无法实现；② 一个持续突发 12 小时的话题（30 分钟采样）会产生 24 条重复告警 → 推送疲劳直接触发需求 §9.3 所述的「用户关掉告警」；③ 无抑制手段 | 见 A-13（TopicMute 表 + 三级抑制：静音 / 冷却 / 全局预算 + BurstEvent 审计表）。

`[B-11] 03-详细设计.md:27-32 / 04-开发进度.md:27-32 | 里程碑时序自相矛盾：M0 前置就绪（InfluxDB bucket 已建、契约冻结）计划完成 **2027-01-09**，而 M1 存储与接入（依赖 bucket 已建）计划完成 **2026-12-19**，M0 比 M1 晚 21 天；§5 又写「建议 M0 前置工作提前到 2026-12-14 前完成」，与 §2 的 M0=2027-01-09 冲突；§1「计划窗口 2027-01-12 → 2027-02-06」不含 M1 | 排期不可执行，M1 在 M0 之前无法开工 | M0 改为 **2026-12-14**（Phase 1 启动前 1 天），并列入 §2 表格首行；M1–M5 顺延；§1 计划窗口改为「2026-12-15 → 2027-02-06（含 W1 共享契约）」。

---

## 二、严重问题

`[S-1] 04-开发进度.md:32（M5 DoD）+ 110 | M5 DoD 含「7 日稳定性观察：缺失率 <2%」，而 M5 计划完成日 2027-02-06 就是整个计划窗口最后一天 | 要求「交付当天已完成 7 天观察」= 需在 2027-01-30 前功能冻结，但 M4 恰好 01-30 才完成 → 数学上不可能 | 二选一：① 计划窗口延至 2027-02-13，把观察期独立为 M6；② M5 DoD 改为「连续 72h 缺失率 <2%」，并在 M5 后加 M6（2027-02-13）做 7 日观察。**不得**把 7 日观察塞进窗口末日。`

`[S-2] 04-开发进度.md:131-133 | 工时算术错误且产能高估：`42 人日 ÷ 2 人 ÷ 5 日/周 = 4.2 周`，文档却写「约 2.1 周（2 人并行）」；且「84 人日 ≈ 80 人日容量」已超售，80 人日还是 100% 利用率的假设 | 按 2 人 8 周、实际可用产能 65%（扣会议/上下文切换/缺陷修复）≈ 52 人日，TT 独占 42 人日意味着 IC 只剩 10 人日——与 IC 独立估算的 42 人日严重冲突 | 重算：`总容量 = 2人 × 8周 × 5日 × 0.65 ≈ 52 人日`；TT 42 + IC 42 = 84 人日 → 缺口 32 人日。必须：① 把 §5 结论改为「**容量缺口约 32 人日，需砍单或加人**」；② 前端 4 个页面（选题台/话题详情/看板/订阅/告警中心共 5 个）+ 因子面板从未计入 42 人日，需单列 ≥14 人日。`

`[S-3] 03-详细设计.md:157（freshness）/ 196-198（peak / long_tail 阈值）| freshness 以 `first_seen_at` 为基准且无地板，导致：话题在 24h 后 freshness = e⁻² = 0.135，仅贡献 **2.7 分**；48h 后 0.37 分。即 `peak`（heat≥80）要求剩余三因子同时取满 0.79——现实中 rank_pct 0.9 / eng_pct 0.8 / breadth 0.6 的组合只有 66 分，**话题活过 24 小时就永远进不了 peak**；反过来 `long_tail`（heat<20）要求 0.35r+0.25e+0.2b < 0.2，一个仍挂在榜首的长青话题（r=1.0 → 35 分）**永远进不了 long_tail**，与需求 §3.2「低热度但持续存在」直接矛盾 | 阶段分布会系统性失真：所有老话题卡在 decaying，peak 只可能出现在话题诞生后 12 小时内 | ① freshness 加地板 `f = max(exp(-Δt/τ), f_floor)`，`f_floor = 0.05`；② peak 判据改为相对判据 `heat ≥ 80 ∨ heat ≥ 近 24h 全体活跃话题 heat 的 P99`；③ long_tail 判据改为 `heat < 20 ∨ (heat < 近 24h P25 ∧ age > 7d ∧ 近 24h 有更新)`，并新增 `dormant` 兜底（见 B-6）。`

`[S-4] 03-详细设计.md:158（`breadth = min(1, log1p(pc)/log1p(8))`）| ① 单平台即 `ln2/ln9 = 0.3155`，白送 6.3 分，「跨平台扩散度」对单平台话题给出 31.55% 的扩散度，语义错误；② 若 IC 实际覆盖平台数 < 8（doc §1.2 枚举的 weibo/bilibili/youtube 等，契约未定总数），则 breadth **上限恒 <1**（覆盖 5 个平台时上限仅 0.8155），该因子永远取不满，等价于隐性降权 | 因子量纲错误 + 隐性权重漂移（平台覆盖变化时 breadth 分布整体平移，与 ADR-TT-05 担心的口径漂移同源） | 见 A-5：`ref = min(8, len(supported_platforms))`，并做零点平移 `breadth = clamp((raw - b0)/(1 - b0), 0, 1)`，`b0 = ln2/ln(ref+1)`。`

`[S-5] 03-详细设计.md:142-152 / 01-需求规格.md:56 | eng 因子口径三重矛盾：① 需求写「互动数 / 曝光数」，详细设计写 `(likes+comments)/max(views,1)`（无分享/收藏）；② 详细设计同时定义 `eng_rate` 与 `eng_pct`，但 heat 公式用的是 `eng_pct`，而「有曝光时」只定义了 `eng_rate`——**有曝光时 eng_pct 等于什么，全文未定义**；③ 微博类平台 `views` 恒为 0（契约 §1 metrics 无曝光字段来源），永远走「对数分位」分支，B 站走「比率」分支 → **同一个 eng_pct 因子里混了两种量纲**，且分支由数据可得性而非平台语义决定 | 跨平台可比性（需求 §3.1 的核心命题）在 eng 维度上直接崩塌；两个分支的分数不可比，却进了同一个加权和 | 见 A-2：统一 eng_pct 恒为平台内分位（与有无曝光无关），eng_rate 仅作展示字段。`

`[S-6] 03-详细设计.md:147-152（Redis ZSET 分位）| 分位实现方案不可执行：① `ZCARD/ZRANK` 只能作用于**单个** ZSET，而窗口是滚动 7 日——跨 7 个 ZSET 无法求秩，文档未给出合并方案；② 分位定义未定（`(count<)/N` 还是 `(count< + 0.5·count=)/N`）；③ 冷启动（N=1）分位值未定；④ Redis 重启/过期后基线全丢，eng_pct 阶跃；⑤ 每点写入都要 ZADD，40 万点/日 × 7 日 = 280 万成员的 ZSET，内存与 ZRANK 成本未估 | 该方案在 M2 会被直接推翻返工，且 eng_pct 突变会引发全站 heat 跳变 → 伪突发潮 | 见 A-3：改用**分桶直方图**（HINCRBY，0.1 对数粒度，600 桶），O(1) 更新、可跨日窗口求和、可处理并列项、可离线兜底。`

`[S-7] 03-详细设计.md:168-171（固定 α=0.4 / β=0.5）| EWMA 用固定 α，但采样间隔可配 2–60 分钟（FR-TRK-01）。α 的物理含义是「每个采样点衰减多少」，间隔从 30 分钟改到 2 分钟会令平滑窗口从 1.5 小时缩到 6 分钟 → **同一话题换个采集频率，velocity 数值就不可比**，且突发检测的灵敏度随配置漂移 | 算法参数与配置项耦合，velocity 的量纲「分/小时」在 EWMA 层面被污染；跨话题、跨时期的 velocity 阈值（v_min=6.0）失去统一含义 | 见 A-6：改为时间感知 EWMA `α_n = 1 - exp(-Δt_h/T)`，`T = 0.75h`。`

`[S-8] 03-详细设计.md:168-171 + 195 + 264-269 | 端到端延迟：h_ewma 平均滞后 `(1-α)/α = 1.5` 点 = 45 min；v_ewma 再滞后 `(1-β)/β = 1` 点 = 30 min；rising 要求「持续 ≥2 采样点」再 +30 min → **突发发生到 rising 判定约 105 分钟（3.5 个采样周期）** | US-3「突发热点第一时间收到推送 / 抢黄金窗口」与 105 分钟延迟直接冲突；且需求 §9.2 已诚实声明「不承诺分钟级」，但 FR-TRK-05/08 的验收标准没有对应的延迟 SLO，产品预期无法对齐 | ① 需求 §5 增 NFR-TT-09「突发告警端到端延迟 P95 ≤ 2 个采样周期（≤60 分钟）」并写入 §7 验收；② 实现上用**未平滑 heat 的单点斜率**做 `early_flag`（仅看板提示 + 站内推送，不触发 Webhook/邮件），正式告警仍走 v_ewma；③ 详细设计 §2.3 补充延迟估算表。`

`[S-9] 03-详细设计.md:258-261 | zscore 三处统计缺陷：① σ 窗口 W=24 **包含当前点 t**，突发自身抬高 σ（掩蔽效应），且当突发持续 >1 点时 z 先升后降 → 出现「越突发越不告警」的反常；② 用 `|residual|` 双尾，暴跌也会被判为「突发」，与方法二/三的单尾语义不一致；③ `hit_zscore ⟺ |residual| > kσ ∧ σ > σ_min`，当序列极平稳（σ<1.0）时该方法**永久失效**，而冷启动与长尾话题恰是这种情况 | 检测率与误报率同时恶化，且无法通过调 k 修复 | 见 A-9：基线改用 `h_ewma(t-1)`，σ 用**不含当前点**的窗口 + MAD 稳健估计，改单尾，窗口下限 8 点。`

`[S-10] 03-详细设计.md:275-277 | rank_jump 两处不可用：① `rank(t-N) - rank(t) >= 10` 对**短榜不可达**（百度热搜 10 位，最大跃迁 9）→ 短榜平台该方法永久失效，只剩两个相关方法（B-5），永不告警；② `rank = -1` 哨兵参与减法运算（`-1 - 3 = -4`，或掉榜时 `45 - (-1) = 46` 反而虚高命中） | 平台间检测能力不对称 + 掉榜/进榜瞬间产生虚假跃迁 | 见 A-11：阈值改为 `max(3, ceil(0.2 × rank_total))`，rank 缺失时该方法记 `unavailable`（不投票、不计入分母），短榜退化为「首次进榜 ∨ 进入前 3」。`

`[S-11] 03-详细设计.md:259（W=24）| 冷启动：zscore 需要 24 点 = 12 小时历史。突发话题的黄金窗口恰恰在前 2–6 小时 → **最需要检测的时间窗内方法一不可用**；且全文无突发检测的最少点数门禁（`TT_INSUFFICIENT_POINTS` 只覆盖 lifecycle） | 新话题在黄金窗口内最多命中 velocity + rank_jump 两个方法，而非榜单平台（B 站/YouTube）只剩 velocity 一个 → **永不告警**，直接违反 FR-TRK-05 | 见 A-13 冷启动分级规则（n<3 不检测；3≤n<8 双方法且要求同 hit；n≥8 启用 zscore；首现跨平台规则）。`

`[S-12] 03-详细设计.md:26-29（tags: topic_id/platform/region/category）| InfluxDB tag 基数失控：`topic_id` 是无界增长的高基数 tag（活跃 8000，90 天内含 churn 按 3 倍估 ≈ 2.4 万话题 × 10 平台 × 2 region × 4 category ≈ **192 万 series**）。文档 §8 容量估算只算了 2–5GB 数据量，**完全没估算 series 基数**，而基数才是 InfluxDB 2.x 单机的真实瓶颈（内存与索引随基数线性增长） | M1 建库时不做基数设计，上线后 2–4 周会出现写入/查询性能断崖，届时改 tag 结构等于重写全部历史数据 | ① tags 只保留 `topic_id` + `platform`；`region`/`category` 移出 tag（从 PG 的 `Topic` 表 join）；② M1 增加验收项「实测 series 基数 < 50 万，且 90 天压测下写入 P99 < 200ms」；③ 超阈值时的备选方案写入 ADR。`

`[S-13] 03-详细设计.md:42（「InfluxDB 天然覆盖」保证 NFR-TT-07）| 断言过强：InfluxDB 的覆盖单位是 **(measurement + 完整 tag set + field key + timestamp)**。把 `region`/`category` 放进 tag 后，一旦话题的 category 被人工修正（或 region 由 cn 改 global），旧点留在新 series 里 → **同一 (topic_id, platform, t) 出现两条 series**，重复投递不再覆盖，NFR-TT-07 失效 | 幂等性被 tag 设计破坏，而幂等是 NFR-TT-07 的硬要求 | 与 S-12 同改：tag 只留 `topic_id` + `platform`；并在 §1.1 补一句「`category`/`region` 变更不需要重写时序点，查询时从 PG join」。`

`[S-14] 03-详细设计.md:46-52（Flux）+ 512（「range 窗口留 30 分钟重叠，保证补数据不重不漏」）| 断言错误：Flux 任务每次都执行 `range(start: -30m)`，**没有水位线**，任务一旦中断超过 30 分钟，`mm_5m` 就**永久缺失**这段数据，下个周期不会补；「不重不漏」无任何机制支撑 | 降采样数据静默丢失，且 NFR-TT-06「聚合曲线 ≥1 年」不可信；`tt_downsample_fail_total` 只告警不修复 | 见 A-17：Redis/PG 存 `watermark`，任务执行 `range(start: watermark-30m, stop: now-5m)`，完成后推进 watermark；配合 (series, field, ts) 覆盖语义保证幂等；新增每 1h 的 backfill 任务覆盖迟到点。`

`[S-15] 03-详细设计.md:50（`aggregateWindow(every: 5m, fn: mean)`）| 统一用 `mean` 对 3 个字段语义错误：① `rank` 含 `-1` 哨兵，掉榜点参与均值 → 均值被污染（真实 #3 与掉榜 -1 平均得 1）；② `is_interpolated`(bool) 求均值无意义；③ `rank_total` 求均值无意义。且 **30 分钟采样降采样到 5 分钟毫无压缩效果**（createEmpty:false 下窗口内最多 1 个点） | 聚合层数据语义错误 + 存储浪费，且查询时拿到的是错误口径的 rank 曲线 | 见 A-17：逐字段指定聚合函数（rank→min 且过滤 ≤0；views/likes/comments→last；is_interpolated→interp_ratio），聚合层改为 `mm_1h`(1y) + `mm_1d`(3y)，并按查询跨度自动路由 bucket。`

`[S-16] 03-详细设计.md:130（`WeightConfig.effective_from`）| 权重变更只处理了「历史 heat 不重算」，但**没处理派生指标跨断点跳变**：权重一改，全部话题的 heat 阶跃 → `velocity = Δh_ewma/Δt` 出现全站性尖峰 → 全站伪突发 → 直接触发告警风暴（叠加 B-10 无抑制机制） | 一次正常的运营调参动作引发全站误告警，是最高概率的线上事故路径 | 见 A-15：权重变更时写 `weight_epoch` 到 InfluxDB 与 Topic 表；生效后第一个采样点强制 `α=1`（重置 h_ewma/v_ewma）；看板按 epoch 分段渲染并标注「口径变更于 …」；变更窗口内全局抑制告警 1 个周期。`

`[S-17] 03-详细设计.md:64（`platform_count`）/ 158（breadth 用它）| `platform_count` 的统计窗口未定义。若按「话题历史上出现过的平台」累计，则 breadth **单调递增、永不衰减** → 一个话题只要曾经登陆过 8 个平台，即使早已凉透，breadth 恒为 1.0，与 `decaying` 阶段语义直接冲突 | 衰退话题的 heat 被 breadth 硬性托底，decaying → long_tail 的判据（heat<20）可能永远不成立，话题卡死在 decaying | `platform_count(t) = |distinct platform where TopicSource.contributed_at ∈ (t-24h, t]|`，随样本时间滑动；PG 侧冗余列同步更新；`heat.breadth.platforms` 输出该窗口内的平台列表（契约必填，见 C-6）。`

`[S-18] 03-详细设计.md:321（「同一 topic 24 小时内只产生一个候选」）/ 94-107（候选表无 route 去重键）| 两处逻辑缺陷：① 去重键只有 `topic_id`，不含 `route`——话题从 `mme` 升级为 `both` 时不会生成新候选，MIA 侧永远收不到；② 话题持续 rising 5 天只产生 1 个候选，一旦被 `reject`，运营**再也看不到它**，选题台对长期热点永久失明 | FR-TRK-09 的核心价值（排序 + 节省筛选时间）对长热话题归零；且 `rejected` 后无复活路径，与「稍后再看 snooze」的产品设计矛盾 | 去重键改为 `(topic_id, route)`；`reject` 后去重窗口改为 `reject_cooldown_hours = 72`（且 `reject_reason` 非空才生效）；`snooze` 写入 `snooze_until`，到期后允许生成新候选；同 topic 的候选版本号 `candidate_seq` 递增，UI 显示「第 N 次推荐」。`

`[S-19] 03-详细设计.md:321（「每 15 分钟扫描 stage ∈ {rising, peak}」）| 只在 rising/peak 生成候选，导致 `B_material`（可直接加工的长视频/文章/播客）类长青内容**永不进选题台**——而这类内容热度天然低、生命周期长，恰好是最适合 MME 加工的素材，也是 FR-TRK-09 四维中「可加工性」权重（0.20）存在的理由 | MME 的输入源被系统性截断，MME 需求 §29「输入 = ② TT 选题送入」在 B_material 场景下不可达 | 候选扫描条件改为 `stage ∈ {rising, peak}` **∨** `category = B_material ∧ workability ≥ 0.6 ∧ compliance ≥ 0.5`；并在 `TopicCandidate` 增列 `trigger_reason`（`stage_hot` / `material_ready`），避免两类候选混淆排序。`

`[S-20] 03-详细设计.md:291-317（选题评分与路由）/ 04-开发进度.md:99（FR-TRK-06 在 M4）| `suggested_angles` 是契约必填、MME 概要 §351 明确「`suggested_angles` 作为内容矩阵的输入」，但三份文档**没有任何生成逻辑**，M4/M5 任务清单里也没有对应条目；`material_refs` 的来源与过滤规则（契约 §6 要求「若 Q1 拍板不做切片搬运，只保留自有/授权/CC 素材」）同样缺失 | MME 的矩阵化输入为空，整条 ④ MME 链路在 M5 之后仍然断链；契约门禁会因 `suggested_angles` 语义不明而无法校验 | 见 A-18（模板生成 + LLM 改写 + 合规复检 + 兜底空数组），并补进 04 §4.5 任务清单（+2 人日）。`

`[S-21] 03-详细设计.md:169-171（accel）/ 01-需求规格.md:59（「accel 判突发」）| `accel` 被定义、计算、写入 InfluxDB、写入契约 `heat.accel` 与 `stage_evidence.accel`、进了单元测试清单和黄金数据集，但**三方法投票与五阶段判据都没有任何一条规则使用 accel** | 需求声称「accel 判突发」与实现不符；一个无消费方的指标占了存储、带宽与测试成本 | 二选一：① 删除 accel（从契约、InfluxDB field、指标、测试全部移除，需走契约演进流程）；② 真正用起来——把方法二改为 `velocity > v_min ∧ accel > a_min`（`a_min = 0`）以区分「持续上涨」与「脉冲噪声」，这同时也是修复 B-5 相关性的一个抓手。推荐 ②。`

`[S-22] 04-开发进度.md:169（砍单第 5 条「FR-TRK-03 聚类降为直接复用 IC cluster_id」）| 与 ADR-TT-02（02:231「话题聚类独立判定，不直接复用 IC cluster_id」）正面冲突；且砍单后 `topic_id` 退化为 IC 的 `cluster_id`，按契约 §0.3 其前缀应为 **`cl_` 而非 `tp_`** → 契约 §2 的 `topic_id: "tp_01H..."` 与 ADR 同时违约 | 砍单是延期时的执行预案，但当前写法会同时违反 ADR 与契约 ID 规范，属于「砍单砍出违约」 | 砍单第 5 条改为「FR-TRK-03 降级为『仅 Step1 硬键归并』（即 A-1 方案去掉 Step2–4），`topic_id` 仍由 TT 生成（`tp_` 前缀），IC `cluster_id` 仅作 `TopicSource.cluster_key` 入参」。ADR-TT-02 补一句降级路径说明。`

`[S-23] 03-详细设计.md:全文 / 01-需求规格.md:95 | 迟到数据（late arrival）零处理：`metrics._as_of` 可能落后数小时（IC 回放、sidecar 补采、JetStream 积压）。写入一个旧时间戳后，已算好的 sequence 中间被插入一个点 → EWMA/velocity/accel 全部需要重算，但文档没有任何回填触发器、重算范围、幂等保证 | 曲线出现「历史被静默改写」或「新旧口径混杂」；且回填本身会二次触发突发检测，产生重复告警 | 见 A-14 第 3 条：`_as_of` 落在 `[t_now - 24h, t_now + 5min]` 之外时打 `late=true`，写入后投递 `tt.recompute` asynq 任务，重算区间 `[t_late, t_now]` 内所有点的派生指标并覆盖写（幂等）；重算期间对该 topic 抑制告警（`recompute_lock`）。`_as_of` 落后 > 24h 直接丢弃并计数 `tt_ingest_too_old_total`。`

`[S-24] 03-详细设计.md:127（02 概要：scoring 每 15 分钟）/ 448（compute_interval: 5m）/ 02-概要设计.md:249（「归一化/动态：每采样点触发」）| 计算触发时机三种说法互斥：① 每采样点触发；② 每 5 分钟；③ 每 15 分钟（scoring）。采样周期是 30 分钟，按 ② 每 5 分钟跑一次意味着 6 次里 5 次空转 | 空转消耗 CPU 且让「velocity 持续 2 点」的语义不确定（是按采样点还是按计算轮次） | 统一为：**计算由快照点驱动（event-driven），不由定时器驱动**。`compute_interval` 配置项删除；scoring 保留 15 分钟定时 + 「stage 转移」事件触发双通道，二者用同一把分布式锁（见 M-8）。`

`[S-25] 03-详细设计.md:26-40 / 02-概要设计.md:43-46 | 不同平台的采集时刻天然不齐（微博 10:05、B 站 10:22），同一「快照」的 `t` 不同 → ① 跨平台曲线叠加时 x 轴错位（需求 US-7「可对比多平台」）；② EWMA 的 `Δt_hours` 在跨平台聚合时无定义；③ `(topic_id, platform, t)` 的 t 不同导致「同一时刻的跨平台快照」无法作为一行写入 | 多平台叠加曲线与跨平台 heat 聚合都不可实现 | 见 A-14 时间桶对齐：`t_bucket = floor(t / 30min) × 30min`；跨平台聚合以 `t_bucket` 为键。`

---

## 三、一般问题

`[M-1] 01-需求规格.md:77 vs 03-详细设计.md:255-262 | 需求写「对 heat_score 时序做 **EWMA/STL 分解**，残差 > kσ」，详细设计只保留了 EWMA 残差，STL 被静默删除且无任何说明 | 需求与实现的算法基线不一致，评审与验收无据 | 在 03 §2.3 明确写「V1 采用 EWMA 残差，STL 分解推迟到 V2（需 ≥14 日历史以估计日内季节性）」，并在需求 §3.3 同步标注。`

`[M-2] 03-详细设计.md:169-171 定义了 `velocity(t)` 与 `v_ewma(t)` 两个量，但全文三处消费方都只写「velocity」 | ① `heat.velocity` 是 velocity 还是 v_ewma？② `stage_evidence.velocity` 是哪个？③ 方法二 `velocity(t) > v_min_burst` 用哪个？三者未指定，且两者数值可差一倍 | `velocity` 统一指 `v_ewma(t)`；原始差分命名 `velocity_raw(t)`；契约 `heat.velocity` 与 `stage_evidence.velocity` 均输出 `v_ewma`，`velocity_raw` 只作调试字段不进契约。`

`[M-3] 03-详细设计.md:195（`rising` 判据只有 `velocity > v_min ∧ 持续 ≥2 点`）vs 232（状态图 decaying→rising 写「velocity>阈值 **且 heat>30**」）| 同一个 rising 转移，判据表与状态图条件不一致（差一个 heat>30） | 二次爆发路径的实现会与判据表分叉 | 统一：rising 判据不含 heat 条件（保持 03:195）；状态图 232 行改为「v_ewma > v_min ∧ 持续≥2 点」，删除「且 heat>30」。`

`[M-4] 03-详细设计.md:190（`阈值可配，存 sys_config 或 WeightConfig 同级配置表`）| 仓库实际表名是 `system_config`（Ent `SystemConfig`，见 `apps/server/app/admin/internal/data/ent/schema/system_config.go`），且不说清放哪张表等于没说 | 配置项落点不确定，M2 会随意实现 | 明确为「生命周期与突发阈值存入 `system_config`，key 前缀 `tt.lifecycle.*` / `tt.burst.*`；权重存入 `WeightConfig`。运行时经 `cache.go` 缓存，TTL 60s，`PATCH /tt/weights` 与配置变更时主动失效」。`

`[M-5] 03-详细设计.md:28（`region` tag: cn / intl）vs 63（Ent `Topic.region`: cn / intl / global）| 同一语义字段两处取值集合不一致 | 写入 InfluxDB 时 `global` 无处安放，会被静默丢弃或报错 | 统一为 `cn | intl | global`；但按 S-12 建议 region 不出 tag，仅 PG 保留三值。`

`[M-6] 契约 00-shared/01-data-contract.md:109 与 04-开发进度.md:59 | 契约写 Go 结构体在 `pkg/mediacontract/intel.go`，但仓库实际包根目录是 `apps/server/pkg/`（无顶层 `pkg/`） | 路径不确定导致 M1 落库位置分歧 | 统一为 `apps/server/pkg/mediacontract/`；契约 §1、§10 与 04 §4.1 三处同步更正。`

`[M-7] 03-详细设计.md:502-509（错误码表）| ① 无 HTTP / gRPC 状态映射（Kratos `errors.New(code, reason, message)` 的 Reason 未定义）；② `TT_INSUFFICIENT_POINTS` 要求「返回 latent + 标记 `insufficient=true`」，但契约 `TrendSeries` **没有 `insufficient` 字段**（见 C-12） | 前端无法区分「真的 latent」与「数据不足的 latent」 | 补齐错误码 → `Reason` → HTTP 映射表；`insufficient` 走契约演进（加可选字段 `diagnostics{insufficient_points: bool, n_points: int}`）。`

`[M-8] 03-详细设计.md:127（15 分钟 scoring）/ 94（asynq 周期任务）| 无任何并发互斥：scoring 周期任务与上一次未跑完时重叠 → 重复生成候选；同一 topic 的 compute 被快照事件与定时任务同时触发 → 双写 | 候选重复、velocity 双算 | `worker/tt/schedule.go` 用 asynq 的 `Unique(ttl)` 做任务级去重；topic 级计算用 Redis `SET tt:compute:lock:{topic_id} NX PX 30000`，抢不到锁则跳过（不排队，避免积压）。`

`[M-9] 03-详细设计.md:319（「支持置顶（+∞ 排序权重）与降权（×0.5）」）| 无任何存储字段承载置顶/降权状态（`TopicCandidate` 表没有 `pinned` / `weight_override` / `sort_bias`） | 功能无法实现；且「操作记录审计」无表 | `TopicCandidate` 增列 `is_pinned bool`、`sort_bias float`（默认 1.0，降权置 0.5）、`override_by`、`override_at`；`ListCandidates` 排序键改为 `order_key = is_pinned DESC, score_total × sort_bias DESC`。`

`[M-10] 03-详细设计.md:82-92（`TrendSubscription`）| ① 契约 §0.3 ID 前缀表**未登记订阅实体的前缀**（现有 `ts_` 未分配）；② 缺 `last_run_at` / `notify_channels` / `mute_until` / `snooze_until`；③ §4.2「向 IC 发回采请求」的协议**未在契约 §8 NATS 对照表中登记**（TT 新增了对 IC 的反向请求，违反 02 §10「TT 不反向要求 IC 增加仅自己需要的字段」的精神） | 订阅功能无法落库；回采请求属于跨系统新接口却绕过契约评审 | ① 在契约 §0.3 登记 `ts_ = TrendSubscription`；② 补齐字段；③ 回采请求要么走契约演进新增 `mm.intel.backfill.req.v1`（需评审），要么降级为「订阅仅支持已存在话题」（即砍单顺序第 2 条），V1 **不做主动回采**。`

`[M-11] 03-详细设计.md:109-120（`Alert` 表）| 缺 `ack_at` / `ack_by` / `mute_until` / `dedupe_key` / `hit_count` / `channels_status`（多渠道发送状态）；且无 `BurstEvent` 审计表，导致「误报率」**没有分母**（`tt_burst_false_positive_total` 只统计被标记的，`tt_burst_total` 只统计命中的，总检测次数不可知） | 需求 §7 验收项 5 与 03 §2.3「按 topic/平台统计误报率」无法计算 | 见 A-13 的 `BurstEvent` 表定义与 Alert 补齐字段。`

`[M-12] 03-详细设计.md:96（FR-TRK-08 多渠道推送）| 推送部分失败（Webhook 成功、企业微信失败）无处理：无 outbox 表、无发送状态、无重试与死信 | 告警静默丢失，而 Alert 契约的 `channels` 字段声称已发 | 新增 `AlertDelivery{ id, alert_id, channel, state(pending|sent|failed), attempts, last_error, sent_at }`，由 asynq 驱动（max_retry=3，失败入死信 + 运营告警）；`Alert.channels` 只表示**计划**渠道，实际结果查 `AlertDelivery`。`

`[M-13] 02-概要设计.md:262（「插值需显式标注为插值点」）/ 03-详细设计.md:39（`is_interpolated` field）| 插值算法全文未定义：何时插（缺口 >几个周期？）、怎么插（线性？前值填充？）、插到哪一层（raw 还是查询时？）、真实点迟到后能否覆盖插值点 | 补点会污染 velocity（线性插值出的点斜率为常数，制造假匀速），且 `is_interpolated` 的语义在降采样后丢失 | 明确：**只在查询层插值，不写入 InfluxDB**（避免覆盖语义冲突与降采样污染）；缺口 ≤2 个周期用线性插值，>2 个周期不插（返回断口）。`is_interpolated` field 保留但仅用于「上游明确声明的补采点」。`

`[M-14] 03-详细设计.md:547（黄金数据集 5 条曲线）/ 01-需求规格.md:139-140 | ① 5 条样本无法支撑任何统计显著性；② 「`heat_score` 排序与人工直觉一致率 ≥80%」中「人工直觉」不可判定（无标注协议、无评分细则、无标注人数量与一致性要求） | 验收项 2/4 不可执行，验收现场必然争议 | ① 黄金数据集扩到 ≥30 条（覆盖 5 阶段 × 3 平台类型 × 含二次爆发/掉榜/跨语言归并）；② 验收项 2 改为「3 名运营独立对 30 个话题两两比较 heat 排序，与系统排序的 Kendall τ ≥ 0.6，且标注人间一致性 Krippendorff α ≥ 0.6」。`

`[M-15] 01-需求规格.md:113（NFR-TT-03「展示 Asia/Shanghai」）/ 03-详细设计.md:492 | 展示时区硬编码为单个值，但① 用户可能跨时区；② `region=intl` 的话题用上海时区展示无意义；③ 跨市场财经对齐的真实需求是「按**市场本地时区**对齐交易时段」，而非统一上海时区 | NFR-TT-03 的立论（「财经跨市场对齐的硬要求」）与其实现（全局单一展示时区）不匹配 | 展示时区改为「用户偏好时区（默认 Asia/Shanghai）+ 可选市场本地时区」；存储侧仍强制 UTC 带 Z（不变）。`

`[M-16] 03-详细设计.md:451（`token: "${INFLUX_TOKEN}"`）| InfluxDB 2.9.0 起**默认启用 API token 哈希存储**（升级后明文 token 不可恢复）；文档无 token 轮换、无最小权限、无「写入 token / 只读 token」分离 | 运维升级 2.9.x 后若未预先保存明文 token，服务将永久失去写入能力 | ① 建库时分别创建 `tt-writer`（仅写 `mm_raw`）与 `tt-reader`（仅读 `mm_raw`/`mm_5m`）两个 token；② 文档注明「InfluxDB ≥2.9.0 token 落盘即哈希，明文须在升级前备份」；③ 轮换窗口：双 token 并存 24h。`

`[M-17] 03-详细设计.md:72（`Topic.is_archived`）| 无任何任务写入 `is_archived`，也无归档逻辑（04 §4.6 把 FR-TRK-12 放 Phase 3，但归档是数据生命周期的必需品，不是可选功能） | 90 天后话题元数据无限膨胀，看板/候选扫描的 `is_archived=false` 过滤永远扫全表 | M5 增一个 asynq 归档任务：`last_seen_at < now - 30d ∨ stage = dormant ∧ age > 7d → is_archived = true`，每日 03:00 跑；`Topic` 的 `idx(stage, updated_at)` 改为 `idx(is_archived, stage, updated_at)`。`

`[M-18] 契约 01-data-contract.md:156 / 01-需求规格.md:130 | 契约规定「`series` 在 NATS 消息中不得超过 200 点」，但 TT 从未定义**这 200 点怎么截**（最近 200 点？按平台均分？降采样到 200 点？） | 30 分钟采样下 200 点 = 100 小时 ≈ 4.2 天，但多平台时若按「最近 200 点」截，可能全被单一平台占满，跨平台对比失效 | 见 A-19：按平台均分 + 全局降采样两段式窗口。`

`[M-19] 03-详细设计.md:71（`representative_intel_id`）/ 61（`label`）| 主条目与话题标签的选取规则均未定义（第一条？热度最高？合规最优？） | 话题标签会随机漂移，UI 出现「话题名反复变化」 | 选取规则：`gate=pass` 优先 → `workability.score` 降序 → `metrics` 互动数降序 → `collected_at` 升序；每 6h 或 heat 变动 >10 重选，变更写入 `TopicLabelHistory`。`

`[M-20] 03-详细设计.md:546（单元测试清单）| 清单覆盖了边界值，但**遗漏了本次评审发现的高风险路径**：迟到/乱序写入、Redis 分位冷启动与 Redis 重启、权重变更断点、跨阶段死锁（B-6 的无匹配状态）、三方法相关性回归、InfluxDB 写入部分失败 | 85% 覆盖率门槛达成但核心缺陷未覆盖，测试策略与风险不匹配 | 在 §7 增补上述 6 类用例，并把「三方法独立性检验」（计算 velocity 与 residual 的样本相关系数，断言 |ρ| < 0.9）加入 CI，作为 B-5 修复的回归门禁。`

`[M-21] 契约 00-shared/01-data-contract.md:142 与 297 与 76 | 契约正文 §0.2 规定「存储一律 UTC（RFC3339 带 Z）」，但 §1 的 `collected_at`、§2 的 `series[].t`、§7 的 `triggered_at` 三处示例全部写成 `+08:00` 偏移 | 契约自相矛盾，TT 的 JSON Schema 该按哪个写不确定；若照示例实现则违反 §0.2 与 NFR-TT-03 | 契约三处示例统一改为 `...Z`；TT 侧在 M1 的契约测试里加一条断言「所有时间字段正则匹配 `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`」。`

`[M-22] 03-MIA/01-需求规格.md:81 | MIA 声明输入包含 `TrendSeries`，但契约 §8 的 NATS 对照表中**没有任何承载 TrendSeries 的 subject**（`mm.trend.*` 三个 subject 分别对应快照内部结构、Alert、TopicCandidate） | MIA 的 TrendSeries 依赖无传输通道；TT 侧也没有对外提供 TrendSeries 的接口定义 | 二选一：① MIA 改为通过 REST `GET /api/v1/media/tt/topics/{id}` 拉取（TT 需在 §6.2 明确该接口返回 `TrendSeries` 完整契约）；② 契约 §8 新增 `mm.trend.series.v1` subject。**必须在 M1 契约冻结前定案**，否则 MIA 侧无法开发。`

---

## 四、契约违规清单

`[C-1] 契约 §0.4「`TrendSeries` → `media.v1.tt.TrendSeries`」| 03-详细设计.md §3 未定义 `TrendSeries` message，只有 `Topic` | 补 B-2 给出的完整 `TrendSeries` / `Heat` / `Breadth` / `SeriesPoint` / `Burst` / `Origin` 定义。`

`[C-2] 契约 §2 `topic_id` `label` `category` `stage` | 03-详细设计.md:368-380 的 `Topic` message 用 `id = 1`，且缺 `api_version`、`series`、`burst`、`origin`、`heat` 包装层 | 契约字段名是 `topic_id`，不是 `id`（`id` 属于 Ent 内部视图，不得进契约，见契约 §0.4）；补 `api_version` 与三个缺失块。`

`[C-3] 契约 §6 `candidate_id` / `reject_reason` | 03-详细设计.md:396-406 的 `Candidate` message 用 `id = 1`，无 `api_version`、无 `reject_reason` | 改为 `candidate_id`；补 `api_version` 与 `reject_reason`（可为 null）。`

`[C-4] 契约 §2 `heat.factors = {w_rank, w_eng, w_fresh, w_breadth}`（只有 4 个权重）| 03-详细设计.md:356-366 的 `HeatFactors` 塞进了 `rank_pct / eng_pct / freshness / breadth / missing` 共 9 个字段 | 二者必须对齐。因 NFR-TT-05 要求因子取值可追溯，正确方向是**改契约**：`heat` 补 `eng_pct`、`freshness` 两个字段（见 C-5），`factors` 补 `w_*_eff`（归一化后权重，用于审计）。走契约 §9 的次版本演进流程。`

`[C-5] 契约 §2 `heat` 块只有 `rank_pct` / `eng_rate` / `velocity` / `accel` / `breadth` / `factors` | 缺 `eng_pct` 与 `freshness`，而 heat 公式实际用的是 `eng_pct` 与 `freshness` | 契约无法承载四因子取值 → NFR-TT-05 在跨系统传输层面必然违约。契约 `heat` 增加 `double eng_pct = 9; double freshness = 10;`。`

`[C-6] 契约 §2 `heat.breadth = {platform_count, platforms}` | TT 全系统只有 `platform_count`（Ent `Topic`），**没有任何地方存储平台列表** | `platforms` 是必填数组，缺失即 Schema 校验失败。`Topic` 增列 `platforms jsonb`（滑动 24h 窗口内的 distinct 平台列表，见 S-17）。`

`[C-7] 契约 §0.2「可选字段允许 `null`；`0` 与 `null` 语义不同（`rank_total=0` 表示未知，不表示榜长为 0）」| 03-详细设计.md:30-36 用 **`-1` 哨兵**表示「无榜 / 无曝光」（`rank=-1`、`rank_total=-1`、`eng_rate=-1`） | 哨兵值与契约空值约定冲突，且哨兵参与算术会出错（见 S-10）。InfluxDB field 内部可保留 -1（便于数值聚合），但**契约输出与 Proto 必须转为 `null`**；`normalize` 的 `Missing` 判定统一走「字段为 null 或 ≤0」。`

`[C-8] 契约 §7 `Alert` | 03-详细设计.md §3 未定义 `Alert` message；Ent 用 `id`（`al_` + ULID） | 补 `Alert` message，字段名用 `alert_id`；`evidence` 用 `map<string,double>`；`channels` 用 `repeated string`。Ent 内部保留 `id`（契约 §0.4 允许内部视图有额外字段）。`

`[C-9] 契约 §8「`mm.trend.snapshot.v1`：生产者 TT 采集 → 消费者 TT 时序写入」| 02-概要设计.md:118-119 的顺序是「写 InfluxDB → 发 `mm.trend.snapshot.v1`」，即 snapshot 在写入**之后**发布，消费者未定义 | 与契约的生产者/消费者定义相反。明确：`mm.trend.snapshot.v1` 由 `ingest_usecase` 产出、由 `timeseries_writer` 消费（契约语义），InfluxDB 写入只发生在消费端；03 §4.1 的主链路图同步更正（当前图把「写 InfluxDB」画在 ingest 内部）。`

`[C-10] 契约 §0.2「枚举一律小写下划线字符串（`rising`、`bullish`），禁止用整型魔法数字跨系统传」| 03-详细设计.md:337-353 用 `enum Stage { STAGE_RISING = 2; }`，grpc-gateway 默认序列化产出 `"STAGE_RISING"` 而非 `"rising"` | 契约消息违反枚举约定；下游 MME/MIA 按 `"rising"` 解析会失败 | 契约层 message（`TrendSeries` / `TopicCandidate` / `Alert`）的 `stage` / `route` / `state` 一律声明为 `string`，Go 侧用 `switch` + 校验；或保留 proto enum 但配置 JSON 序列化为小写名称，并在 M1 契约测试中断言产出为 `"rising"`。`sys_config` 与 Ent 内部仍可用整型。`

`[C-11] 契约 §0.2「每份契约顶层必须带 `api_version`，当前统一 `"1.0"`」| 03-详细设计.md:368-380 / 396-406 的 `Topic` 与 `Candidate`，以及 `TrendSeries`、`Alert`（未定义）**全部没有 `api_version`** | 契约门禁（生产者契约测试）直接失败 | 三份 message 顶层补 `string api_version = 1;`，恒为 `"1.0"`；M1 往返测试断言该字段。`

`[C-12] 契约 §2 字段集 | 03-详细设计.md:507 `TT_INSUFFICIENT_POINTS` 要求「标记 `insufficient=true`」，契约无此字段 | 错误语义无法跨系统传递 | 契约 `TrendSeries` 增加可选 `Diagnostics diagnostics`，含 `insufficient_points bool` 与 `n_points int32`（补丁级演进，`api_version` 不变）。`

`[C-13] 契约 §2 / §7 示例时间戳 `+08:00` | 与契约 §0.2「UTC 带 Z」冲突 | 见 M-21。`

`[C-14] 契约 §7「`state=muted` 人工静音后不再重复推送**同一 topic**」| 02-概要设计.md:208 `POST /api/v1/media/tt/alerts/{id}/mute`（按**单条告警**静音） | 粒度不符，契约语义无法实现 | 见 B-10 / A-13：新增 `TopicMute` 实体与 `POST /tt/topics/{id}/mute`。`

`[C-15] 契约 §0.4「`TrendSeries` → 元数据 `Topic` + 时序 `topic_heat`」| Ent `Topic` 表缺 `stage_evidence`（velocity/accel/duration_points）、缺四因子取值、缺 `platforms`；InfluxDB `topic_heat` 缺四因子 field | 契约 §2 要求的 `stage_evidence` 与 `heat.factors` 在持久化侧无落点 | 见 B-3；`stage_evidence` 增列 `stage_duration_points int`（当前阶段已持续采样点数，按 B-7 口径计算）。`

---

## 五、算法歧义清单

`[A-1] 话题聚类与归并`
- 当前描述（02:117 / 04:82）：「按 cluster_id + 语义相似度 → topic_id」「以 IC cluster_id 为强特征 + 语义相似度」
- 歧义点：相似度函数、召回方式、阈值、时间窗、跨语言归并、人工锁定、label 选取、复杂度，全部未定义；「强特征」无量化含义
- 建议定义：见 B-1 的六步方案（硬键 → 倒排+SimHash 召回 → 三路加权打分 → 三段阈值 → merge_lock → label 重选），默认值 `θ_merge=0.82`、`θ_review=0.70`、`T_window=7d`、召回上限 200、embedding 用多语言模型且跨语言要求 `cos≥0.72`

`[A-2] eng_rate / eng_pct`
- 当前描述（01:56）：`互动数 / 曝光数`；无曝光退化为「互动量对数分位」。（03:146-149）：`eng_rate = (likes+comments)/max(views,1)`；`eng_pct = percentile_rank(log1p(likes+comments), within=(platform,category), window=rolling 7d)`
- 歧义点：① 需求「互动数」是否含分享/收藏；② heat 公式用 `eng_pct`，但「有曝光时」只定义了 `eng_rate`，`eng_pct` 在有曝光时无定义；③ 微博无 `views` → 恒走分位分支，B 站走比率分支，两分支量纲不同却进同一加权和
- 建议定义：

```
eng_rate（仅展示，不进 heat 公式）
  = (likes + comments + shares + favorites) / max(impressions, 1)   有曝光
  = null                                                            无曝光（契约用 null，内部 -1）
eng_pct（恒为平台内分位，与有无曝光无关 —— 这是跨平台可比的前提）
  raw     = log1p(likes + comments + shares + favorites)
  eng_pct = ECDF_{platform, category}(raw; 窗口 (t-7d, t])
          = (count(x < raw) + 0.5 × count(x == raw)) / max(N, 1)
  ⚠ 契约 §1 metrics 当前无 shares/favorites 字段：V1 用 (likes+comments)，
    并在契约 §1 登记 shares/favorites 为 V1.1 新增可选字段
```

`[A-3] 分位计算（ECDF）`
- 当前描述（03:152）：「滚动 7 日窗口内的经验分位，用 Redis 有序集合维护（score=log1p(互动数)），`ZCARD`/`ZRANK` 求分位。避免全量扫描」
- 歧义点：① ZRANK 只能作用于单 ZSET，7 日滚动需跨 7 个 ZSET，无法求秩；② 并列项处理未定；③ 冷启动 N 小时分位值未定；④ Redis 重启后基线丢失；⑤ 内存与时间复杂度未估
- 建议定义：

```
存储：分桶直方图（不是 ZSET）
  key    = tt:eng:{platform}:{category}:{yyyymmdd}      TTL = 8d
  bucket = clamp(floor(log1p(v) × 10), 0, 599)           # 0.1 对数粒度，覆盖 v ≈ 0..1.8e21
  写入：HINCRBY key bucket 1
查询：读 (t-7d, t] 覆盖的 7 个 key，HMGET 全部 600 桶后累加
  N      = Σ c_b
  eng_pct= (Σ_{b'<b} c_b' + 0.5·c_b) / max(N, 1)         # 并列项取中点，无偏
  优化：每个 key 额外维护 prefix[b] 前缀和（写入时 O(600) 更新），查询降为 O(7)
冷启动：N < 200 → 退化到离线静态基线表 platform_eng_baseline[platform]（随代码发布，季度更新）
Redis 不可用：同样退化到静态基线表，并在 HeatResult.Missing 中不标记 eng（避免触发权重重分配导致口径跳变）
```

`[A-4] freshness`
- 当前描述（01:57 / 03:157）：`exp(-Δt_hours / τ)`，`τ = 12h`，`Δt = now - first_seen_at`
- 歧义点：① 用 `now` 而非样本时间 → 历史不可重现（B-4）；② 以 `first_seen_at` 为基准，二次爆发（decaying→rising）的话题 freshness≈0，与状态机允许二次爆发矛盾；③ 无地板，导致 24h 后 peak 不可达（S-3）；④ 衰减函数选择（指数 vs 线性 vs 半衰期）未论证
- 建议定义：

```
主口径（进 heat 公式，可重现）：
  freshness(t) = max( exp( -(t - first_seen_at) / τ ), f_floor )
  τ = 12h，f_floor = 0.05，t 为样本时间戳（禁止 now()）
二次爆发识别（不进 heat 公式，仅作特征）：
  last_active_freshness(t) = exp( -(t - last_seen_at) / τ )
  当 last_active_freshness > 0.5 且 velocity > v_min 时，允许 decaying → rising（与状态机图一致）
半衰期口径（文档表述统一用这个，避免「τ」被误读）：
  t_1/2 = τ · ln2 = 8.32h，即「话题诞生 8.3 小时后新鲜度因子减半」
  ⚠ 文档应同时给出半衰期，便于运营理解；配置键保持 freshness_tau_hours = 12
```

`[A-5] breadth`
- 当前描述（03:158）：`min(1, log1p(platform_count) / log1p(ref_platform_count))`，`ref = 8`
- 歧义点：① 单平台即得 0.3155，语义错误；② IC 覆盖平台数 <8 时因子上限恒 <1（隐性降权）；③ `platform_count` 无统计窗口（S-17）；④ 未考虑平台体量差异（8 个小号 vs 3 个大号）
- 建议定义：

```
P     = len(supported_platforms)                     # IC 实际支持并已接入的平台数
ref   = min(8, P)                                    # 关键：不得超过实际覆盖数
pc(t) = |distinct platform where TopicSource.contributed_at ∈ (t-24h, t]|
raw   = log1p(pc) / log1p(ref)
b0    = log1p(1) / log1p(ref)                        # 单平台基线，ref=8 时为 0.3155
breadth = clamp( (raw - b0) / (1 - b0), 0, 1 )       # 单平台 → 0；pc ≥ ref → 1
平台体量差异（V1 不做，记录为已知局限）：不做加权，因为会引入难以标定的平台权重，
  跨平台可比性改由「排序时用 heat 的 24h 分位而非 heat 绝对值」来兜底（见 A-6 注）
```

`[A-6] heat 合成与权重重分配`
- 当前描述（01:57 / 03:160-162）：`heat = 100 × (w_rank·rank_pct + w_eng·eng_pct + w_fresh·freshness + w_breadth·breadth)`，默认 `{0.35, 0.25, 0.20, 0.20}`，「某因子缺失时，该权重按比例分配给其余因子（分母取有效权重之和）」
- 歧义点：① 重分配无显式公式；② 全因子缺失时 `Σw = 0` → 除零；③ `HeatFactors.w_rank` 等四个字段是**原始权重**还是**归一化后权重**未定（审计时两者都要）；④ 「因子有效」的判定条件未统一（`rank<0`？`eng_pct` 为 null？`freshness` 恒有值？`breadth` 恒有值？）
- 建议定义：

```
有效判定：f_i ≠ null（内部哨兵 ≤0 视同 null）
V = { i | f_i 有效 }
S = Σ_{i∈V} w_i
若 S == 0：heat = null，stage 强制 latent，missing = ["rank","eng","fresh","breadth"]，
           发 TT_HEAT_NO_FACTOR，不写 InfluxDB（避免写 null 污染曲线）
否则：
  w_i' = w_i / S                       (i ∈ V)
  heat = 100 × Σ_{i∈V} w_i' · f_i
输出结构（同时保留两套权重，供审计）：
  HeatFactors{ rank_pct, eng_pct, freshness, breadth,
               w_rank, w_eng, w_fresh, w_breadth,        // 原始配置权重
               w_rank_eff, w_eng_eff, w_fresh_eff, w_breadth_eff,  // 本次实际生效权重
               missing[] }
  ⚠ 恒等校验（CI 断言）：Σ_{i∈V} w_i_eff == 1.0（浮点容差 1e-9）
注（跨平台排序）：跨平台「谁更热」应比较 heat 在近 24h 全体活跃话题中的分位，
  而非 heat 绝对值；heat 绝对值仅用于同平台内排序。此规则须写入 03 §2.1 与前端文案。
```

`[A-7] rank_pct` ★ 两份文档公式不一致
- 当前描述：（01:55）`(rank_total - rank) / rank_total`；（03:142）`clamp((rank_total - rank) / max(rank_total - 1, 1), 0, 1)`
- 歧义点：两式在 `rank=1` 时分别为 `(rt-1)/rt` 与 `1.0`。`rt=50` 时是 0.98 vs 1.00；`rt=10`（百度）时是 0.90 vs 1.00，**差 0.10**；榜尾两式均为 0。且 03:143 的注释「rank=1（榜首）→ rank_pct≈1.0」只对 03 式成立，与 01 式矛盾
- 建议定义（统一采用 03 式，并修正 01）：

```
rank_pct = clamp( (rank_total - rank) / max(rank_total - 1, 1), 0, 1 )
  rank=1        → 1.0（榜首满分）
  rank=rank_total → 0.0（榜尾零分）
  rank 或 rank_total 缺失/≤0 → rank_pct = null，权重按 A-6 重分配
01-需求规格.md:55 同步改为上式，并删除「榜首≈1，榜尾≈0」的模糊表述

跨平台可比性（必须显式承认并给出兜底）：
  上式只做「榜内相对位置」归一化，不解决「榜长不同导致的热度含金量差异」——
  百度第 5 位（10 位榜）与微博第 5 位（50 位榜）会给出 0.556 与 0.918，
  但前者的绝对曝光量远高于后者。V1 的兜底是 A-6 注的「按 heat 分位排序」，
  V2 引入 platform_board_weight（按各榜历史平均互动量标定，默认 1.0，范围 [0.8, 1.2]）。
```

`[A-8] 生命周期五阶段判定与最小驻留`
- 当前描述（03:192-212，判据表 + min_dwell + 状态图）
- 歧义点：① 五条判据无覆盖全空间、无初始态（B-6）；② min_dwell 点数 vs 时长错配（B-7）；③ 判据表 `rising` 无 heat 条件，状态图 `decaying→rising` 有 `heat>30`（M-3）；④ `latent` 的「采样点 <3」与 `peak` 的 `heat≥80` 可能同时满足时的优先级未定；⑤ `long_tail` 要求「近 24h 仍有更新」与 `min_dwell=6 点`（3 小时）时间粒度不匹配；⑥ `duration_points` 的计算方式（累加计数器 vs 由 stage_since 推导）决定幂等性，未定义
- 建议定义：

```
求值顺序（03:192 判据表改为有序列表，第一个命中为目标阶段）：
  0) n_points < 3                                   → latent（强制）
  1) heat < 20 ∨ (heat < P25_24h ∧ age > 7d) ∧ 近24h有更新   → long_tail
  2) heat ≥ 80 ∨ heat ≥ P99_24h                              → peak
     ∨ (v_ewma < 0 ∧ heat ≥ 60 ∧ 上阶段 ∈ {rising, peak})
  3) v_ewma > v_min(6.0) ∧ 持续 ≥2 点                         → rising
  4) v_ewma < 0 ∧ 持续 ≥3 点                                  → decaying
  5) 近 48h 无更新                                             → dormant（新增）
  6) default                                                   → 保持当前阶段
min_dwell_points = {latent:0, rising:2, peak:1, decaying:3, long_tail:6, dormant:0}
  按 B-7 用「落在该阶段的样本点数」计数，不用墙钟时长
duration_points（契约 stage_evidence.duration_points）
  = 当前阶段内已写入的样本点数（与 dwell_points 同一计数器，重启后由 InfluxDB count 恢复）
  ⚠ 必须是「由 stage_since 反查点数」而非累加器，否则任务重跑会重复计数（破坏幂等）
二次爆发：decaying → rising 走第 3 条（不加 heat>30），与判据表一致；状态图 232 行按 M-3 修正
```

`[A-9] 突发方法一：Z-score 残差`
- 当前描述（03:255-261）：`residual(t) = heat(t) - h_ewma(t)`；`σ = stddev(residual over last W=24 points)`；`hit ⟺ |residual(t)| > k·σ ∧ σ > σ_min`，`k=3, σ_min=1.0`
- 歧义点：① 窗口含当前点 → 掩蔽效应（S-9）；② `|residual|` 双尾会把暴跌判为突发；③ `stddev` 是样本（n-1）还是总体（n）未定；④ `σ_min=1.0` 的物理单位（heat 分）与「极平稳序列」的量化标准未说明；⑤ 与 velocity 线性相关（B-5）
- 建议定义：

```
B(t)      = h_ewma(t-1)                          # 基线只含历史，不含当前点
r(t)      = heat(t) - B(t)                       # 有符号残差
R         = { r(t-W), ..., r(t-1) }              # 窗口不含当前点，W = 24
MAD       = median( |r - median(R)| )
σ         = max( 1.4826 × MAD, σ_min )           # 稳健估计，抗单点污染；σ_min = 1.0（单位：heat 分）
z(t)      = r(t) / σ                             # 单尾（上尾）
hit_zscore⟺ z(t) > k (k = 3.0) ∧ |R| ≥ W_min (W_min = 8)
  ⚠ 即便如此，z 与 velocity 仍线性相关（见 A-12），本方法不能作为独立票；
    独立性必须靠「通道化投票」或「改用长周期（STL/7 日同期）基线」来解决
```

`[A-10] 突发方法二：双阈值增速`
- 当前描述（03:264-269）：`hit ⟺ velocity(t) > v_min_burst(8.0) ∧ heat(t) > h_min_burst(40)`
- 歧义点：① `velocity(t)` 是 `velocity(t)` 还是 `v_ewma(t)`（M-2）；② `heat(t)` 是原始 heat 还是 `h_ewma(t)`；③ `velocity > 8.0` 与 lifecycle 的 `v_min = 6.0` 关系未说明（burst 应更严格，但 8 vs 6 的标定依据缺失）；④ 与 zscore 冗余（B-5）
- 建议定义：

```
hit_velocity ⟺ v_ewma(t) > v_min_burst ∧ h_ewma(t) > h_min_burst
   统一用平滑值（避免单点噪声）
   v_min_burst = 8.0 分/小时，h_min_burst = 40
   标定依据（补进文档）：v_min_burst 对应「30 分钟内 heat 上升 ≥4 个 EWMA 点」，
   约为近 30 日全体话题 velocity 分布的 P97（V1 先用 8.0，M4 用首周实测分布回标）
   与 lifecycle v_min=6.0 的关系：burst 为告警级（更严、需第二票），lifecycle 为标注级（更宽、单点即可）
   ⚠ 若采用 S-21 建议②，追加 accel 条件：∧ accel(t) > 0 —— 用于区分「持续上涨」与「脉冲噪声」，
     同时降低与方法一的相关性（accel 是二阶量，与一阶 velocity 弱相关）
```

`[A-11] 突发方法三：榜单跃迁`
- 当前描述（03:272-277）：`(首次进榜 ∧ heat > 20) ∨ (rank(t-N) - rank(t) >= rank_jump_min)`，`N=1 周期`，默认 10 位
- 歧义点：① 「首次进榜」是「史上首次」还是「本轮首次」未定义；② 固定 10 位对短榜不可达（S-10）；③ `rank = -1` 哨兵参与减法 → 掉榜/进榜瞬间虚高命中；④ `rank_total` 变化（榜单改版）时跃迁位数不可比；⑤ 非榜单平台（B 站/YouTube）恒不可用
- 建议定义：

```
unavailable（不投票、不计入分母）⟺ rank(t) ≤ 0 ∨ rank_total(t) ≤ 0
首次进榜 ⟺ rank(t) > 0 ∧ rank(t-1) ≤ 0 ∧ 该 topic 历史首次进榜标记 first_listing_done = false
  命中后置 first_listing_done = true（避免每次掉榜再进榜都算首次）
跃迁（短长榜统一用相对阈值）：
  thr = max( 3, ceil(0.2 × rank_total(t)) )        # 50 位榜 → 10 位；10 位榜 → 3 位（原固定 10 不可达）
  jump⟺ rank(t-N) > 0 ∧ rank(t) > 0 ∧ rank_total(t) == rank_total(t-N)
        ∧ rank(t-N) - rank(t) >= thr
  N = 1 个采样周期；rank_total 变化（榜单改版）时该方法本轮记 unavailable
短榜（rank_total ≤ 10）退化：jump ⟺ 进入前 3 ∨ 首次进榜
hit_ranjump ⟺ (首次进榜 ∧ heat(t) > 20) ∨ jump
evidence 输出：{rank_jump: 实际跃迁位数, rank_prev: rank(t-N), rank_curr: rank(t), thr: thr}
```

`[A-12] 突发投票与三方法独立性` ★★ 最严重算法问题
- 当前描述（03:280-283）：`IsBurst ⟺ count(methodsHit) >= 2`
- 歧义点：**三方法并非独立**。
- 数学证明（基于 03:168-171 与 03:255-269 的定义）：

```
令 X = heat(t) - h_ewma(t-1)
h_ewma(t)  = α·heat(t) + (1-α)·h_ewma(t-1)                α = 0.4
residual(t)= heat(t) - h_ewma(t) = (1-α)·X = 0.6·X
velocity(t)= (h_ewma(t) - h_ewma(t-1))/Δt = α·X/Δt = 0.4·X/0.5 = 0.8·X   (Δt = 0.5h)
⟹ residual(t) = 0.75 × velocity(t)          —— 确定性线性关系，相关系数 ρ = 1

代入判据：
  hit_zscore   ⟺ |residual| > k·σ ⟺ velocity > k·σ/0.75 = 4σ
  hit_velocity ⟺ velocity > 8.0 ∧ heat > 40
⟹ 二者是「同一个统计量 velocity 的两个阈值」。
   当 σ < 2.0 时 4σ < 8.0，凡方法二命中者方法一必命中 ⟹ 方法二的票 100% 冗余，零独立信息量。
   （若方法二改用 v_ewma，ρ 降至约 0.9+，仍高度相关，投票意义有限。）
进一步：rank_pct 是 heat 的输入 ⟹ rank_jump 与 heat 通道也是正相关的，三票中无真正独立者。
```
- 建议定义（结构性修复，调参无法解决）：

```
方案一（推荐，V1 落地）：通道化投票 —— 要求两张票来自不同数据通道
  channel(zscore)    = heat          （heat 通道）
  channel(velocity)  = heat          （heat 通道，最多贡献 1 票）
  channel(rank_jump) = rank          （榜单通道，独立数据源）
  channel(eng_spike) = engagement    （互动通道，新增）

  IsBurst ⟺ |hit| ≥ 2 ∧ ∃ m1, m2 ∈ hit : channel(m1) ≠ channel(m2)
  （等价于：两个 heat 通道方法合计最多 1 票；必须再有一个 rank 或 engagement 通道的票）

  新增第四方法（互动通道，与 heat 弱相关，同时修复 S-11 的非榜单平台冷启动）：
    hit_engspike ⟺ eng(t) > 3 × median(近 7 日同时段 eng)
                   ∧ eng(t) 绝对值 > 1000
    其中 eng = likes + comments + shares + favorites

方案二（V2）：方法一改用长周期基线 —— baseline = STL 的 trend+seasonal 分量
  （需 ≥14 日历史以估计日内季节性），使 residual 反映「偏离同期常态」而非「偏离短期均线」，
  从而在数学上解除与 velocity 的线性关系。

CI 回归门禁（防止回退）：
  对黄金数据集计算 corr(velocity, residual)，断言 |ρ| < 0.9（方案二下应 < 0.5）
```

`[A-13] 突发冷启动与告警生成/抑制`
- 当前描述（03:241-286 全节）：无最少点数门禁；无告警冷却；无全局限流；无静音实体
- 歧义点：① zscore 需 24 点（12h），黄金窗口内不可用（S-11）；② 无 `unavailable` 与 `未命中` 的区分 → 误报率分母错误；③ 无告警去重/冷却 → 单一话题持续突发刷屏；④ 无全局预算 → 阈值误配时推送风暴；⑤ 「首次进榜」与冷启动的关系未利用
- 建议定义：

```
【冷启动分级】
  n_points < 3        → 不做突发检测，不落 BurstEvent（不算分母），返回 IsBurst=false
  3 ≤ n_points < 8    → 仅启用 velocity 与 rank_jump；vote_threshold 提为 2（即二者须同时命中）
                        zscore 记 unavailable（不投票、不计入分母）
  n_points ≥ 8        → 三方法（V1 修完后为四方法）全启用；σ 窗口用可用点数，要求 |R| ≥ 8
  首现跨平台规则（age < 2h）：
    首次进榜 ∧ heat ≥ 40 ∧ platform_count ≥ 2 → 记方法 first_cross_platform（属 rank 通道），
    仍需搭配任一其他通道才满足 ≥2

【告警抑制三级】
  新增 Ent：TopicMute{ id, topic_id, muted_until, muted_by, channels, reason, created_at } idx(topic_id)
  新增 Ent：BurstEvent{ id, topic_id, checked_at, n_points, methods_available[],
                        methods_hit[], evidence jsonb, is_burst bool,
                        suppressed_reason string }  idx(topic_id, checked_at)
    —— 误报率 = FP / count(BurstEvent where is_burst=true)，分母来源明确
  新增 Ent：AlertDelivery{ id, alert_id, channel, state, attempts, last_error, sent_at }（见 M-12）

  ① 静音：TopicMute 命中（且未过 muted_until）→ 不生成 Alert，仍写 BurstEvent（suppressed_reason=muted）
  ② 冷却：同 topic 在 alert_cooldown_minutes(360) 内已有 Alert → 不新建，
           仅更新原 Alert 的 evidence / hit_count / last_hit_at
  ③ 全局预算：单 5 分钟窗口 Alert 数 > global_alert_budget(20) → 超出的按 heat 降序截断，
              被截断的写 BurstEvent（suppressed_reason=budget_exceeded），
              下一周期合并为 1 条「热点综述」推送
  ④ 升级：同 topic 连续 3 周期命中且 heat 较上次 +15 以上 → 允许突破冷却 1 次

【REST 调整】
  新增 POST /api/v1/media/tt/topics/{id}/mute   （静音话题，契约 §7 语义）
  保留 POST /api/v1/media/tt/alerts/{id}/ack
  /alerts/{id}/mute 废弃（或改语义为「本条不再提醒，不影响 topic 级推送」）
```

`[A-14] 快照时间戳 t 的定义、时间桶对齐与迟到回填`
- 当前描述（03:118 / 02:271「依赖 `metrics._as_of` 判断快照新鲜度」）：`t` 从未定义
- 歧义点：① `t` 取 `_as_of`、`collected_at` 还是 ingest 时刻？② 跨平台采集时刻不齐（S-25）；③ 迟到数据无落点（S-23）；④ 时钟漂移无校验；⑤ 同桶内多条数据的取舍未定
- 建议定义：

```
【t 的取值（按优先级）】
  1) IntelItem.metrics._as_of      （契约 §1 定义，首选）
  2) IntelItem.source.collected_at
  3) 二者均无 → 丢弃 + 计数 tt_ingest_no_timestamp_total（禁止用 now 兜底，否则破坏幂等）

【时钟漂移校验】
  若 t ∈ [now - 24h, now + 5min]  → 正常写入
  若 t < now - 24h                → 写入并打 late=true，触发回填（见下）
  若 t > now + 5min               → 丢弃 + 计数 tt_ingest_future_timestamp_total
                                    （同时告警，提示上游采集器时钟异常）

【时间桶对齐】
  t_bucket = floor(t / bucket_minutes) × bucket_minutes，bucket_minutes = 30
  InfluxDB 写入时间戳用 t_bucket，原始 t 存为 field `t_raw`（RFC3339 字符串或 epoch）
  ⟹ (topic_id, platform, t_bucket) 唯一，JetStream 重投覆盖而非追加（满足 NFR-TT-07）
  ⟹ 微博 10:05 与 B 站 10:22 同落 10:00 桶，跨平台曲线可对齐（满足 US-7）
  同桶多条（如 10:05 与 10:28 均落 10:00）→ 取 _as_of 最大者（last-wins），
    并写 field `sample_count` 记录被折叠的条数（用于缺失率统计，避免虚低）

【迟到回填】
  late=true 的点写入后，投递 asynq 任务 tt.recompute：
    重算区间 [t_late, now] 内该 topic 的全部派生指标（h_ewma / velocity / accel / heat），
    覆盖写 InfluxDB（幂等，依赖 (series, field, ts) 覆盖语义）
    重算期间对该 topic 置 recompute_lock，抑制告警（避免重复触发）
    重算完成后解锁并补发一次突发检测（同样受 A-13 的冷却约束）
```

`[A-15] 权重变更的断点处理`
- 当前描述（03:130）：「`effective_from` 权重生效时间（保证历史可比）」
- 歧义点：只保证了「历史 heat 不重算」，未处理派生指标跨断点跳变（S-16）
- 建议定义：

```
变更流程（PATCH /tt/weights）：
  ① 写 WeightConfig 新行，effective_from = 变更时刻（不得回溯）
  ② 生成 weight_epoch = sha256(WeightConfig.id)[:8]
  ③ 对所有活跃 Topic 与后续写入的 InfluxDB 点打 field `weight_epoch`（字符串 field）
  ④ 变更生效后该 topic 的第一个采样点强制 α = 1（重置 h_ewma 与 v_ewma），
     避免跨口径差分产生假 velocity ⟹ 防止「改一次权重，全站话题集体 rising」的伪突发潮
  ⑤ 变更窗口内（1 个采样周期）全局抑制突发告警
  ⑥ 历史点不重算；看板按 weight_epoch 分段渲染，并在 UI 标注「口径变更于 YYYY-MM-DD HH:mm」
  ⑦ 契约输出：HeatFactors 同时给出原始权重与生效权重（见 A-6），供跨 epoch 审计
```

`[A-16] InfluxDB 写入缓冲、背压与死信`
- 当前描述（02:160「落 Redis 缓冲队列（TTL 1h）」/ 03:505 / 03:535「buffer_depth > 5000 critical」）
- 歧义点：TTL 1h 与「不丢点」矛盾；阈值 5000 与实际流量（500 点/秒）差 360 倍；无容量上限、无死信、无重放失败处理、无背压
- 建议定义：

```
缓冲结构：Redis Stream `tt:influx:buf`（消费组 tt_writer），非普通 TTL 队列
  XADD 上限 MAXLEN ≈ 2,000,000 条（≈ 400MB，需按实际 point 大小压测确认）
  达上限时：XTRIM 最旧 + 计数 tt_influx_dropped_total + critical 告警
  ⚠ 需求 §9 诚实声明应补一条：「InfluxDB 宕机超过约 1 小时且缓冲写满后，原始快照点会丢失；
     系统保证『尽力不丢 + 显式告知丢失量』，不保证『绝对不丢』」— 与 §9.2 的诚实口径一致
重放：独立 worker，XPENDING 处理超时消息；单条重试 5 次仍失败 →
  XACK 后 XADD 到 `tt:influx:poison`（保留 30d），人工介入
背压：NATS JetStream 消费者 max_ack_pending = 2000，ack_wait = 30s
  缓冲深度 > 500,000 → 暂停拉取（不 nack，避免红elivery 风暴）
  缓冲深度 < 100,000 → 恢复
告警阈值修正（与流量对齐，替换 03:535）：
  tt_influx_buffer_depth > 100,000 (≈15 分钟缓冲) → warn
  tt_influx_buffer_depth > 500,000                 → critical
  tt_influx_dropped_total 增长 > 0                 → critical（直接违反 NFR-TT-01，须人工介入）
降级提示（NFR-TT-08）：查询失败时看板返回最近 N 点内存态，
  N = 200（与契约 series 上限一致），并显式标注「时序库不可用，以下为最近约 4 天的内存快照」
```

`[A-17] 降采样（水位线 + 逐字段聚合 + bucket 路由）`
- 当前描述（03:46-52 / 512）：`range(start: -30m)` + `aggregateWindow(every: 5m, fn: mean)` + 「30 分钟重叠保证补数据不重不漏」
- 歧义点：无水位线（S-14）；统一 mean 对 rank/bool 语义错误（S-15）；5m 对 30m 采样无压缩；查询未做 bucket 路由（NFR-TT-02 无保障）
- 建议定义：

```
【水位线】
  watermark 存 Redis `tt:ds:watermark:mm_1h`（首次运行置 now - 24h）
  每次任务：range(start: watermark - 30m, stop: now - 5m)
  成功后：watermark = 本次 stop
  ⟹ 中断任意时长均可补齐；配合 (series, field, ts) 覆盖语义保证幂等

【聚合层重定义】
  mm_raw  90d（原始，30 分钟桶）
  mm_1h   1y   ← 原 mm_5m 取消（5 分钟对 30 分钟采样无压缩意义）
  mm_1d   3y   ← 原 mm_1h(3y) 下移
【逐字段聚合（V1 先在 Go 侧 pivot 成一行多列再写，避免 Flux 逐 field 聚合的复杂度）】
  heat_score / velocity / accel / eng_rate        → mean
  rank_pct / eng_pct / freshness / breadth        → mean（新增，见 B-3）
  rank        → filter(rank > 0) 后取 min（最好名次）；全为 -1 则写 -1
  rank_total  → last
  views / likes / comments                        → last（若上游确认为增量值则改 sum，须先定论）
  is_interpolated → 改为 interp_ratio = sum(is_interpolated) / count
  sample_count                                    → sum
【补数（late arrival）】
  每 1h 跑一次 backfill：range(start: now - 6h, stop: now - 5m)，重算并覆盖写
【查询 bucket 路由（保障 NFR-TT-02 P95 ≤800ms）】
  span ≤ 7d     → mm_raw
  7d < span ≤ 90d → mm_1h
  span > 90d      → mm_1d
  ⚠ 路由规则须写进 data/tt/influx.go 的注释与单元测试；90 天查询压测用 mm_1h 路径
```

`[A-18] suggested_angles 与 material_refs 生成`
- 当前描述：全文无生成逻辑（S-20），但契约 §6 必填、MME 概要 §351 明确依赖
- 建议定义：

```
suggested_angles（最多 3 条，每条 ≤15 字）：
  1. 取 topic 下 compliance.gate = pass 且 workability.score 最高的前 5 条 IntelItem
  2. 按 category 套模板：
     A_hot       → 「{事件}：{首发平台}最先曝光的 3 个细节」
     B_material  → 「{标题}：可剪辑的 {N} 个高光片段（约 {时长}）」
     C_market    → 「{事件}：对 {entities 中的 org/tech} 的传导路径」
  3. LLM 改写为差异化角度，temperature ≤ 0.4，输出 ≤15 字/条，去重（两两相似度 < 0.6）
  4. 落库前过合规闸门（复用 MME 的内容审核）；失败则 suggested_angles = []，仍产生候选
material_refs：
  = dedup( IntelItem.payload.media_refs where compliance.gate == "pass" )
  若 Q1 拍板「不做切片搬运」→ 过滤掉非 asset:// 且非自有/授权/CC 的条目
  上限 10 条；空数组合法
⚠ 以上须补进 04 §4.5 任务清单（+2 人日）与 03 §2.4
```

`[A-19] series 窗口截取（≤200 点）`
- 当前描述（契约 §156 / 01:130）：「`series` 数组在 NATS 消息中不得超过 200 点」
- 歧义点：200 点如何分配未定义（M-18）
- 建议定义：

```
平台数 P，预算 B = 200
第一段（窗口确定）：
  span = 最近 200/P ÷ (点/小时) 小时；若 span > 90d 则截断到 90d
  ⚠ 30 分钟采样下每平台 48 点/日，P=5 时每平台 40 点 = 20 小时；
     若 P 较大导致窗口过短（< 6h），改为全局等距降采样（见第二段）
第二段（降采样兜底）：当 P > 8 或所需窗口 < 6h 时
  全窗口取最近 7d，按等时间间隔抽样到恰好 200 点，跨平台交替取点，
  并在响应头带 `X-Series-Downsampled: true` 与 `X-Series-Window: 7d`
规则：同一消息内所有平台的点数之差 ≤ 1（保证跨平台对比公平）
完整时序一律走 GET /api/v1/media/tt/topics/{id}/series 分页（page_size 默认 1000，上限 5000）
```

---

## 六、缺失项清单

**契约与接口层**
1. `TrendSeries` / `Alert` / `Heat` / `Breadth` / `SeriesPoint` / `Burst` / `Origin` 的 Proto message（B-2、C-8）
2. 三份契约 message 的 `api_version` 顶层字段（C-11）
3. 契约 `heat` 缺 `eng_pct` / `freshness`、`factors` 缺 `w_*_eff`、`TrendSeries` 缺 `diagnostics`（C-4、C-5、C-12）
4. TrendSeries 到 MIA 的传输通道（REST 还是新 NATS subject，M-22）
5. 契约 §0.3 未登记 `ts_`（TrendSubscription）前缀（M-10）
6. JSON Schema 文件存放位置不一致：契约 §10 写 `docs/systems/00-shared/schema/`，04 §4.1 写 `pkg/mediacontract`（M-6）
7. `sys_config` → 实际表名 `system_config` 的映射与 key 命名规范（M-4）

**算法层**
8. 话题聚类完整算法（B-1）
9. 突发第四方法 `eng_spike`（互动通道，用于修复相关性与冷启动，A-12）
10. `suggested_angles` / `material_refs` 生成规则（A-18、S-20）
11. 插值补点的完整规则（M-13）
12. `label` / `representative_intel_id` 选取规则（M-19）
13. `compliance` 在话题级（多 IntelItem 混合 gate）的聚合规则——`decideRoute` 只接收单个 `it`，未定义选哪条
14. 各阈值的标定依据与回标计划（v_min=6.0、v_min_burst=8.0、h_min=40、k=3、σ_min=1.0、θ_merge=0.82 全部无来源说明），需在 M4 前建立「首周实测分布 → 回标阈值」的机制
15. `accel` 的消费方（S-21）

**存储层**
16. Ent `Topic` 缺：四因子取值与生效权重、`platforms`、`stage_duration_points`、`weight_epoch`、`merge_lock`、`needs_review`（B-3、C-6、C-15）
17. Ent `TopicSource` 缺：`id`、PK、`cluster_key`、`intel_id` 反向索引、`moved_by` / `moved_at`（B-1）
18. Ent `TopicCandidate` 缺：`created_at`、`snooze_until`、`is_pinned`、`sort_bias`、`candidate_seq`、`trigger_reason`、`created_by`（M-9、S-18、S-19）
19. Ent `Alert` 缺：`ack_at`、`ack_by`、`mute_until`、`dedupe_key`、`hit_count`、`topic_label`（M-11）
20. 缺三张表：`TopicMute`、`BurstEvent`、`AlertDelivery`（A-13、M-12）
21. 缺 `TopicLabelHistory`（M-19）
22. InfluxDB 缺 field：四因子取值、`weight_epoch`、`t_raw`、`sample_count`（B-3、A-14、A-15）
23. InfluxDB 建库参数：retention policy 具体值、shard group duration、series 基数上限与压测验收（S-12）
24. `WeightConfig` heat / candidate 两套权重混一张表，一半字段恒空；缺 `updated_at`、`is_active`、唯一约束、热加载与缓存失效策略（M-4）

**工程层**
25. 错误码 → HTTP / gRPC Reason 的完整映射表（M-7）
26. 并发互斥：asynq 任务级去重 + topic 级分布式锁（M-8）
27. 迟到数据回填与重算的完整链路（S-23、A-14）
28. 时钟漂移校验与 `_as_of` 合理窗口（A-14）
29. 背压：NATS `max_ack_pending` / `ack_wait`、缓冲上限、暂停/恢复阈值（A-16）
30. 死信：`tt:influx:poison` 队列与人工介入流程（A-16）
31. 推送 outbox 与多渠道部分失败的补偿（M-12）
32. 归档任务：无任何任务写入 `is_archived`（M-17）
33. 单元测试缺 6 类高危用例（M-20），CI 缺「三方法独立性」回归断言（A-12）
34. 观测性缺指标：`tt_series_cardinality`、`tt_influx_dropped_total`、`tt_ingest_no_timestamp_total`、`tt_ingest_future_timestamp_total`、`tt_ingest_too_old_total`、`tt_recompute_total`、`tt_alert_suppressed_total`（按 suppressed_reason 分标签）
35. 观测性缺：InfluxDB `weight_epoch` 分布（用于发现口径混用）、`tt_stage_no_match_total`（B-6 的运行时护栏，非 0 即判据表有洞）

**进度层**
36. M0 计划日期错误（B-11）
37. 人日算术错误与容量缺口未披露（S-2）；前端 5 个页面 + 因子面板从未计入
38. M5 DoD 的 7 日观察与窗口末日冲突（S-1）
39. 砍单顺序第 5 条与 ADR-TT-02 及契约 ID 规范冲突（S-22）
40. `suggested_angles` 生成未进 M4/M5 任务清单（S-20）
41. 缺少「契约冻结」的具体判据与责任人（04 §6 只写「⚠️ 待评审」，无 DoD）
42. 缺少「首周实测分布回标阈值」的排期（M4 之后应有一次调参窗口，当前排期到 02-06 截止，无调参时间）

---

**评审结论**：四份文档的骨架完整、诚实声明（需求 §9）质量较高，但**核心算法层未达到可编码标准**——B-1（聚类零算法）、B-5（投票机制被自身公式推翻）、B-3/B-4（因子不落盘且不可重现）、B-6（状态机有洞）四项在进入 M2/M3 前必须闭环，否则 42 人日的投入会大量消耗在返工上。建议：M1 前完成契约层修正（C-1 至 C-15）与 B-2；M2 开工前闭环 B-3/B-4/B-11；M3 开工前闭环 B-1/B-6/B-7；M4 开工前闭环 B-5/B-8/B-10。

[Agent ID: agent-8209d9b4-2636-4604-b8a4-6cbc6a195993]