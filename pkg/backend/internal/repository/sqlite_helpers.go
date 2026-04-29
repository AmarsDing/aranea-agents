package repository

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

// idCounter 单调递增，用于区分同一纳秒内生成的 ID；在紧循环插入
//（测试、批量遥测等）中仅靠 UnixNano() 会碰撞，故此计数器很关键。
var idCounter atomic.Uint64

// uniqueID 组合出可排序、抗碰撞的 ID。ns 前缀保持自然时间顺序，
// 计数器后缀保证同一纳秒内被重复调用时仍唯一。
func uniqueID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), idCounter.Add(1))
}

// scanner 抽象 *sql.Row 与 *sql.Rows，便于扫描辅助函数在单行与多行查询中复用。
type scanner interface {
	Scan(dest ...any) error
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// optionalColumn 目前原样传回列名；保留为单一间接层，以便迁移惰性增加列
// 时需要替换为 NULL 占位符。
func optionalColumn(_ string, column string) string {
	return column
}

// previewText 返回 value 的前 limit 个字符并加省略号。
// limit 非正或内容已更短时，在 TrimSpace 后原样返回。
func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

// normalizeJSONList 确保持久化值为合法 JSON 数组。裸字符串会包成单元素数组；空输入为 "[]"。
func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	encoded, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// decodeJSONFloatMap 将 `{"key":number}` 形式的 JSON 解析为 Go map。
// 空或非法输入返回 nil。供 L4 智能体演进仓库的 tool/provider/model 偏好列使用。
func decodeJSONFloatMap(raw string) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	out := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// encodeJSONFloatMap 是 decodeJSONFloatMap 的逆操作。空 map 序列化为 "{}"，避免列出现非法空字符串。
func encodeJSONFloatMap(in map[string]float64) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// EncodeFloat32Blob 将 float32 向量序列化为小端字节，以便在 SQLite BLOB 列中往返。
func EncodeFloat32Blob(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	out := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

// decodeFloat32Blob 是 EncodeFloat32Blob 的逆操作。字节长度不是 4 的倍数时返回错误。
func decodeFloat32Blob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, errors.New("invalid float32 blob length")
	}
	out := make([]float32, len(blob)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return out, nil
}

// vectorNorm 返回 float32 向量的 L2 范数。用于余弦相似度查询侧及填充 embedding_norm 列。
func vectorNorm(vec []float32) float64 {
	if len(vec) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

// dotProduct 为向量召回路径中使用的内积（利于展开循环）。
func dotProduct(a []float32, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// sanitizePromptFileID 将任意提示文件名规范为适合作主键的稳定 id。
func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}
