package ui

import (
	"fmt"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// Table renders a text table to the UI's Out stream.
func (u UI) Table(headers []string, rows [][]string) {
	if !u.IsTTY {
		// Non-TTY: plain text rows.
		for _, row := range rows {
			fmt.Fprintln(u.Out, strings.Join(row, "\t"))
		}
		return
	}

	tbl := tablewriter.NewTable(u.Out,
		tablewriter.WithBorders(tw.Border{Left: tw.Off, Top: tw.Off, Right: tw.Off, Bottom: tw.Off}),
	)

	// Set header.
	hdrs := make([]any, len(headers))
	for i, h := range headers {
		hdrs[i] = h
	}
	tbl.Header(hdrs...)

	// Append rows.
	for _, row := range rows {
		cells := make([]any, len(row))
		for i, c := range row {
			cells[i] = c
		}
		_ = tbl.Append(cells...)
	}
	_ = tbl.Render()
}
