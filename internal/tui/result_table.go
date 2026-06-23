package tui

import (
	"fmt"
	"strings"

	"github.com/farhank15/dbTui/internal/model"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ResultTable displays query results in a table
type ResultTable struct {
	*tview.Table
	app             *App
	result          *model.QueryResult
	exportPath      string
	detail          *model.TableDetail // stored for column filtering
	columnFilter    string             // current column filter text
	rowSearchColumn string             // column name to filter rows by
	rowSearchQuery  string             // row search query text
}

// NewResultTable creates a new result table
func NewResultTable(app *App) *ResultTable {
	rt := &ResultTable{
		Table: tview.NewTable(),
		app:   app,
	}

	rt.Table.
		SetBorders(true).
		SetSelectable(true, true).
		SetFixed(1, 0)

	rt.Table.SetBorder(true).
		SetTitle(" Results ").
		SetBorderColor(Styles.Border)

	rt.Table.SetSelectedStyle(tcell.StyleDefault.
		Background(Styles.Selection).
		Foreground(Styles.Text))

	// Visual focus indicator: change border color when focused
	rt.Table.SetFocusFunc(func() {
		rt.Table.SetBorderColor(Styles.BorderFocus)
	})
	rt.Table.SetBlurFunc(func() {
		rt.Table.SetBorderColor(Styles.Border)
	})

	rt.Table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			if event.Modifiers() == tcell.ModNone {
				app.app.SetFocus(app.sidebar.GetTreeView())
				return nil
			}
		case tcell.KeyCtrlE:
			app.ExportResults()
			return nil
		case tcell.KeyEnter:
			row, col := rt.Table.GetSelection()
			if rt.result != nil && row > 0 && row <= len(rt.result.Rows) && col >= 0 && col < len(rt.result.Columns) {
				rt.InspectCell(row, col)
				return nil
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case '/':
				if app.resultTable.detail != nil {
					app.dialogs.ShowInputDialog("Filter Columns", "Column name...", func(text string) {
						app.resultTable.FilterColumns(text)
					})
					return nil
				} else if app.resultTable.result != nil {
					app.ShowSearchRowsDialog()
					return nil
				}
			case 'y', 'Y':
				row, col := rt.Table.GetSelection()
				cell := rt.Table.GetCell(row, col)
				if cell != nil && cell.Text != "" {
					if err := writeToClipboard(cell.Text); err != nil {
						app.statusBar.ShowError(fmt.Sprintf("Failed to copy cell: %v", err))
					} else {
						app.statusBar.ShowSuccess(fmt.Sprintf("Copied cell value to clipboard!"))
					}
					return nil
				}
			case 'e', 'E':
				row, col := rt.Table.GetSelection()
				if rt.result != nil && row > 0 && row <= len(rt.result.Rows) && col >= 0 && col < len(rt.result.Columns) {
					rt.EditCell(row, col)
					return nil
				}
			case 'r', 'R':
				app.RefreshActiveQuery()
				return nil
			}
		}
		return event
	})

	return rt
}

// FilterColumns filters the displayed columns by name
func (rt *ResultTable) FilterColumns(filter string) {
	rt.columnFilter = strings.TrimSpace(filter)
	if rt.detail != nil {
		rt.DisplayTableDetail(rt.detail)
	}
}

