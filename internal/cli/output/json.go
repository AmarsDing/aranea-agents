package output

import (
	"encoding/json"
	"fmt"
	"io"

	"aranea-agents/internal/cli/clierr"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type jsonPrinter struct {
	w     io.Writer
	quiet bool
}

var jsonOpts = protojson.MarshalOptions{
	Multiline:       true,
	Indent:          "  ",
	UseProtoNames:   false,
	EmitUnpopulated: false,
}

func (p *jsonPrinter) PrintList(items any, total int) error {
	type listResp struct {
		Items any `json:"items"`
		Total int `json:"total"`
	}

	if p.quiet {
		// Each item: extract id and print.
		return printQuietJSON(p.w, items)
	}

	var out any
	switch v := items.(type) {
	case []proto.Message:
		var jsonItems []json.RawMessage
		for _, msg := range v {
			b, err := jsonOpts.Marshal(msg)
			if err != nil {
				return err
			}
			jsonItems = append(jsonItems, json.RawMessage(b))
		}
		out = listResp{Items: jsonItems, Total: total}
	case []map[string]string:
		out = listResp{Items: v, Total: total}
	case []any:
		out = listResp{Items: v, Total: total}
	default:
		out = listResp{Items: items, Total: total}
	}

	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func (p *jsonPrinter) PrintDetail(item any) error {
	if msg, ok := item.(proto.Message); ok {
		b, err := jsonOpts.Marshal(msg)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(p.w, string(b))
		return err
	}
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(item)
}

func (p *jsonPrinter) PrintError(e *clierr.CLIError) error {
	type errResp struct {
		Error struct {
			Code       string         `json:"code"`
			Message    string         `json:"message"`
			HTTPStatus int            `json:"http_status"`
			Hint       string         `json:"hint,omitempty"`
			Metadata   map[string]any `json:"metadata"`
		} `json:"error"`
	}
	resp := errResp{}
	resp.Error.Code = e.Code
	resp.Error.Message = e.Message
	resp.Error.HTTPStatus = e.HTTPStatus
	resp.Error.Hint = e.Hint
	resp.Error.Metadata = e.Metadata

	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func (p *jsonPrinter) PrintSuccess(message string, kv ...string) error {
	m := map[string]any{"message": message}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func (p *jsonPrinter) PrintKeyValue(pairs ...string) error {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func printQuietJSON(w io.Writer, items any) error {
	// Just print IDs one per line as a JSON array element.
	type idItem struct {
		ID string `json:"id"`
	}
	switch v := items.(type) {
	case []map[string]string:
		for _, item := range v {
			id := item["id"]
			if id == "" {
				id = item["Id"]
			}
			b, _ := json.Marshal(idItem{ID: id})
			fmt.Fprintln(w, string(b))
		}
	default:
		// Fallback.
		enc := json.NewEncoder(w)
		return enc.Encode(items)
	}
	return nil
}
