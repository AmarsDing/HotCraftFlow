package jobrun

import "testing"

// TestMMERenderNormalize M4 回归门：crf 未传（json 省略 → 0）回填默认 23——0 在
// x264 是无损（~10x 码率）而非「沿用默认」；preset 缺省 medium。非法 CRF 不在
// 规整层遮盖，原样交给调用点范围检查判 BAD_SPEC。
func TestMMERenderNormalize(t *testing.T) {
	cases := []struct {
		name                string
		crf                 int
		preset              string
		wantCRF             int
		wantPreset          string
	}{
		{"crf 省略（0）回填 23", 0, "fast", 23, "fast"},
		{"显式 23 不动", 23, "medium", 23, "medium"},
		{"下界有效值 1 不动", 1, "fast", 1, "fast"},
		{"上界有效值 51 不动", 51, "fast", 51, "fast"},
		{"负值不遮盖（留给范围检查拒）", -1, "fast", -1, "fast"},
		{"越界 52 不遮盖（留给范围检查拒）", 52, "fast", 52, "fast"},
		{"preset 缺省回填 medium", 23, "", 23, "medium"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mmeRenderPayload{Preset: c.preset, CRF: c.crf}
			p.normalize()
			if p.CRF != c.wantCRF || p.Preset != c.wantPreset {
				t.Fatalf("in(crf=%d,preset=%q) 想要 (crf=%d,preset=%q)，得到 (crf=%d,preset=%q)",
					c.crf, c.preset, c.wantCRF, c.wantPreset, p.CRF, p.Preset)
			}
		})
	}
}
