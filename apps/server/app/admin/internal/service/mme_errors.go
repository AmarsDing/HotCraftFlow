package service

import (
	"errors"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"

	bizmme "framework/app/admin/internal/biz/mme"
)

// mmeBizErr MME 领域哨兵错误 → HTTP 状态分层（docs/systems/04-MME/03-详细设计 §5：400/404/409/502）。
// reason 取哨兵串（MME_*，前端可精确分支）；message 保留完整上下文（含 variant_id 等）。
// 未命中哨兵的错误原样透传（kratos 默认 500），不吞不包。
func mmeBizErr(err error) error {
	if err == nil {
		return nil
	}
	layers := []struct {
		code    int
		targets []error
	}{
		// 400 客户端输入类：驳回未填原因 / 展开超规模 / 无内容版本。
		{400, []error{bizmme.ErrRejectReasonEmpty, bizmme.ErrExpandTooLarge, bizmme.ErrHighlightsEmpty}},
		// 404 资源不存在。
		{404, []error{bizmme.ErrTaskNotFound, bizmme.ErrSetNotFound, bizmme.ErrVariantNotFound, bizmme.ErrJobNotFound, bizmme.ErrAssetMissing}},
		// 409 状态机冲突 / QC 硬门禁 / 存储满。
		{409, []error{bizmme.ErrInvalidState, bizmme.ErrQCFailed, bizmme.ErrQCBlock, bizmme.ErrStorageFull}},
		// 502 下游依赖不可用（yt-dlp/whisper/ollama/ffmpeg/asynq 队列）。
		{502, []error{bizmme.ErrDownloadFail, bizmme.ErrASRFail, bizmme.ErrLLMUnavailable, bizmme.ErrRenderFail, bizmme.ErrQueueUnavailable}},
	}
	for _, layer := range layers {
		for _, target := range layer.targets {
			if errors.Is(err, target) {
				return kratoserrors.New(layer.code, target.Error(), err.Error())
			}
		}
	}
	return err
}
