package biz

import (
	"encoding/json"
	"strconv"
	"strings"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 字典分类用途取值（与 proto DictCategory.kind 对齐）。
const (
	DictCategoryKindEnum  = "enum"  // 枚举选项型：给前端下拉/业务编码提供候选值
	DictCategoryKindParam = "param" // 参数配置型：后端代码读取的运行时配置
)

// DictCategoryMeta 服务端权威分类元数据。
// 消费方（Consumers）是代码耦合事实——哪个模块的哪段代码在读这个分类，只有代码知道，
// 不属于可编辑数据，故由注册表声明而非入库；前端硬编码清单（CONSUMED_CATEGORY_KEYS）由此替代。
type DictCategoryMeta struct {
	Code      string   // 分类编码（= dict.type）
	Name      string   // 默认中文名（种子写入 dict_category.name）
	Kind      string   // enum / param
	Builtin   bool     // 系统内置：禁删分类、禁删条目（param）、禁改编码/分类（身份锁定）
	Consumers []string // 业务消费方说明（人类可读，与代码读取点一一对应）
	// Validate 内置参数类分类的值校验；返回 nil 表示合法。仅声明于 param 类。
	Validate func(itemKey, value string) error
}

// dictCategoryRegistry 内置分类注册表（种子来源 + 护栏依据）。
// 新增后端读字典的代码点时，必须在此登记对应分类，否则"业务接入"标记不可信。
var dictCategoryRegistry = map[string]DictCategoryMeta{
	"asset_subsystem": {
		Code:    "asset_subsystem",
		Name:    "资产所属子系统",
		Kind:    DictCategoryKindEnum,
		Builtin: true,
		Consumers: []string{
			"资产管理 · 新建资产向导（所属子系统，按条目 ID 引用）",
			"拓扑配置 · 域建议",
		},
	},
	"warehouse.env_thresholds": {
		Code:      "warehouse.env_thresholds",
		Name:      "库房环境阈值（全局）",
		Kind:      DictCategoryKindParam,
		Builtin:   true,
		Consumers: []string{"智能库房 · 环境阈值评估（全局默认）"},
		Validate:  validateEnvThresholdValue,
	},
	"warehouse.device_offline_alert": {
		Code:      "warehouse.device_offline_alert",
		Name:      "库房设备离线告警开关",
		Kind:      DictCategoryKindParam,
		Builtin:   true,
		Consumers: []string{"智能库房 · 设备离线告警 Worker"},
		Validate:  validateBoolSwitchValue,
	},
	"inventory": {
		Code:      "inventory",
		Name:      "库存参数",
		Kind:      DictCategoryKindParam,
		Builtin:   true,
		Consumers: []string{"库存统计 · 呆滞判定阈值（stagnant_days）"},
		Validate:  validateInventoryValue,
	},
}

// lookupDictCategoryMeta 查注册表元数据；未登记返回 false。
func lookupDictCategoryMeta(code string) (DictCategoryMeta, bool) {
	meta, ok := dictCategoryRegistry[code]
	return meta, ok
}

// validateEnvThresholdValue 校验 warehouse.env_thresholds 值：
// item_key=config/default 必须是 JSON 对象（合并全部键）；其余键为标量（数字/布尔/字符串，按 JSON 解析）。
// 与 data 层 LoadGlobalThresholds 的组装约定一一对应。
func validateEnvThresholdValue(itemKey, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return adminV1.ErrorBadRequest("库房环境阈值不允许空值")
	}
	if itemKey == "config" || itemKey == "default" {
		var obj map[string]any
		if err := json.Unmarshal([]byte(v), &obj); err != nil {
			return adminV1.ErrorBadRequest("item_key=%s 的值必须是合法 JSON 对象", itemKey)
		}
		return nil
	}
	var scalar any
	if err := json.Unmarshal([]byte(v), &scalar); err != nil {
		return adminV1.ErrorBadRequest("标量阈值项必须是合法 JSON 标量（数字/布尔/字符串），如 30、true")
	}
	return nil
}

// validateBoolSwitchValue 校验开关类值（与 LoadDeviceOfflineAlertEnabled 兼容集一致）。
func validateBoolSwitchValue(_, value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off":
		return nil
	default:
		return adminV1.ErrorBadRequest("开关值仅允许 true/false/1/0/yes/no/on/off")
	}
}

// validateInventoryValue 校验库存参数值：stagnant_days 必须为正整数（天）。
func validateInventoryValue(itemKey, value string) error {
	if itemKey != "stagnant_days" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return adminV1.ErrorBadRequest("stagnant_days 必须为正整数（天）")
	}
	return nil
}
