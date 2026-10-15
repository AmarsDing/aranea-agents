package skillruntime

import (
	"strings"
)

// asciiWordContains 在已小写的 query 中查找已小写的 ASCII trigger，
// 要求命中位置前后不是 ASCII 词字符（字母/数字/下划线）。
func asciiWordContains(queryLower, triggerLower string) bool {
	idx := 0
	for idx <= len(queryLower) {
		i := strings.Index(queryLower[idx:], triggerLower)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(triggerLower)
		beforeOK := start == 0 || !isASCIIWordChar(queryLower[start-1])
		afterOK := end == len(queryLower) || !isASCIIWordChar(queryLower[end])
		if beforeOK && afterOK {
			return true
		}
		idx = start + 1
	}
	return false
}

func isASCIIWordChar(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z')
}
