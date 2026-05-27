package output

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"aranea-agents/internal/cli/clierr"
	"github.com/fatih/color"
)

type textPrinter struct {
	w       io.Writer
	quiet   bool
	noColor bool
}

func (p *textPrinter) noColorFn() bool { return p.noColor }

func (p *textPrinter) red(s string) string {
	if p.noColor {
		return s
	}
	return color.New(color.FgRed, color.Bold).Sprint(s)
}

func (p *textPrinter) yellow(s string) string {
	if p.noColor {
		return s
	}
	return color.New(color.FgYellow).Sprint(s)
}

func (p *textPrinter) green(s string) string {
	if p.noColor {
		return s
	}
	return color.New(color.FgGreen).Sprint(s)
}

func (p *textPrinter) dim(s string) string {
	if p.noColor {
		return s
	}
	return color.New(color.Faint).Sprint(s)
}

func (p *textPrinter) bold(s string) string {
	if p.noColor {
		return s
	}
	return color.New(color.Bold).Sprint(s)
}

func (p *textPrinter) PrintList(items any, total int) error {
	rows := toRows(items)
	if len(rows) == 0 {
		fmt.Fprintln(p.w, p.dim("(no items)"))
		return nil
	}

	if p.quiet {
		for _, row := range rows {
			id := row["id"]
			if id == "" {
				id = row["Id"]
			}
			fmt.Fprintln(p.w, id)
		}
		return nil
	}

	// Determine column headers from the first row.
	if len(rows) == 0 {
		return nil
	}
	headers := prioritizedKeys(rows[0])

	// Print header.
	fmt.Fprintf(p.w, "%-36s  %-30s  %-12s\n",
		p.bold(truncStr(headers[0], 36)),
		p.bold(safeIdx(headers, 1, "name")),
		p.bold(safeIdx(headers, 2, "status")),
	)
	fmt.Fprintln(p.w, strings.Repeat("-", 82))

	for _, row := range rows {
		id := truncStr(row[safeIdx(headers, 0, "id")], 36)
		name := truncStr(row[safeIdx(headers, 1, "name")], 30)
		status := row[safeIdx(headers, 2, "status")]

		statusStr := status
		if !p.noColor {
			switch strings.ToLower(status) {
			case "active", "enabled", "published":
				statusStr = p.green(status)
			case "inactive", "disabled", "draft":
				statusStr = p.dim(status)
			}
		}
		fmt.Fprintf(p.w, "%-36s  %-30s  %s\n", id, name, statusStr)
	}
	fmt.Fprint(p.w, p.dim(fmt.Sprintf("\nTotal: %d\n", total)))
	return nil
}

func (p *textPrinter) PrintDetail(item any) error {
	rows := toRows(item)
	if len(rows) == 0 {
		// Try single map.
		if m, ok := item.(map[string]string); ok {
			rows = []map[string]string{m}
		}
	}
	if len(rows) == 0 {
		fmt.Fprintf(p.w, "%v\n", item)
		return nil
	}
	row := rows[0]
	keys := prioritizedKeys(row)
	for _, k := range keys {
		v := row[k]
		if p.quiet && k != "id" && k != "status" {
			continue
		}
		fmt.Fprintf(p.w, "%-24s  %s\n", p.bold(k), v)
	}
	return nil
}

func (p *textPrinter) PrintError(e *clierr.CLIError) error {
	fmt.Fprintf(p.w, "%s  %s",
		p.red("Error"),
		p.red(e.Code),
	)
	if e.HTTPStatus > 0 {
		fmt.Fprintf(p.w, " (HTTP %d)", e.HTTPStatus)
	}
	fmt.Fprintln(p.w)
	if e.Message != "" {
		fmt.Fprintf(p.w, "message: %s\n", e.Message)
	}
	if e.Hint != "" {
		fmt.Fprintf(p.w, "hint   : %s\n", p.yellow(e.Hint))
	}
	if e.Cause != nil {
		fmt.Fprintf(p.w, "cause  : %v\n", p.dim(e.Cause.Error()))
	}
	return nil
}

func (p *textPrinter) PrintSuccess(message string, kv ...string) error {
	fmt.Fprintf(p.w, "%s  %s\n", p.green("✓"), message)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(p.w, "  %-16s  %s\n", kv[i], kv[i+1])
	}
	return nil
}

func (p *textPrinter) PrintKeyValue(pairs ...string) error {
	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(p.w, "%s=%s\n", pairs[i], pairs[i+1])
	}
	return nil
}

// toRows converts various item types to []map[string]string.
func toRows(items any) []map[string]string {
	if items == nil {
		return nil
	}
	// Already a slice of maps.
	if s, ok := items.([]map[string]string); ok {
		return s
	}
	// Reflect: slice of structs / interfaces.
	v := reflect.ValueOf(items)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice {
		var result []map[string]string
		for i := range v.Len() {
			row := reflectToMap(v.Index(i).Interface())
			if row != nil {
				result = append(result, row)
			}
		}
		return result
	}
	// Single item.
	if m := reflectToMap(items); m != nil {
		return []map[string]string{m}
	}
	return nil
}

// reflectToMap converts a struct or map to map[string]string via reflection.
func reflectToMap(item any) map[string]string {
	if item == nil {
		return nil
	}
	if m, ok := item.(map[string]string); ok {
		return m
	}
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return map[string]string{"value": fmt.Sprintf("%v", item)}
	}
	t := v.Type()
	m := make(map[string]string, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		key := fieldKey(f)
		val := fmt.Sprintf("%v", v.Field(i).Interface())
		m[key] = val
	}
	return m
}

// fieldKey extracts the JSON name from a struct field tag.
func fieldKey(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		tag = f.Tag.Get("protobuf")
	}
	if tag != "" {
		parts := strings.Split(tag, ",")
		name := parts[0]
		// protobuf tags: look for name= or json_name=
		for _, p := range parts {
			if strings.HasPrefix(p, "name=") {
				name = strings.TrimPrefix(p, "name=")
			}
		}
		if name != "" && name != "-" {
			return name
		}
	}
	return strings.ToLower(f.Name)
}

// prioritizedKeys returns keys in a display-friendly order.
func prioritizedKeys(m map[string]string) []string {
	priority := []string{"id", "key", "agent_key", "name", "display_name", "slug", "status", "enabled", "created_at"}
	var ordered []string
	seen := make(map[string]bool)
	for _, k := range priority {
		if _, ok := m[k]; ok {
			ordered = append(ordered, k)
			seen[k] = true
		}
	}
	for k := range m {
		if !seen[k] {
			ordered = append(ordered, k)
		}
	}
	return ordered
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func safeIdx(s []string, i int, fallback string) string {
	if i < len(s) {
		return s[i]
	}
	return fallback
}
