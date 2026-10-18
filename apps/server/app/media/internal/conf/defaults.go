package conf

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultProbeTimeout = 3 * time.Second
	defaultReadyTimeout = 30 * time.Second
	defaultGenWatchdog  = 600 * time.Second
	defaultIdleReap     = 300 * time.Second
)

// 默认车道配置：infer 串行（GPU 纪律）、render 半并发（CPU 密集）、io 宽松。
var defaultLanes = map[string]int32{
	"infer":  1,
	"render": 2,
	"io":     4,
}

// Normalize 填充默认值（对标 biztt 模式：独立 yaml + 补默认 + 判红线）。
func (m *Media) Normalize() {
	if m.Jobs == nil {
		m.Jobs = &Media_Jobs{}
	}
	if len(m.Jobs.Lanes) == 0 {
		m.Jobs.Lanes = make(map[string]int32, len(defaultLanes))
		for k, v := range defaultLanes {
			m.Jobs.Lanes[k] = v
		}
	}
	if m.Jobs.RetainDays <= 0 {
		m.Jobs.RetainDays = 30
	}
	if m.Jobs.EventCapPerJob <= 0 {
		m.Jobs.EventCapPerJob = 500
	}
	if m.Jobs.QueueCapacity <= 0 {
		m.Jobs.QueueCapacity = 256
	}

	if m.Storage == nil {
		m.Storage = &Media_Storage{}
	}
	if m.Storage.ArtifactDir == "" {
		m.Storage.ArtifactDir = "./storage/media/artifacts"
	}

	if m.Doctor == nil {
		m.Doctor = &Media_Doctor{}
	}
	if len(m.Doctor.Probes) == 0 {
		m.Doctor.Probes = []string{"ffmpeg", "ffprobe", "yt-dlp", "python", "ollama"}
	}
	if m.Doctor.ProbeTimeout == nil || m.Doctor.ProbeTimeout.AsDuration() <= 0 {
		m.Doctor.ProbeTimeout = durationpb.New(defaultProbeTimeout)
	}

	if m.Engines == nil {
		m.Engines = &Media_Engines{}
	}
	if m.Engines.Default == "" {
		m.Engines.Default = "echo"
	}

	if m.Sidecar == nil {
		m.Sidecar = &Media_Sidecar{}
	}
	sc := m.Sidecar
	if sc.ReadyTimeout == nil || sc.ReadyTimeout.AsDuration() <= 0 {
		sc.ReadyTimeout = durationpb.New(defaultReadyTimeout)
	}
	if sc.GenWatchdog == nil || sc.GenWatchdog.AsDuration() <= 0 {
		sc.GenWatchdog = durationpb.New(defaultGenWatchdog)
	}
	if sc.IdleReap == nil || sc.IdleReap.AsDuration() <= 0 {
		sc.IdleReap = durationpb.New(defaultIdleReap)
	}
	if sc.MaxFrameBytes <= 0 {
		sc.MaxFrameBytes = 64 << 20
	}
}

// Validate 红线校验：无效配置直接终止（fail-fast 胜过错配静默运行）。
func (m *Media) Validate() error {
	if !m.Enabled {
		return nil
	}
	if m.Jobs == nil || len(m.Jobs.Lanes) == 0 {
		return errors.New("media.jobs.lanes 至少配置一条任务车道")
	}
	for name, conc := range m.Jobs.Lanes {
		if conc < 1 || conc > 64 {
			return fmt.Errorf("media.jobs.lanes[%q] 并发度 %d 越界 [1,64]", name, conc)
		}
	}
	if m.Jobs.EventCapPerJob < 16 {
		return fmt.Errorf("media.jobs.event_cap_per_job=%d 过小（<16）", m.Jobs.EventCapPerJob)
	}
	if m.Storage == nil || m.Storage.ArtifactDir == "" {
		return errors.New("media.storage.artifact_dir 不能为空")
	}
	if m.Sidecar == nil || m.Sidecar.ReadyTimeout.AsDuration() <= 0 ||
		m.Sidecar.GenWatchdog.AsDuration() <= 0 {
		return errors.New("media.sidecar 就绪握手与生成看门狗超时必须为正")
	}
	return nil
}
