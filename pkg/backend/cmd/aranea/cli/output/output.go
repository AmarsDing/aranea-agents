// Package output 集中所有「向用户打印结果」的调用，使 --output、--quiet
// 等全局标志在各子命令中行为一致。目前支持 text、json、table 三种格式。
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
)

var (
	currentFormat = "text"
	currentQuiet  = false
	colorEnabled  = true
)

// Configure 在 PersistentPreRunE 中调用一次，使本包在进程存活期内记住用户偏好。
func Configure(format string, quiet, noColor bool) {
	if format != "" {
		currentFormat = strings.ToLower(format)
	}
	currentQuiet = quiet
	if noColor {
		colorEnabled = false
	} else {
		colorEnabled = isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == ""
	}
}

// Format 返回当前启用的输出格式。
func Format() string { return currentFormat }

// Quiet 表示用户是否要求最小输出。
func Quiet() bool { return currentQuiet }

// Color 表示是否应发出 ANSI 颜色。
func Color() bool { return colorEnabled }

// Render 是主力：按当前格式选择编码器，将 value 写入 w。表格通过对
// 结构体切片的反射生成；若反射失败则回退 JSON 编码，确保总有输出。
func Render(w io.Writer, value any) {
	switch currentFormat {
	case "json":
		renderJSON(w, value)
	case "table":
		if !renderTable(w, value) {
			renderJSON(w, value)
		}
	default:
		if !renderText(w, value) {
			renderJSON(w, value)
		}
	}
}

func renderJSON(w io.Writer, value any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func renderText(w io.Writer, value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		_, _ = fmt.Fprintln(w, v.String())
		return true
	case reflect.Slice, reflect.Array:
		// 对切片回退为表格；renderTable 已写入 w。
		return renderTable(w, value)
	case reflect.Struct:
		// 列表响应包装（{Items, Total, Limit, Offset}）用表格比键值倾卸更易读，先检测该形态。
		if items := v.FieldByName("Items"); items.IsValid() && items.Kind() == reflect.Slice {
			return renderSliceTable(w, items)
		}
		printStruct(w, v, "")
		return true
	}
	return false
}

func renderTable(w io.Writer, value any) bool {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	// 许多列表端点返回 {Items: [...], Total: N}。检测并解包。
	if v.Kind() == reflect.Struct {
		if items := v.FieldByName("Items"); items.IsValid() && items.Kind() == reflect.Slice {
			return renderSliceTable(w, items)
		}
		return false
	}
	if v.Kind() != reflect.Slice {
		return false
	}
	return renderSliceTable(w, v)
}

func renderSliceTable(w io.Writer, slice reflect.Value) bool {
	if slice.Len() == 0 {
		if !currentQuiet {
			_, _ = fmt.Fprintln(w, "(empty)")
		}
		return true
	}
	first := slice.Index(0)
	for first.Kind() == reflect.Ptr {
		first = first.Elem()
	}
	if first.Kind() != reflect.Struct {
		return false
	}
	headers := pickColumns(first.Type())
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if !currentQuiet {
		_, _ = fmt.Fprintln(tw, strings.Join(highlightHeaders(headers), "\t"))
	}
	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		row := make([]string, len(headers))
		for j, name := range headers {
			row[j] = stringify(elem.FieldByName(structFieldFromHeader(elem.Type(), name)))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush() == nil
}

// pickColumns 为结构体返回至多六列有信息量的列。优先顺序：ID/Key/Name
// 等标识，再若干人类可读字段，再类状态字段。目标是从不一倾卸所有列，
// 而给出可用的概览。
func pickColumns(t reflect.Type) []string {
	preferred := []string{
		"ID", "Key", "Slug", "AgentKey", "ToolKey",
		"DisplayName", "Name", "Title",
		"Status", "Enabled", "RiskLevel",
		"Provider", "Model",
		"UpdatedAt", "CreatedAt",
	}
	have := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() && !f.Anonymous {
			have[f.Name] = true
		}
	}
	chosen := []string{}
	for _, name := range preferred {
		if have[name] && len(chosen) < 6 {
			chosen = append(chosen, jsonHeader(t, name))
		}
	}
	if len(chosen) == 0 {
		// 回退：枚举前六个已导出的标量字段。
		for i := 0; i < t.NumField() && len(chosen) < 6; i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Anonymous {
				continue
			}
			switch f.Type.Kind() {
			case reflect.String, reflect.Bool,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
				chosen = append(chosen, jsonHeader(t, f.Name))
			}
		}
	}
	sort.SliceStable(chosen, func(i, j int) bool { return false })
	return chosen
}

// jsonHeader 若存在则返回该字段的 json 标签（不含选项）。
func jsonHeader(t reflect.Type, fieldName string) string {
	if f, ok := t.FieldByName(fieldName); ok {
		tag := f.Tag.Get("json")
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			return tag
		}
	}
	return strings.ToLower(fieldName)
}

// structFieldFromHeader 是 jsonHeader 的逆。
func structFieldFromHeader(t reflect.Type, header string) string {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		if tag == header {
			return f.Name
		}
	}
	return header
}

func stringify(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return ""
		}
		return stringify(v.Elem())
	case reflect.String:
		return v.String()
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Slice, reflect.Array:
		return fmt.Sprintf("[%d]", v.Len())
	case reflect.Struct:
		return "{...}"
	}
	return fmt.Sprintf("%v", v.Interface())
}

func printStruct(w io.Writer, v reflect.Value, indent string) {
	t := v.Type()
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}
		header := jsonHeader(t, f.Name)
		_, _ = fmt.Fprintf(tw, "%s%s:\t%s\n", indent, dim(header), stringify(v.Field(i)))
	}
	_ = tw.Flush()
}

// Success 打印友好的成功行（--quiet 时跳过）。
func Success(w io.Writer, message string) {
	if currentQuiet {
		return
	}
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[32mok\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "ok %s\n", message)
}

// Warn 在 stderr 上打印警告，即使在 --quiet 模式下也输出。
func Warn(w io.Writer, message string) {
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[33mwarn\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "warn %s\n", message)
}

// Error 在 stderr 上打印错误行。
func Error(w io.Writer, message string) {
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[31merror\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "error %s\n", message)
}

func dim(s string) string {
	if !colorEnabled {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

func highlightHeaders(headers []string) []string {
	if !colorEnabled {
		return headers
	}
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = "\x1b[1m" + h + "\x1b[0m"
	}
	return out
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
