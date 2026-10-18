package data

import "time"

// derefTime *time.Time → time.Time（nil → 零值）。MME 各 repo 共用。
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}
