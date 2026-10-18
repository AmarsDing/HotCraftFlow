package tt

import (
	"context"
	"encoding/json"

	"github.com/go-kratos/kratos/v2/log"
)

// WriterUsecase 消费 mm.trend.snapshot.v1 → 入批写 InfluxDB（C-9：
// InfluxDB 写入只发生在本消费端；失败批次由 PointSink 内部转 SnapshotBuffer）。
type WriterUsecase struct {
	conf *Conf
	sink PointSink
	log  *log.Helper
}

// NewWriterUsecase 构造。
func NewWriterUsecase(conf *Conf, sink PointSink, logger log.Logger) *WriterUsecase {
	return &WriterUsecase{
		conf: conf,
		sink: sink,
		log:  log.NewHelper(log.With(logger, "module", "biz/tt-writer")),
	}
}

// HandleSnapshotData 处理一条 mm.trend.snapshot.v1 消息体。
// 返回 error → 订阅器 NAK 重投（JetStream 至少一次由写入侧的 (series,ts) 覆盖幂等兜底）。
func (uc *WriterUsecase) HandleSnapshotData(ctx context.Context, data []byte) error {
	var msg SnapshotPublished
	if err := json.Unmarshal(data, &msg); err != nil {
		uc.log.Warnf("decode snapshot failed (poison, ack & drop): %v", err)
		return nil // 格式毒消息不重投
	}
	return uc.sink.Record(ctx, msg.Point)
}

// Sink 返回底层批处理器（订阅器做背压查询与优雅停机冲刷）。
func (uc *WriterUsecase) Sink() PointSink { return uc.sink }
