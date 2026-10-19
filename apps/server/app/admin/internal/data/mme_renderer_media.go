package data

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/golang-jwt/jwt/v5"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/conf"
)

// envRenderExecutor MME 渲染执行面开关：local（缺省，进程内 FFmpegRenderer）| media
// （委托 media 服务）。走 env 而非 conf.pb：conf.proto 在并行 TT 线在途，
// 且 pb 再生成工具链未验证——下次 MME 配置正变动时顺手提进 pb。
const (
	envRenderExecutor = "MME_RENDER_EXECUTOR"
	envMediaBaseURL   = "MME_MEDIA_BASE_URL"
	defaultMediaURL   = "http://127.0.0.1:7790"
)

// NewMMERenderer MME 渲染器 provider：按 env 开关在本地/委托实现间选择。
// 缺省 local——未设 env 时行为与改动前逐字节一致（并行 MME 线零感知）。
func NewMMERenderer(cfg bizmme.Config, bc *conf.Bootstrap, logger log.Logger) bizmme.Renderer {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(envRenderExecutor)), "media") {
		base := strings.TrimRight(os.Getenv(envMediaBaseURL), "/")
		if base == "" {
			base = defaultMediaURL
		}
		return NewMediaServiceRenderer(base, bc.GetAuth().GetAuthn().GetJwt().GetKey(), logger)
	}
	return NewFFmpegRenderer(cfg, logger)
}

// ------ MediaServiceRenderer：经 media 服务 REST/SSE 委托渲染 ------

// MediaServiceRenderer 把 bizmme.Renderer 语义落到 media 服务的
// jobstore/jobrun/exectree 执行面上：
//   - Render = POST studio/jobs（lane=render, type=mme.render, 幂等键=
//     mme_render_<variantID>，重试同键不重复渲染）→ SSE 进度回传 onProgress
//     → 终态后 GET job 行取权威 result/error；
//   - ctx 取消 → best-effort POST :cancel（media 侧树杀 ffmpeg 进程组）；
//   - SSE 建连失败时降级为 1.5s 轮询 GetJob（代理剪长连的兜底）。
//
// 鉴权：与 media 同钥自铸 HS256（sub=admin-mme-render，12h），401 时重铸重试一次。
type MediaServiceRenderer struct {
	base string
	key  []byte
	hc   *http.Client
	log  *log.Helper

	tokMu  sync.Mutex
	tok    string
	tokExp time.Time
}

// NewMediaServiceRenderer 构造。
func NewMediaServiceRenderer(baseURL, jwtKey string, logger log.Logger) bizmme.Renderer {
	return &MediaServiceRenderer{
		base: strings.TrimRight(baseURL, "/"),
		key:  []byte(jwtKey),
		hc:   &http.Client{Timeout: 15 * time.Second},
		log:  log.NewHelper(log.With(logger, "module", "mme/media-renderer")),
	}
}

// token 取缓存令牌；过期/空则重铸。
func (r *MediaServiceRenderer) token() (string, error) {
	r.tokMu.Lock()
	defer r.tokMu.Unlock()
	if r.tok != "" && time.Now().Add(time.Minute).Before(r.tokExp) {
		return r.tok, nil
	}
	exp := time.Now().Add(12 * time.Hour)
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "admin-mme-render", "exp": exp.Unix(), "iat": time.Now().Unix(),
	}).SignedString(r.key)
	if err != nil {
		return "", fmt.Errorf("mint media token: %w", err)
	}
	r.tok, r.tokExp = signed, exp
	return signed, nil
}

// doJSON 带鉴权发 JSON 请求；401 时重铸令牌重试一次。返回 (响应体, 状态码)。
func (r *MediaServiceRenderer) doJSON(ctx context.Context, method, url string, body []byte) ([]byte, int, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := r.token()
		if err != nil {
			return nil, 0, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rd)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		// kratos 自定义动词 POST 必须显式 Content-Type，否则 400。
		req.Header.Set("Content-Type", "application/json")
		resp, err := r.hc.Do(req)
		if err != nil {
			return nil, 0, err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			r.tokMu.Lock()
			r.tok = "" // 强制重铸
			r.tokMu.Unlock()
			continue
		}
		return raw, resp.StatusCode, nil
	}
	return nil, 0, fmt.Errorf("unreachable")
}

// jstr 双键容忍取串（kratos json 编解码 camelCase；兜底 proto 原名）。
func jstr(m map[string]any, camel, proto string) string {
	if v, ok := m[camel].(string); ok && v != "" {
		return v
	}
	if v, ok := m[proto].(string); ok {
		return v
	}
	return ""
}

// jobView media MediaJob 的最小视图。
type jobView struct {
	ID         string
	State      string
	Progress   float64
	ResultJSON string
	ErrorCode  string
	ErrorMsg   string
}

