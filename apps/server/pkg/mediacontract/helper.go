package mediacontract

import (
	"fmt"
	"strconv"
)

// errField 构造一个字段级契约违规错误。
func errField(field, msg string) error {
	return fmt.Errorf("mediacontract: 契约违规 %s: %s", field, msg)
}

// itoa 是 strconv.Itoa 的包内别名，避免在结构体文件里重复 import。
func itoa(i int) string { return strconv.Itoa(i) }
