package mme

import (
	contract "framework/pkg/mediacontract"
)

// 内部实体 ID 前缀（不进契约 §0.3 分配表，但保持同形 ULID 纪律）。
const prefixTemplate = "tpl"

// newTemplateID 模板 ULID（tpl_ 前缀）。
func newTemplateID() string { return contract.NewID(prefixTemplate) }