func parseJobView(raw []byte) (jobView, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return jobView{}, fmt.Errorf("job json unparseable: %w", err)
	}
	v := jobView{
		ID:         jstr(m, "id", "id"),
		State:      jstr(m, "state", "state"),
		ResultJSON: jstr(m, "resultJson", "result_json"),
		ErrorCode:  jstr(m, "errorCode", "error_code"),
		ErrorMsg:   jstr(m, "errorMsg", "error_msg"),
	}
	if p, ok := m["progress"].(float64); ok {
		v.Progress = p
	}
	return v, nil
}

func (r *MediaServiceRenderer) getJob(ctx context.Context, id string) (jobView, error) {
	raw, code, err := r.doJSON(ctx, http.MethodGet, r.base+"/api/v1/media/studio/jobs/"+id, nil)
	if err != nil {
		return jobView{}, err
	}
	if code != http.StatusOK {
		return jobView{}, fmt.Errorf("get job %s: status %d: %s", id, code, tail(string(raw), 300))
	}
	return parseJobView(raw)
}

// Available doctor 探活（3s）+ ffmpeg 探针须 ok。
func (r *MediaServiceRenderer) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	probeOK, alive := r.probe(ctx, "ffmpeg")
	return alive && probeOK
}

// probe 查 doctor 里 name 探针；返回 (探针 ok, media 服务可达)。
func (r *MediaServiceRenderer) probe(ctx context.Context, name string) (bool, bool) {
	raw, code, err := r.doJSON(ctx, http.MethodGet, r.base+"/api/v1/media/studio/doctor", nil)
	if err != nil || code != http.StatusOK {
		return false, false
	}
	var rep struct {
		Probes []struct {
			Name    string `json:"name"`
			OK      bool   `json:"ok"`
			Version string `json:"version"`
		} `json:"probes"`
	}
	if json.Unmarshal(raw, &rep) != nil {
		return false, false
	}
	for _, p := range rep.Probes {
		if p.Name == name {
			return p.OK, true
		}
	}
	return false, true
}