// DisplayResult renders query results in the table
func (rt *ResultTable) DisplayResult(result *model.QueryResult) {
	rt.Clear()
	rt.result = result
	rt.detail = nil

	if result == nil {
		rt.SetTitle(" Results ")
		return
	}

	// Show error
	if result.Error != "" {
		rt.SetTitle(fmt.Sprintf(" Results - ERROR "))

		errorCell := tview.NewTableCell(result.Error).
			SetTextColor(Styles.Error).
			SetSelectable(false)
		rt.SetCell(0, 0, errorCell)
		return
	}

	// Find column index to filter if specified
	colIdxToFilter := -1
	if rt.rowSearchColumn != "" {
		for i, colName := range result.Columns {
			if strings.ToLower(colName) == strings.ToLower(rt.rowSearchColumn) {
				colIdxToFilter = i
				break
			}
		}
	}

	// Render headers
	for colIdx, colName := range result.Columns {
		headerCell := tview.NewTableCell(colName).
			SetTextColor(Styles.Accent).
			SetBackgroundColor(Styles.Surface).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1).
			SetMaxWidth(30)
		rt.SetCell(0, colIdx, headerCell)
	}

	// Filter and render data rows
	renderedRowIdx := 0
	for _, row := range result.Rows {
		// Apply row filtering
		match := true
		if rt.rowSearchQuery != "" {
			match = false
			if colIdxToFilter >= 0 && colIdxToFilter < len(row) {
				if strings.Contains(strings.ToLower(row[colIdxToFilter]), strings.ToLower(rt.rowSearchQuery)) {
					match = true
				}
			} else {
				// Search all columns
				for _, cellValue := range row {
					if strings.Contains(strings.ToLower(cellValue), strings.ToLower(rt.rowSearchQuery)) {
						match = true
						break
					}
				}
			}
		}

		if !match {
			continue
		}

		for colIdx, cellValue := range row {
			color := Styles.Text
			if cellValue == "NULL" {
				color = Styles.TextSecondary
				cellValue = "NULL"
			} else {
				// Sanitize control characters and newlines to prevent TUI display corruption
				cellValue = strings.ReplaceAll(cellValue, "\r\n", " ")
				cellValue = strings.ReplaceAll(cellValue, "\n", " ")
				cellValue = strings.ReplaceAll(cellValue, "\r", " ")
				cellValue = strings.ReplaceAll(cellValue, "\t", "    ")
			}

			cell := tview.NewTableCell(cellValue).
				SetTextColor(color).
				SetExpansion(1).
				SetMaxWidth(30)

			// Alternate row colors
			if renderedRowIdx%2 == 0 {
				cell.SetBackgroundColor(tcell.ColorDefault)
			} else {
				cell.SetBackgroundColor(Styles.InputBg)
			}

			rt.SetCell(renderedRowIdx+1, colIdx, cell)
		}
		renderedRowIdx++
	}

	// Set title with info
	title := fmt.Sprintf(" Results (%d cols x %d/%d rows) ", result.ColCount(), renderedRowIdx, result.RowCount())
	if rt.rowSearchQuery != "" {
		if rt.rowSearchColumn != "" {
			title += fmt.Sprintf("[filtered: col %s = *%s*] ", rt.rowSearchColumn, rt.rowSearchQuery)
		} else {
			title += fmt.Sprintf("[filtered: *%s*] ", rt.rowSearchQuery)
		}
	}
	if result.Duration != "" {
		title += fmt.Sprintf("[%s] ", result.Duration)
	}
	rt.SetTitle(title)

	// Scroll to top
	rt.ScrollToBeginning()
}

// FilterRows filters the displayed data rows by column value
func (rt *ResultTable) FilterRows(columnName, query string) {
	rt.rowSearchColumn = strings.TrimSpace(columnName)
	rt.rowSearchQuery = strings.TrimSpace(query)
	if rt.result != nil {
		rt.DisplayResult(rt.result)
	}
}

