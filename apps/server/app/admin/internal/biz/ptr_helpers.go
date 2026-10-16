package biz

func ptrString(v string) *string { return &v }
func ptrInt32(v int32) *int32    { return &v }
func ptrBool(v bool) *bool       { return &v }
