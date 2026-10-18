package engines

import "errors"

// codedError 实现 ErrorCode() 的包装错误（jobrun.classifyError 识别）。
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// ErrorCode 稳定公开错误码（jobrun 落 media_jobs.error_code）。
func (e *codedError) ErrorCode() string { return e.code }

// WithPublicCode 给引擎层错误挂上稳定公开码；
// handler 把它返回后，任务失败码即 MEDIA_* 词表里的稳定值。
func WithPublicCode(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrEngineNotFound):
		return &codedError{code: "MEDIA_ENGINE_NOT_FOUND", err: err}
	case errors.Is(err, ErrEngineNoCPUFallbk):
		return &codedError{code: "MEDIA_ENGINE_NO_CPU_FALLBACK", err: err}
	case errors.Is(err, ErrEngineUnavailable):
		return &codedError{code: "MEDIA_ENGINE_UNAVAILABLE", err: err}
	case errors.Is(err, ErrEngineLicense):
		return &codedError{code: "MEDIA_ENGINE_LICENSE", err: err}
	case errors.Is(err, ErrSynthesisFailed):
		return &codedError{code: "MEDIA_SYNTHESIS_FAILED", err: err}
	default:
		return &codedError{code: "MEDIA_JOB_FAILED", err: err}
	}
}