// DisplayTableDetail renders table detail information
func (rt *ResultTable) DisplayTableDetail(detail *model.TableDetail) {
	rt.Clear()
	rt.result = nil
	rt.detail = detail

	if detail == nil {
		rt.SetTitle(" Table Detail ")
		return
	}

	title := fmt.Sprintf(" 📋 %s (%d rows) ", detail.Table.Name, detail.Table.RowCount)
	if rt.columnFilter != "" {
		title += fmt.Sprintf("[filter: %s]", rt.columnFilter)
	}
	rt.SetTitle(title)

	// Columns section
	headers := []string{"Column", "Type", "Nullable", "Key", "Default", "Extra"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetTextColor(Styles.Accent).
			SetBackgroundColor(Styles.Surface).
			SetAlign(tview.AlignCenter).
			SetSelectable(false).
			SetExpansion(1)
		rt.SetCell(0, i, cell)
	}

	rowIdx := 1
	for _, col := range detail.Table.Columns {
		// Apply column filter
		if rt.columnFilter != "" && !strings.Contains(strings.ToLower(col.Name), strings.ToLower(rt.columnFilter)) {
			continue
		}
		rowData := []string{col.Name, col.Type, col.Nullable, col.Key, col.Default, col.Extra}
		for j, val := range rowData {
			cell := tview.NewTableCell(val).
				SetTextColor(Styles.Text).
				SetExpansion(1)
			if (rowIdx-1)%2 == 0 {
				cell.SetBackgroundColor(tcell.ColorDefault)
			} else {
				cell.SetBackgroundColor(Styles.InputBg)
			}
			rt.SetCell(rowIdx, j, cell)
		}
		rowIdx++
	}

	// Indexes section
	if len(detail.Indexes) > 0 {
		idxStart := len(detail.Table.Columns) + 3
		rt.SetCell(idxStart-1, 0, tview.NewTableCell("").
			SetSelectable(false))
		rt.SetCell(idxStart, 0, tview.NewTableCell(fmt.Sprintf("📌 INDEXES (%d)", len(detail.Indexes))).
			SetTextColor(Styles.Warning).
			SetSelectable(false).
			SetExpansion(1))

		idxHeaders := []string{"Name", "Columns", "Unique", "Primary"}
		for i, h := range idxHeaders {
			cell := tview.NewTableCell(h).
				SetTextColor(Styles.Accent).
				SetBackgroundColor(Styles.Surface).
				SetAlign(tview.AlignCenter).
				SetSelectable(false).
				SetExpansion(1)
			rt.SetCell(idxStart+1, i, cell)
		}

		for i, idx := range detail.Indexes {
			rowData := []string{idx.Name, strings.Join(idx.Columns, ", "),
				boolToStr(idx.Unique), boolToStr(idx.Primary)}
			for j, val := range rowData {
				cell := tview.NewTableCell(val).
					SetTextColor(Styles.Text).
					SetExpansion(1)
				rt.SetCell(idxStart+2+i, j, cell)
			}
		}
	}

	// Foreign Keys section
	if len(detail.ForeignKeys) > 0 {
		fkStart := len(detail.Table.Columns) + 3
		if len(detail.Indexes) > 0 {
			fkStart += len(detail.Indexes) + 3
		}

		rt.SetCell(fkStart-1, 0, tview.NewTableCell("").SetSelectable(false))
		rt.SetCell(fkStart, 0, tview.NewTableCell(fmt.Sprintf("🔗 FOREIGN KEYS (%d)", len(detail.ForeignKeys))).
			SetTextColor(Styles.Warning).
			SetSelectable(false).
			SetExpansion(1))

		fkHeaders := []string{"Name", "Column", "Ref Table", "Ref Column"}
		for i, h := range fkHeaders {
			cell := tview.NewTableCell(h).
				SetTextColor(Styles.Accent).
				SetBackgroundColor(Styles.Surface).
				SetAlign(tview.AlignCenter).
				SetSelectable(false).
				SetExpansion(1)
			rt.SetCell(fkStart+1, i, cell)
		}

		for i, fk := range detail.ForeignKeys {
			rowData := []string{fk.Name, fk.Column, fk.RefTable, fk.RefColumn}
			for j, val := range rowData {
				cell := tview.NewTableCell(val).
					SetTextColor(Styles.Text).
					SetExpansion(1)
				rt.SetCell(fkStart+2+i, j, cell)
			}
		}
	}

	rt.ScrollToBeginning()
}

// ExportToCSV returns the current result as CSV
func (rt *ResultTable) ExportToCSV() string {
	if rt.result == nil {
		return ""
	}
	return rt.result.CSV()
}

