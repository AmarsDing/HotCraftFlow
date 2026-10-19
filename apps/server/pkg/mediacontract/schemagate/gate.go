// Package schemagate 跨系统契约（① IC IntelItem；② TT TrendSeries / TopicCandidate /
// Alert）的 JSON Schema 闸门。发布前校验，违约消息不得出门（评审 C-1~C-15 的落地检查点）。
//
// schema 内嵌自 docs/systems/00-shared/schema/ 的同名片段（SSOT 在 docs），
// gate_test.go 的漂移测试防两侧不一致。
package schemagate

import (
	"bytes"
	"embed"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json
var schemaFS embed.FS

// 契约名 → schema 文件名。
const (
	TrendSeries    = "trend-series.v1.json"
	TopicCandidate = "topic-candidate.v1.json"
	Alert          = "alert.v1.json"
	// IntelItem ① IC 输出契约（S-34；跨系统词表全部锚定契约 §1 闭集）。
	IntelItem = "intel-item.v1.json"
)

var compiled = mustCompile()

func mustCompile() map[string]*jsonschema.Schema {
	out := map[string]*jsonschema.Schema{}
	for _, name := range []string{TrendSeries, TopicCandidate, Alert, IntelItem} {
		raw, err := schemaFS.ReadFile("schemas/" + name)
		if err != nil {
			panic(fmt.Sprintf("schemagate: read %s: %v", name, err))
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			panic(fmt.Sprintf("schemagate: parse %s: %v", name, err))
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(name, doc); err != nil {
			panic(fmt.Sprintf("schemagate: add %s: %v", name, err))
		}
		sch, err := c.Compile(name)
		if err != nil {
			panic(fmt.Sprintf("schemagate: compile %s: %v", name, err))
		}
		out[name] = sch
	}
	return out
}

// Validate 用指定契约校验一份 JSON 文档。schema 名非法或文档不是 JSON 直接报错；
// 违约返回 *jsonschema.ValidationError（含逐字段定位，可直接进日志/告警）。
func Validate(schemaName string, doc []byte) error {
	sch, ok := compiled[schemaName]
	if !ok {
		return fmt.Errorf("schemagate: unknown schema %q", schemaName)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return fmt.Errorf("schemagate: %s 非合法 JSON: %w", schemaName, err)
	}
	return sch.Validate(inst)
}

// ValidateTrendSeries / ValidateTopicCandidate / ValidateAlert / ValidateIntelItem 常用快捷入口。
func ValidateTrendSeries(doc []byte) error    { return Validate(TrendSeries, doc) }
func ValidateTopicCandidate(doc []byte) error { return Validate(TopicCandidate, doc) }
func ValidateAlert(doc []byte) error          { return Validate(Alert, doc) }
func ValidateIntelItem(doc []byte) error      { return Validate(IntelItem, doc) }
