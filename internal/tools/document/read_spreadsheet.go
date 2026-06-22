package document

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aranea-agents/pkg/apierror"
	"github.com/xuri/excelize/v2"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	toolReadSpreadsheet     = "read_spreadsheet"
	defaultReadSheetChars   = 4000
	defaultSheetPreviewRows = 20
	maxSpreadsheetRows      = 10000

	sheetKindXLSX = "xlsx"
	sheetKindCSV  = "csv"
)

type readSpreadsheetInput struct {
	Path     string `json:"path"`
	Sheet    string `json:"sheet,omitempty"`
	Row      *int   `json:"row,omitempty"`
	StartRow *int   `json:"start_row,omitempty"`
	EndRow   *int   `json:"end_row,omitempty"`
	MaxChars *int   `json:"max_chars,omitempty"`
}

type spreadsheetRow struct {
	Index  int      `json:"index"`
	Values []string `json:"values,omitempty"`
}

type readSpreadsheetOutput struct {
	Path      string           `json:"path"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Sheet     string           `json:"sheet,omitempty"`
	StartRow  int              `json:"start_row,omitempty"`
	EndRow    int              `json:"end_row,omitempty"`
	RowCount  int              `json:"row_count,omitempty"`
	Rows      []spreadsheetRow `json:"rows,omitempty"`
	Text      string           `json:"text"`
	Truncated bool             `json:"truncated,omitempty"`
}

func NewReadSpreadsheetTool(baseDir string) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in readSpreadsheetInput) (readSpreadsheetOutput, error) {
			if strings.TrimSpace(in.Path) == "" {
				return readSpreadsheetOutput{}, apierror.BadRequest(apierror.DomainTool, "path is required")
			}

			path, err := ValidatePath(strings.TrimSpace(in.Path), baseDir)
			if err != nil {
				return readSpreadsheetOutput{}, apierror.BadRequest(apierror.DomainTool, err.Error())
			}

			kind := spreadsheetKindFromPath(path)
			if kind == "" {
				return readSpreadsheetOutput{}, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("unsupported spreadsheet type: %s", filepath.Ext(path)))
			}

			if err := ValidateFileSize(path); err != nil {
				return readSpreadsheetOutput{}, apierror.BadRequest(apierror.DomainTool, err.Error())
			}

			info, err := os.Stat(path)
			if err != nil {
				return readSpreadsheetOutput{}, apierror.Internal(apierror.DomainTool, "stat path: "+err.Error())
			}
			if info.IsDir() {
				return readSpreadsheetOutput{}, apierror.BadRequest(apierror.DomainTool, "path is a directory: "+path)
			}

			rows, sheetName, err := readSpreadsheetRows(path, kind, in.Sheet)
			if err != nil {
				return readSpreadsheetOutput{}, err
			}

			selected, startRow, endRow, err := selectSpreadsheetRows(rows, in)
			if err != nil {
				return readSpreadsheetOutput{}, err
			}

			text := formatSpreadsheetRows(selected)
			maxChars := defaultReadSheetChars
			if in.MaxChars != nil && *in.MaxChars > 0 {
				maxChars = *in.MaxChars
			}
			text, truncated := truncateText(text, maxChars)

			return readSpreadsheetOutput{
				Path:      path,
				Kind:      kind,
				Title:     filepath.Base(path),
				Sheet:     sheetName,
				StartRow:  startRow,
				EndRow:    endRow,
				RowCount:  len(rows),
				Rows:      selected,
				Text:      text,
				Truncated: truncated,
			}, nil
		},
		trpcfunction.WithName(toolReadSpreadsheet),
		trpcfunction.WithDescription(
			"Read tabular files such as XLSX and CSV. "+
				"Use this instead of exec_command when the user asks for rows, sheets, or table excerpts.",
		),
		trpcfunction.WithInputSchema(&trpctool.Schema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]*trpctool.Schema{
				"path": {
					Type:        "string",
					Description: "Spreadsheet file path.",
				},
				"sheet": {
					Type:        "string",
					Description: "Optional worksheet name. Defaults to the first sheet.",
				},
				"row": {
					Type:        "integer",
					Description: "Optional 1-based row number to read.",
				},
				"start_row": {
					Type:        "integer",
					Description: "Optional 1-based range start row.",
				},
				"end_row": {
					Type:        "integer",
					Description: "Optional 1-based range end row.",
				},
				"max_chars": {
					Type:        "integer",
					Description: "Optional maximum characters to return.",
				},
			},
		}),
		trpcfunction.WithOutputSchema(&trpctool.Schema{
			Type:     "object",
			Required: []string{"path", "kind", "text"},
			Properties: map[string]*trpctool.Schema{
				"path":      {Type: "string", Description: "Resolved file path."},
				"kind":      {Type: "string", Description: "Spreadsheet kind (xlsx, csv)."},
				"title":     {Type: "string", Description: "File name."},
				"sheet":     {Type: "string", Description: "Selected worksheet name (XLSX only)."},
				"start_row": {Type: "integer", Description: "First row returned (1-based)."},
				"end_row":   {Type: "integer", Description: "Last row returned (1-based)."},
				"row_count": {Type: "integer", Description: "Total rows in sheet."},
				"rows":      {Type: "array", Description: "Selected row data."},
				"text":      {Type: "string", Description: "Formatted text representation."},
				"truncated": {Type: "boolean", Description: "Whether output was truncated."},
			},
		}),
	)
}

func spreadsheetKindFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xls", ".xlsm":
		return sheetKindXLSX
	case ".csv":
		return sheetKindCSV
	default:
		return ""
	}
}

func readSpreadsheetRows(path string, kind string, sheet string) ([][]string, string, error) {
	switch kind {
	case sheetKindCSV:
		rows, err := readCSVRows(path)
		return rows, "", err
	case sheetKindXLSX:
		return readWorkbookRows(path, sheet)
	default:
		return nil, "", apierror.BadRequest(apierror.DomainTool, "unsupported spreadsheet kind: "+kind)
	}
}

func readCSVRows(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainTool, "open csv: "+err.Error())
	}
	defer file.Close()

	reader := csv.NewReader(file)
	var rows [][]string
	for count := 0; count < maxSpreadsheetRows; count++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, apierror.Internal(apierror.DomainTool, "read csv: "+err.Error())
		}
		rows = append(rows, record)
	}
	return rows, nil
}

func readWorkbookRows(path string, sheet string) ([][]string, string, error) {
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		return nil, "", apierror.Internal(apierror.DomainTool, "open spreadsheet: "+err.Error())
	}
	defer func() { _ = workbook.Close() }()

	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, "", apierror.BadRequest(apierror.DomainTool, "spreadsheet has no sheets")
	}

	selected := strings.TrimSpace(sheet)
	if selected == "" {
		selected = sheets[0]
	}

	rows, err := workbook.GetRows(selected)
	if err != nil {
		return nil, "", apierror.Internal(apierror.DomainTool, fmt.Sprintf("read sheet %q: %s", selected, err.Error()))
	}
	if len(rows) > maxSpreadsheetRows {
		rows = rows[:maxSpreadsheetRows]
	}
	return rows, selected, nil
}

func selectSpreadsheetRows(rows [][]string, in readSpreadsheetInput) ([]spreadsheetRow, int, int, error) {
	totalRows := len(rows)
	if totalRows == 0 {
		return nil, 0, 0, nil
	}

	startRow, endRow, err := spreadsheetRange(totalRows, in)
	if err != nil {
		return nil, 0, 0, err
	}

	selected := make([]spreadsheetRow, 0, endRow-startRow+1)
	for rowIndex := startRow; rowIndex <= endRow; rowIndex++ {
		values := sanitizeSpreadsheetCells(rows[rowIndex-1])
		selected = append(selected, spreadsheetRow{
			Index:  rowIndex,
			Values: values,
		})
	}
	return selected, startRow, endRow, nil
}

func spreadsheetRange(totalRows int, in readSpreadsheetInput) (int, int, error) {
	row := normalizedPositive(in.Row)
	if row != nil {
		if *row > totalRows {
			return 0, 0, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("row %d exceeds row count %d", *row, totalRows))
		}
		return *row, *row, nil
	}

	start := 1
	if v := normalizedPositive(in.StartRow); v != nil {
		start = *v
	}
	if start > totalRows {
		return 0, 0, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("start_row %d exceeds row count %d", start, totalRows))
	}

	end := minInt(totalRows, defaultSheetPreviewRows)
	if start > 1 {
		end = start
	}
	if v := normalizedPositive(in.EndRow); v != nil {
		end = *v
	}
	if end < start {
		return 0, 0, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("end_row %d is smaller than start_row %d", end, start))
	}
	if end > totalRows {
		end = totalRows
	}
	if v := normalizedPositive(in.StartRow); v != nil && normalizedPositive(in.EndRow) == nil {
		end = minInt(totalRows, start+defaultSheetPreviewRows-1)
	}
	return start, end, nil
}

func sanitizeSpreadsheetCells(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		sanitized := strings.ReplaceAll(value, "\n", " ")
		sanitized = strings.TrimSpace(sanitized)
		out = append(out, sanitized)
	}
	return out
}

func formatSpreadsheetRows(rows []spreadsheetRow) string {
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, formatSpreadsheetRow(row))
	}
	return strings.Join(lines, "\n")
}

func formatSpreadsheetRow(row spreadsheetRow) string {
	return "row " + strconv.Itoa(row.Index) + ": " + strings.Join(row.Values, "\t")
}
