package schema

import "time"

// timeNow 返回当前 UTC 时间，作为 Ent Schema 字段默认值。
func timeNow() time.Time {
	return time.Now().UTC()
}