// Version doctor 报告里的 ffmpeg version 串；不可得为空串。
func (r *MediaServiceRenderer) Version(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, code, err := r.doJSON(ctx, http.MethodGet, r.base+"/api/v1/media/studio/doctor", nil)
	if err != nil || code != http.StatusOK {
		return ""
	}
	var rep struct {
		Probes []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"probes"`
	}
	if json.Unmarshal(raw, &rep) != nil {
		return ""
	}
	for _, p := range rep.Probes {
		if p.Name == "ffmpeg" {
			return p.Version
		}
	}
	return ""
}

// mediaRenderPayload 与 media mme.render handler 的载荷契约一致。
type mediaRenderPayload struct {
	VariantID   string  `json:"variant_id"`
	SrcPath     string  `json:"src_path"`
	DstPath     string  `json:"dst_path"`
	CoverPath   string  `json:"cover_path,omitempty"`
	SrtPath     string  `json:"srt_path,omitempty"`
	ManifestDst string  `json:"manifest_dst,omitempty"`
	Aspect      struct {
		Key    string `json:"key"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Filter string `json:"filter"`
	} `json:"aspect"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Margin   struct {
		Bottom int `json:"bottom"`
		Top    int `json:"top"`
	} `json:"margin"`
	Preset string `json:"preset"`
	CRF    int    `json:"crf"`
}

// Render 委托 media 执行渲染。
func (r *MediaServiceRenderer) Render(ctx context.Context, spec bizmme.RenderSpec, onProgress func(p float64)) (*bizmme.RenderResult, error) {
	var p mediaRenderPayload
	p.VariantID = spec.VariantID
	p.SrcPath = spec.SrcPath
	p.DstPath = spec.DstPath
	p.CoverPath = spec.CoverPath
	p.SrtPath = spec.SrtPath
	p.ManifestDst = spec.ManifestDst
	p.Aspect.Key = spec.Aspect.Key
	p.Aspect.Width = spec.Aspect.Width
	p.Aspect.Height = spec.Aspect.Height
	p.Aspect.Filter = spec.Aspect.Filter
	p.StartSec = spec.StartSec
	p.EndSec = spec.EndSec
	p.Margin.Bottom = spec.Margin.Bottom
	p.Margin.Top = spec.Margin.Top
	p.Preset = spec.Preset
	p.CRF = spec.CRF
	pl, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	// M2 修复：幂等键纳入渲染内容指纹——此前仅 "mme_render_{VariantID}"，media 侧
	// jobstore 无限期保留 → QC 失败编辑后重渲/取消后重试都命中旧（done）job 的
	// stale result_json。指纹取全部渲染参数（源/区间/画幅/字幕/预设/CRF）的 FNV-1a：
	// 内容或参数任何变化自然产生新键，同参重试仍命中同一 job（幂等语义保留）。
	fp := fnv.New64a()
	_, _ = fp.Write(pl)
	idem := fmt.Sprintf("mme_render_%s_%016x", spec.VariantID, fp.Sum64())
	submit, err := json.Marshal(map[string]any{
		"lane":           "render",
		"type":           "mme.render",
		"payloadJson":    string(pl),
		"idempotencyKey": idem,
	})
	if err != nil {
		return nil, err
	}

	raw, code, err := r.doJSON(ctx, http.MethodPost, r.base+"/api/v1/media/studio/jobs", submit)
	if err != nil {
		return nil, fmt.Errorf("%w: media submit: %v", bizmme.ErrRenderFail, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("%w: media submit status %d: %s", bizmme.ErrRenderFail, code, tail(string(raw), 300))
	}
	j, err := parseJobView(raw)
	if err != nil || j.ID == "" {
		return nil, fmt.Errorf("%w: media submit response unparseable: %v", bizmme.ErrRenderFail, err)
	}

	// 取消看护：ctx 一死就 best-effort 远端取消（media 侧树杀）。
	stopWatch := context.AfterFunc(ctx, func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, _ = r.doJSON(cctx, http.MethodPost, r.base+"/api/v1/media/studio/jobs/"+j.ID+":cancel", []byte("{}"))
	})
	defer stopWatch()

	// SSE 进度；建连失败降级轮询。
	final, werr := r.watchSSE(ctx, j.ID, onProgress)
	if werr != nil {
		r.log.Warnf("sse watch for %s failed (%v); fallback to polling", j.ID, werr)
		final, werr = r.pollJob(ctx, j.ID, onProgress)
		if werr != nil {
			return nil, fmt.Errorf("%w: media watch: %v", bizmme.ErrRenderFail, werr)
		}
	}

	switch final.State {
	case "done":
		var res struct {
			VideoPath   string  `json:"video_path"`
			CoverPath   string  `json:"cover_path"`
			SrtPath     string  `json:"srt_path"`
			DurationSec float64 `json:"duration_sec"`
		}
		if err := json.Unmarshal([]byte(final.ResultJSON), &res); err != nil || res.VideoPath == "" {
			return nil, fmt.Errorf("%w: media result_json unparseable: %v", bizmme.ErrRenderFail, err)
		}
		if onProgress != nil {
			onProgress(1)
		}
		return &bizmme.RenderResult{
			VideoPath:   res.VideoPath,
			CoverPath:   res.CoverPath,
			SrtPath:     res.SrtPath,
			DurationSec: res.DurationSec,
		}, nil
	case "cancelled":
		return nil, fmt.Errorf("%w: media job cancelled", bizmme.ErrRenderFail)
	default:
		return nil, fmt.Errorf("%w: media render failed [%s] %s", bizmme.ErrRenderFail, final.ErrorCode, final.ErrorMsg)
	}
}

// watchSSE 挂 /api/v1/media/jobs/{id}/events 消费 progress→终态；终态后回查权威行。
func (r *MediaServiceRenderer) watchSSE(ctx context.Context, id string, onProgress func(p float64)) (jobView, error) {
	tok, err := r.token()
	if err != nil {
		return jobView{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		r.base+"/api/v1/media/jobs/"+id+"/events", nil)
	if err != nil {
		return jobView{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "text/event-stream")
	hc := &http.Client{} // 长连不设整体超时；取消靠 ctx
	resp, err := hc.Do(req)
	if err != nil {
		return jobView{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return jobView{}, fmt.Errorf("sse status %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	kind, data := "", ""
	terminal := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, ":"):
			// keepalive
		case strings.HasPrefix(line, "event: "):
			kind = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if kind != "" {
				switch kind {
				case "job.progress":
					var m struct {
						Progress float64 `json:"progress"`
					}
					if json.Unmarshal([]byte(data), &m) == nil && onProgress != nil {
						onProgress(m.Progress)
					}
				case "job.done", "job.failed", "job.cancelled":
					terminal = true
				}
			}
			kind, data = "", ""
		}
		if terminal {
			break
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return jobView{}, fmt.Errorf("sse read: %w", err)
	}
	if ctx.Err() != nil {
		return jobView{}, ctx.Err()
	}
	return r.getJob(ctx, id)
}

// pollJob SSE 失败时的轮询兜底（1.5s，直至终态）。
func (r *MediaServiceRenderer) pollJob(ctx context.Context, id string, onProgress func(p float64)) (jobView, error) {
	tk := time.NewTicker(1500 * time.Millisecond)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return jobView{}, ctx.Err()
		case <-tk.C:
			j, err := r.getJob(ctx, id)
			if err != nil {
				continue // 瞬断容忍：下拍再试；ctx 兜底
			}
			if onProgress != nil && j.Progress > 0 {
				onProgress(j.Progress)
			}
			switch j.State {
			case "done", "failed", "cancelled":
				return j, nil
			}
		}
	}
}