func boolToStr(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// EditCell triggers inline cell editing for a specific cell in the grid
func (rt *ResultTable) EditCell(row, col int) {
	result := rt.result
	if result == nil || result.Table == "" || result.ConnID == "" || result.Database == "" {
		rt.app.statusBar.ShowError("Cannot edit cell: Table context not available")
		return
	}

	colName := result.Columns[col]
	currentVal := result.Rows[row-1][col]

	connector, err := rt.app.dbManager.GetConnector(result.ConnID)
	if err != nil {
		rt.app.statusBar.ShowError(fmt.Sprintf("Error: %v", err))
		return
	}

	rt.app.statusBar.ShowInfo("Fetching table metadata for editing...")
	go func() {
		detail, err := connector.GetTableDetail(result.Database, result.Table)
		if err != nil {
			rt.app.app.QueueUpdateDraw(func() {
				rt.app.statusBar.ShowError(fmt.Sprintf("Failed to fetch table details: %v", err))
			})
			return
		}

		rt.app.app.QueueUpdateDraw(func() {
			rt.app.statusBar.ShowInfo("Ready to edit cell")

			// Find primary key columns
			var pkCols []string
			for _, idx := range detail.Indexes {
				if idx.Primary {
					pkCols = idx.Columns
					break
				}
			}

			// Fallback: If no primary key, check if there's an 'id' or 'ID' column
			if len(pkCols) == 0 {
				for _, c := range detail.Table.Columns {
					if strings.ToLower(c.Name) == "id" {
						pkCols = []string{c.Name}
						break
					}
				}
			}

			// Construct WHERE clause
			var whereClause string
			var whereArgs []interface{}
			
			connConfig := rt.app.config.GetConnectionByID(result.ConnID)
			quote := func(name string) string {
				if connConfig != nil && connConfig.Type == model.TypeMySQL {
					return fmt.Sprintf("`%s`", name)
				}
				return fmt.Sprintf("\"%s\"", name)
			}

			if len(pkCols) > 0 {
				var parts []string
				for _, pkCol := range pkCols {
					pkColIdx := -1
					for idx, name := range result.Columns {
						if strings.ToLower(name) == strings.ToLower(pkCol) {
							pkColIdx = idx
							break
						}
					}
					if pkColIdx != -1 {
						val := result.Rows[row-1][pkColIdx]
						parts = append(parts, fmt.Sprintf("%s = ?", quote(pkCol)))
						whereArgs = append(whereArgs, val)
					}
				}
				whereClause = strings.Join(parts, " AND ")
			}

			if whereClause == "" {
				var parts []string
				for idx, name := range result.Columns {
					val := result.Rows[row-1][idx]
					if val == "NULL" {
						parts = append(parts, fmt.Sprintf("%s IS NULL", quote(name)))
					} else {
						parts = append(parts, fmt.Sprintf("%s = ?", quote(name)))
						whereArgs = append(whereArgs, val)
					}
				}
				whereClause = strings.Join(parts, " AND ")
			}

			rt.app.dialogs.ShowCellEditDialog(result.Database, result.Table, colName, currentVal, whereClause, whereArgs, connector, func(newVal string) {
				cell := rt.GetCell(row, col)
				if cell != nil {
					cell.SetText(newVal)
					if newVal == "NULL" {
						cell.SetTextColor(Styles.TextSecondary)
					} else {
						cell.SetTextColor(Styles.Text)
					}
				}
				result.Rows[row-1][col] = newVal
				rt.app.statusBar.ShowSuccess("Cell updated successfully!")
			})
		})
	}()
}

// InspectCell shows a modal dialog with the full content of the selected cell
func (rt *ResultTable) InspectCell(row, col int) {
	result := rt.result
	if result == nil || row <= 0 || row > len(result.Rows) || col < 0 || col >= len(result.Columns) {
		return
	}

	colName := result.Columns[col]
	cellValue := result.Rows[row-1][col]

	rt.app.dialogs.ShowCellInspectDialog(result.Table, colName, cellValue)
}
