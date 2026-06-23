package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/farhank15/dbTui/internal/db"
	"github.com/farhank15/dbTui/internal/model"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Dialogs manages modal dialogs for connections and table operations
type Dialogs struct {
	app *App
}

// NewDialogs creates a new dialogs manager
func NewDialogs(app *App) *Dialogs {
	return &Dialogs{app: app}
}

// ShowConnectionDialog shows a form to add/edit a connection
func (d *Dialogs) ShowConnectionDialog(conn *model.Connection) {
	form := tview.NewForm()
	isNew := conn == nil
	editing := conn

	if isNew {
		conn = &model.Connection{
			Type: model.TypePostgres,
			Port: 5432,
			Host: "localhost",
		}
	}

	// Connection name
	form.AddInputField("Name", conn.Name, 30, nil, func(text string) {
		conn.Name = text
	})

	// Connection type
	dbTypes := []string{"PostgreSQL", "MySQL", "SQLite"}
	currentType := 0
	switch conn.Type {
	case model.TypePostgres:
		currentType = 0
	case model.TypeMySQL:
		currentType = 1
	case model.TypeSQLite:
		currentType = 2
	}

	form.AddDropDown("Type", dbTypes, currentType, func(option string, index int) {
		switch index {
		case 0:
			conn.Type = model.TypePostgres
			conn.Port = 5432
		case 1:
			conn.Type = model.TypeMySQL
			conn.Port = 3306
		case 2:
			conn.Type = model.TypeSQLite
			conn.Port = 0
		}
	})

	// Host
	form.AddInputField("Host", conn.Host, 30, nil, func(text string) {
		conn.Host = text
	})

	// Port
	portStr := fmt.Sprintf("%d", conn.Port)
	form.AddInputField("Port", portStr, 6, nil, func(text string) {
		fmt.Sscanf(text, "%d", &conn.Port)
	})

	// Username
	form.AddInputField("User", conn.User, 30, nil, func(text string) {
		conn.User = text
	})

	form.AddPasswordField("Password", conn.Password, 30, '*', func(text string) {
		conn.Password = text
	})

	form.AddInputField("Database", conn.Database, 30, nil, func(text string) {
		conn.Database = text
	})

	// SQLite file path
	form.AddInputField("SQLite File", conn.File, 50, nil, func(text string) {
		conn.File = text
		conn.Database = text
	})

	// SSL mode for PostgreSQL
	sslOptions := []string{"disable", "require", "verify-ca", "verify-full"}
	currentSSL := 0
	for i, s := range sslOptions {
		if s == conn.SSLMode {
			currentSSL = i
			break
		}
	}
	form.AddDropDown("SSL Mode", sslOptions, currentSSL, func(option string, index int) {
		conn.SSLMode = option
	})

	// Buttons
	saveText := "Connect"
	if editing != nil {
		saveText = "Save && Connect"
	}

	form.AddButton(saveText, func() {
		if isNew {
			conn.ID = d.app.config.GenerateUniqueID()
		}

		// Clean up
		if conn.Type == model.TypeSQLite && conn.File != "" {
			conn.Database = conn.File
		}

		d.app.closeDialog()

		// Save config
		if isNew {
			d.app.config.AddConnection(*conn)
		} else {
			d.app.config.UpdateConnection(editing.ID, *conn)
		}

		// Connect
		connectConn := conn
		if editing != nil {
			connectConn = editing
			connectConn.Host = conn.Host
			connectConn.Port = conn.Port
			connectConn.User = conn.User
			connectConn.Password = conn.Password
			connectConn.Database = conn.Database
		}
		d.app.ConnectTo(connectConn)
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
		editing = nil
	})

	form.SetBorder(true).SetTitle(" Database Connection ").SetTitleAlign(tview.AlignLeft)
	form.SetButtonsAlign(tview.AlignCenter)

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 0, 3, true).
			AddItem(nil, 0, 1, false),
			0, 2, true).
		AddItem(nil, 0, 1, false)

	d.app.showDialog(flex)
}

// ShowCreateTableDialog shows a form to create a new table
func (d *Dialogs) ShowCreateTableDialog(connID, dbName string) {
	form := tview.NewForm()
	tableName := ""
	var columns []model.ColumnDef

	form.AddInputField("Table Name", "", 30, nil, func(text string) {
		tableName = text
	})

	columnsLabel := tview.NewTextView().SetText("Add columns below (edit & confirm):")
	form.AddFormItem(columnsLabel)

	// Start with one column
	addColumnRow(form, &columns, 0)

	form.AddButton("+ Add Column", func() {
		idx := len(columns)
		addColumnRow(form, &columns, idx)
	})

	form.AddButton("Create Table", func() {
		if tableName == "" || len(columns) == 0 {
			d.app.statusBar.ShowError("Table name and at least 1 column required")
			return
		}

		d.app.closeDialog()
		go func() {
			conn, err := d.app.dbManager.GetConnector(connID)
			if err != nil {
				d.app.app.QueueUpdateDraw(func() {
					d.app.statusBar.ShowError(fmt.Sprintf("Error: %v", err))
				})
				return
			}

			if err := conn.CreateTable(dbName, tableName, columns); err != nil {
				d.app.app.QueueUpdateDraw(func() {
					d.app.statusBar.ShowError(fmt.Sprintf("Failed to create table: %v", err))
				})
				return
			}

			d.app.app.QueueUpdateDraw(func() {
				d.app.statusBar.ShowSuccess(fmt.Sprintf("Table %s created!", tableName))
				d.app.sidebar.RefreshConnections()
			})
		}()
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})

	form.SetBorder(true).SetTitle(" Create Table ").SetTitleAlign(tview.AlignLeft)

	d.app.showDialog(form)
}

func addColumnRow(form *tview.Form, columns *[]model.ColumnDef, idx int) {
	prefix := fmt.Sprintf("Col%d", idx+1)

	nameInput := tview.NewInputField()
	nameInput.SetLabel(fmt.Sprintf("%s Name", prefix))
	nameInput.SetFieldWidth(15)

	typeOptions := []string{"INTEGER", "VARCHAR", "TEXT", "BOOLEAN", "BIGINT", "FLOAT", "DECIMAL", "DATE", "TIMESTAMP", "BLOB"}
	typeDropdown := tview.NewDropDown()
	typeDropdown.SetLabel(fmt.Sprintf("%s Type", prefix))
	typeDropdown.SetOptions(typeOptions, nil)
	typeDropdown.SetCurrentOption(0)

	nullableCheckbox := tview.NewCheckbox()
	nullableCheckbox.SetLabel(fmt.Sprintf("%s Null?", prefix))

	pkCheckbox := tview.NewCheckbox()
	pkCheckbox.SetLabel(fmt.Sprintf("%s PK?", prefix))

	autoincCheckbox := tview.NewCheckbox()
	autoincCheckbox.SetLabel(fmt.Sprintf("%s AutoInc?", prefix))

	*columns = append(*columns, model.ColumnDef{
		Type: "INTEGER",
	})
	colIdx := len(*columns) - 1

	nameInput.SetChangedFunc(func(text string) {
		(*columns)[colIdx].Name = text
	})
	typeDropdown.SetSelectedFunc(func(option string, index int) {
		(*columns)[colIdx].Type = option
	})
	nullableCheckbox.SetChangedFunc(func(checked bool) {
		(*columns)[colIdx].Nullable = checked
	})
	pkCheckbox.SetChangedFunc(func(checked bool) {
		(*columns)[colIdx].PrimaryKey = checked
	})
	autoincCheckbox.SetChangedFunc(func(checked bool) {
		(*columns)[colIdx].AutoInc = checked
	})

	form.AddFormItem(nameInput)
	form.AddFormItem(typeDropdown)
	form.AddFormItem(nullableCheckbox)
	form.AddFormItem(pkCheckbox)
	form.AddFormItem(autoincCheckbox)
}

// ShowSearchDataDialog shows a form to search data within a table column
func (d *Dialogs) ShowSearchDataDialog(ref *sidebarRef) {
	if ref == nil || ref.kind != "table" {
		d.app.statusBar.ShowError("Select a table first")
		return
	}

	form := tview.NewForm()
	columnName := ""
	searchValue := ""

	// Table name (read-only)
	form.AddTextView("Table", fmt.Sprintf("%s.%s", ref.db, ref.table), 30, 1, false, false)

	// Column name input
	form.AddInputField("Column", columnName, 30, nil, func(text string) {
		columnName = text
	})

	// Search value input
	form.AddInputField("Search Value", searchValue, 30, nil, func(text string) {
		searchValue = text
	})

	form.AddButton("🔍 Search", func() {
		if columnName == "" || searchValue == "" {
			d.app.statusBar.ShowError("Column and search value required")
			return
		}

		d.app.closeDialog()

		// Build the SELECT LIKE query - use parameterized form in query panel
		// Let user adjust quoting based on their database type
		// Escape single quotes for SQL
		escapedVal := ""
		for _, c := range searchValue {
			if c == '\'' {
				escapedVal += "''"
			} else {
				escapedVal += string(c)
			}
		}

		var quotedColumn string
		quotedTable := d.quoteTableNameWithConn(ref.id, ref.table)
		connConfig := d.app.config.GetConnectionByID(ref.id)
		if connConfig != nil && connConfig.Type == model.TypeMySQL {
			quotedColumn = fmt.Sprintf("`%s`", columnName)
		} else {
			quotedColumn = fmt.Sprintf("\"%s\"", columnName)
		}

		query := fmt.Sprintf("SELECT * FROM %s WHERE %s LIKE '%%%s%%';",
			quotedTable, quotedColumn, escapedVal)

		d.app.queryPanel.SetQueryText(query)
		d.app.ExecuteQuery()
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})

	form.SetBorder(true).SetTitle(fmt.Sprintf(" Search in %s ", ref.table)).SetTitleAlign(tview.AlignLeft)
	d.app.showDialog(form)
}



// ShowTableContextMenu shows context menu actions for a table
func (d *Dialogs) ShowTableContextMenu(connID, dbName, tableName string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Table: %s", tableName)).
		AddButtons([]string{"📋 Select", "🔍 Search Data", "🗑️ Drop", "✏️ Rename", "❌ Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			d.app.closeDialog()

			switch buttonIndex {
			case 0: // Select
				quotedTable := d.quoteTableNameWithConn(connID, tableName)
				d.app.queryPanel.SetQueryText(fmt.Sprintf("SELECT * FROM %s;", quotedTable))
				d.app.ExecuteQuery()
			case 1: // Search Data
				d.ShowSearchDataDialog(&sidebarRef{
					kind:  "table",
					id:    connID,
					db:    dbName,
					table: tableName,
				})
			case 2: // Drop
				d.ShowConfirmDialog(
					fmt.Sprintf("Drop table %s? This cannot be undone!", tableName),
					func() {
						go func() {
							conn, err := d.app.dbManager.GetConnector(connID)
							if err != nil {
								return
							}
							conn.DropTable(dbName, tableName)
							d.app.app.QueueUpdateDraw(func() {
								d.app.statusBar.ShowSuccess(fmt.Sprintf("Table %s dropped!", tableName))
								d.app.sidebar.RefreshConnections()
							})
						}()
					})
			case 3: // Rename
				renameForm := tview.NewForm()
				newName := ""
				renameForm.AddInputField("New Name", tableName, 30, nil, func(text string) {
					newName = text
				})
				renameForm.AddButton("Rename", func() {
					d.app.closeDialog()
					go func() {
						conn, err := d.app.dbManager.GetConnector(connID)
						if err != nil {
							return
						}
						conn.RenameTable(dbName, tableName, newName)
						d.app.app.QueueUpdateDraw(func() {
							d.app.statusBar.ShowSuccess(fmt.Sprintf("Table renamed to %s!", newName))
							d.app.sidebar.RefreshConnections()
						})
					}()
				})
				renameForm.AddButton("Cancel", func() {
					d.app.closeDialog()
				})
				renameForm.SetBorder(true).SetTitle(" Rename Table ")
				d.app.showDialog(renameForm)
			}
		})

	d.app.showDialog(modal)
}

// ShowConfirmDialog shows a confirmation dialog
func (d *Dialogs) ShowConfirmDialog(message string, onConfirm func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"✅ Yes", "❌ No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			d.app.closeDialog()
			if buttonIndex == 0 {
				onConfirm()
			}
		})
	d.app.showDialog(modal)
}

// ShowExportDialog shows export options
func (d *Dialogs) ShowExportDialog() {
	if d.app.resultTable.result == nil {
		d.app.statusBar.ShowError("No results to export")
		return
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Export %d rows x %d cols?", d.app.resultTable.result.RowCount(), d.app.resultTable.result.ColCount())).
		AddButtons([]string{"📄 Export CSV", "❌ Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			d.app.closeDialog()
			if buttonIndex == 0 {
				d.app.ExportResults()
			}
		})
	d.app.showDialog(modal)
}

// ShowHelpDialog shows keyboard shortcuts
func (d *Dialogs) ShowHelpDialog() {
	helpText := "[::b]Keyboard Shortcuts:[::-]\n\n" +
		"  [green]Ctrl+N[::-]    New connection\n" +
		"  [green]Ctrl+D[::-]    Disconnect\n" +
		"  [green]Ctrl+J[::-]    Execute query (Ctrl+Enter)\n" +
		"  [green]Ctrl+E[::-]    Export results to CSV\n" +
		"  [green]Ctrl+L[::-]    Clear query panel\n" +
		"  [green]Ctrl+B[::-]    Toggle sidebar (Show/Hide)\n" +
		"  [green]Ctrl+H[::-]    Toggle SQL editor (Show/Hide)\n" +
		"  [green]F1[::-]        Show this help\n" +
		"  [green]Tab[::-]       Navigate panels\n" +
		"  [green]Esc[::-]       Back / Close dialog\n" +
		"  [green]F5[::-]        Refresh\n\n" +
		"[::b]Sidebar:[::-]\n" +
		"  [green]/[::-]          Search/filter tables\n" +
		"  [green]f[::-]          Search data in selected table column\n" +
		"  [green]y[::-]          Copy database or table name to clipboard\n" +
		"  [green]c[::-]          New connection\n" +
		"  [green]r[::-]          Refresh connections\n" +
		"  [green]d[::-]          Disconnect\n" +
		"  [green]Delete[::-]     Delete connection\n\n" +
		"[::b]Table Detail (Results Panel):[::-]\n" +
		"  [green]/[::-]          Filter columns by name\n"

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(helpText).
		SetTextAlign(tview.AlignLeft)

	textView.SetBorder(true).SetTitle(" Help ").SetTitleAlign(tview.AlignLeft)

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(textView, 0, 1, true).
			AddItem(nil, 0, 1, false),
			50, 1, true).
		AddItem(nil, 0, 1, false)

	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			d.app.closeDialog()
			return nil
		}
		return event
	})

	d.app.showDialog(flex)
}

// ShowDatabaseContextMenu shows context menu for a database
func (d *Dialogs) ShowDatabaseContextMenu(connID, dbName string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Database: %s", dbName)).
		AddButtons([]string{"📋 Show Tables", "🗑️ Drop DB", "➕ Create Table", "❌ Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			d.app.closeDialog()
			switch buttonIndex {
			case 0:
				d.app.sidebar.ExpandDatabase(connID, dbName)
			case 1: // Drop database
				d.ShowConfirmDialog(
					fmt.Sprintf("Drop database %s? This cannot be undone!", dbName),
					func() {
						go func() {
							conn, err := d.app.dbManager.GetConnector(connID)
							if err != nil {
								return
							}
							conn.DropDatabase(dbName)
							d.app.app.QueueUpdateDraw(func() {
								d.app.statusBar.ShowSuccess(fmt.Sprintf("Database %s dropped!", dbName))
								d.app.sidebar.RefreshConnections()
							})
						}()
					})
			case 2: // Create table
				d.ShowCreateTableDialog(connID, dbName)
			}
		})
	d.app.showDialog(modal)
}

// ShowInputDialog shows a simple input dialog
func (d *Dialogs) ShowInputDialog(title, label string, callback func(string)) {
	form := tview.NewForm()
	inputValue := ""
	form.AddInputField(label, "", 40, nil, func(text string) {
		inputValue = text
	})
	form.AddButton("OK", func() {
		d.app.closeDialog()
		callback(inputValue)
	})
	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})
	form.SetBorder(true).SetTitle(fmt.Sprintf(" %s ", title))

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 0, 1, true).
			AddItem(nil, 0, 1, false),
			40, 1, true).
		AddItem(nil, 0, 1, false)
	d.app.showDialog(flex)
}

// ShowSearchRowsDialog shows a form to search/filter rows currently in the result table
func (d *Dialogs) ShowSearchRowsDialog() {
	form := tview.NewForm()
	columnName := d.app.resultTable.rowSearchColumn
	searchValue := d.app.resultTable.rowSearchQuery

	form.AddInputField("Column (Optional)", columnName, 30, nil, func(text string) {
		columnName = text
	})

	form.AddInputField("Search Value", searchValue, 30, nil, func(text string) {
		searchValue = text
	})

	form.AddButton("🔍 Filter", func() {
		d.app.closeDialog()
		d.app.resultTable.FilterRows(columnName, searchValue)

		// Auto-generate SQL command in SQL editor
		if searchValue != "" && d.app.resultTable.result != nil {
			currentQuery := d.app.queryPanel.GetQueryText()
			tableName := extractTableName(currentQuery)
			if tableName != "" {
				escapedVal := ""
				for _, c := range searchValue {
					if c == '\'' {
						escapedVal += "''"
					} else {
						escapedVal += string(c)
					}
				}

				quotedTable := d.quoteTableNameWithConn(d.app.activeConn, tableName)
				connConfig := d.app.config.GetConnectionByID(d.app.activeConn)

				var sqlQuery string
				if columnName != "" {
					var quotedColumn string
					if connConfig != nil && connConfig.Type == model.TypeMySQL {
						quotedColumn = fmt.Sprintf("`%s`", columnName)
					} else {
						quotedColumn = fmt.Sprintf("\"%s\"", columnName)
					}
					sqlQuery = fmt.Sprintf("SELECT * FROM %s WHERE %s LIKE '%%%s%%';", quotedTable, quotedColumn, escapedVal)
				} else {
					// Search all columns
					var clauses []string
					for _, colName := range d.app.resultTable.result.Columns {
						var quotedCol string
						if connConfig != nil && connConfig.Type == model.TypeMySQL {
							quotedCol = fmt.Sprintf("`%s`", colName)
						} else {
							quotedCol = fmt.Sprintf("\"%s\"", colName)
						}
						clauses = append(clauses, fmt.Sprintf("%s LIKE '%%%s%%'", quotedCol, escapedVal))
					}
					if len(clauses) > 0 {
						sqlQuery = fmt.Sprintf("SELECT * FROM %s WHERE %s;", quotedTable, strings.Join(clauses, " OR "))
					}
				}

				if sqlQuery != "" {
					d.app.queryPanel.SetQueryText(sqlQuery)
				}
			}
		}
	})

	form.AddButton("Reset", func() {
		d.app.closeDialog()
		d.app.resultTable.FilterRows("", "")
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})

	form.SetBorder(true).SetTitle(" Filter Results ").SetTitleAlign(tview.AlignLeft)

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 0, 1, true).
			AddItem(nil, 0, 1, false),
			45, 1, true).
		AddItem(nil, 0, 1, false)

	d.app.showDialog(flex)
}

// ShowSearchExplorerDialog shows a dialog to search databases and tables in the sidebar
func (d *Dialogs) ShowSearchExplorerDialog(sidebar *Sidebar) {
	form := tview.NewForm()

	query := sidebar.searchQuery
	input := tview.NewInputField().
		SetLabel("Search: ").
		SetText(query).
		SetFieldWidth(40)

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			d.app.closeDialog()
			sidebar.filterTables(input.GetText())
			return nil
		}
		return event
	})

	form.AddFormItem(input)

	form.AddButton("Search", func() {
		d.app.closeDialog()
		sidebar.filterTables(input.GetText())
	})

	form.AddButton("Clear Search", func() {
		d.app.closeDialog()
		sidebar.filterTables("")
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})

	form.SetBorder(true).SetTitle(" Search Explorer ").SetTitleAlign(tview.AlignLeft)
	form.SetButtonsAlign(tview.AlignCenter)

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 9, 0, true).
			AddItem(nil, 0, 1, false),
			50, 0, true).
		AddItem(nil, 0, 1, false)

	d.app.showDialog(flex)
}

func (d *Dialogs) quoteTableNameWithConn(connID, tableName string) string {
	connConfig := d.app.config.GetConnectionByID(connID)
	if connConfig != nil && connConfig.Type == model.TypeMySQL {
		parts := strings.Split(tableName, ".")
		for i, part := range parts {
			parts[i] = fmt.Sprintf("`%s`", part)
		}
		return strings.Join(parts, ".")
	}
	parts := strings.Split(tableName, ".")
	for i, part := range parts {
		parts[i] = fmt.Sprintf("\"%s\"", part)
	}
	return strings.Join(parts, ".")
}

// extractTableName parses the table name from a SELECT query string
func extractTableName(query string) string {
	queryUpper := strings.ToUpper(query)
	fromIdx := strings.Index(queryUpper, " FROM ")
	if fromIdx == -1 {
		return ""
	}

	// Get the part after FROM
	afterFrom := strings.TrimSpace(query[fromIdx+6:])

	// Find the end of the table name (space, semicolon, newline, or LIMIT)
	endIdx := -1
	for i, c := range afterFrom {
		if c == ' ' || c == ';' || c == '\n' || c == '\r' {
			endIdx = i
			break
		}
	}

	tableName := afterFrom
	if endIdx != -1 {
		tableName = afterFrom[:endIdx]
	}

	// Split schema prefix if any (e.g. schema.table -> table)
	if strings.Contains(tableName, ".") {
		parts := strings.Split(tableName, ".")
		tableName = parts[len(parts)-1]
	}

	return strings.Trim(tableName, "\"` ")
}

// ShowSQLTemplatesDialog shows a list of common SQL templates
func (d *Dialogs) ShowSQLTemplatesDialog() {
	list := tview.NewList()
	list.SetBorder(true).SetTitle(" Select SQL Template ")

	templates := []struct {
		name string
		sql  string
	}{
		{"SELECT ALL", "SELECT * FROM table_name LIMIT 100;"},
		{"SELECT JOIN", "SELECT t1.*, t2.*\nFROM table1 t1\nJOIN table2 t2 ON t1.id = t2.ref_id\nLIMIT 100;"},
		{"INSERT INTO", "INSERT INTO table_name (column1, column2)\nVALUES ('value1', 'value2');"},
		{"UPDATE SET", "UPDATE table_name\nSET column1 = 'value1', column2 = 'value2'\nWHERE id = 1;"},
		{"DELETE FROM", "DELETE FROM table_name\nWHERE id = 1;"},
		{"CREATE TABLE", "CREATE TABLE table_name (\n  id SERIAL PRIMARY KEY,\n  name VARCHAR(100) NOT NULL,\n  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP\n);"},
	}

	for _, temp := range templates {
		tName := temp.name
		tSQL := temp.sql
		list.AddItem(tName, strings.ReplaceAll(tSQL, "\n", " "), 0, func() {
			currentText := d.app.queryPanel.GetQueryText()
			if currentText != "" {
				d.app.queryPanel.SetQueryText(currentText + "\n\n" + tSQL)
			} else {
				d.app.queryPanel.SetQueryText(tSQL)
			}
			d.app.closeDialog()
			d.app.FocusQueryPanel()
		})
	}

	list.AddItem("Cancel", "Return without template", 'c', func() {
		d.app.closeDialog()
	})

	d.app.showDialog(list)
}

// ShowTableDDLDialog shows the DDL for the selected table
func (d *Dialogs) ShowTableDDLDialog(connID, dbName, tableName string) {
	d.app.statusBar.ShowInfo(fmt.Sprintf("Generating DDL for %s...", tableName))
	
	go func() {
		ddl, err := d.app.GetTableDDL(connID, dbName, tableName)
		d.app.app.QueueUpdateDraw(func() {
			if err != nil {
				d.app.statusBar.ShowError(fmt.Sprintf("Failed to generate DDL: %v", err))
				return
			}
			d.app.statusBar.ShowSuccess("DDL generated!")

			textView := tview.NewTextView().
				SetDynamicColors(true).
				SetRegions(true).
				SetWordWrap(true).
				SetText(ddl)
			textView.SetBorder(true).SetTitle(fmt.Sprintf(" DDL: %s ", tableName))

			form := tview.NewForm()
			form.AddButton("Copy to Clipboard", func() {
				if err := writeToClipboard(ddl); err != nil {
					d.app.statusBar.ShowError(fmt.Sprintf("Failed to copy: %v", err))
				} else {
					d.app.statusBar.ShowSuccess("Copied DDL to clipboard!")
				}
			})
			form.AddButton("Close", func() {
				d.app.closeDialog()
			})

			flex := tview.NewFlex().
				SetDirection(tview.FlexRow).
				AddItem(textView, 0, 1, true).
				AddItem(form, 5, 0, false)

			d.app.showDialog(flex)
		})
	}()
}

// ShowCellEditDialog shows a dialog to edit a single cell value
func (d *Dialogs) ShowCellEditDialog(dbName, tableName, colName, currentVal, whereClause string, whereArgs []interface{}, connector db.Connector, onSuccess func(string)) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(fmt.Sprintf(" Edit Cell: %s.%s ", tableName, colName))

	var editedVal string = currentVal
	form.AddInputField("Value", currentVal, 40, nil, func(text string) {
		editedVal = text
	})

	var isNull bool = (currentVal == "NULL")
	form.AddCheckbox("Set NULL", isNull, func(checked bool) {
		isNull = checked
	})

	form.AddButton("Save", func() {
		var quotedTable, quotedCol string
		if state := d.app.dbManager.GetConnectionState(d.app.activeConn); state != nil && state.Connection.Type == model.TypeMySQL {
			quotedTable = fmt.Sprintf("`%s`", tableName)
			quotedCol = fmt.Sprintf("`%s`", colName)
		} else {
			quotedTable = fmt.Sprintf("\"%s\"", tableName)
			quotedCol = fmt.Sprintf("\"%s\"", colName)
		}

		var query string
		var args []interface{}
		
		if isNull {
			query = fmt.Sprintf("UPDATE %s SET %s = NULL WHERE %s", quotedTable, quotedCol, whereClause)
			args = whereArgs
		} else {
			query = fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s", quotedTable, quotedCol, whereClause)
			args = append([]interface{}{editedVal}, whereArgs...)
		}

		if state := d.app.dbManager.GetConnectionState(d.app.activeConn); state != nil && state.Connection.Type == model.TypePostgres {
			placeholderIdx := 1
			var pgQuery strings.Builder
			for _, r := range query {
				if r == '?' {
					pgQuery.WriteString(fmt.Sprintf("$%d", placeholderIdx))
					placeholderIdx++
				} else {
					pgQuery.WriteRune(r)
				}
			}
			query = pgQuery.String()
		}

		d.app.statusBar.ShowInfo("Updating cell in database...")
		go func() {
			dbConn := connector.GetDB()
			_, err := dbConn.Exec(query, args...)

			d.app.app.QueueUpdateDraw(func() {
				if err != nil {
					d.app.statusBar.ShowError(fmt.Sprintf("Failed to update cell: %v", err))
				} else {
					d.app.closeDialog()
					if isNull {
						onSuccess("NULL")
					} else {
						onSuccess(editedVal)
					}
				}
			})
		}()
	})

	form.AddButton("Cancel", func() {
		d.app.closeDialog()
	})

	d.app.showDialog(form)
}

// ShowCellInspectDialog shows a scrollable modal dialog with the full value of a cell
func (d *Dialogs) ShowCellInspectDialog(tableName, colName, cellValue string) {
	title := fmt.Sprintf(" View Value: %s.%s ", tableName, colName)
	if tableName == "" {
		title = fmt.Sprintf(" View Value: %s ", colName)
	}

	// Check if cellValue is valid JSON, and pretty-print it
	var jsonObject interface{}
	isJSON := false
	if err := json.Unmarshal([]byte(cellValue), &jsonObject); err == nil {
		formatted, err2 := json.MarshalIndent(jsonObject, "", "  ")
		if err2 == nil {
			cellValue = string(formatted)
			isJSON = true
		}
	}

	displayValue := cellValue
	if isJSON {
		displayValue = colorizeJSON(cellValue)
	} else {
		// Escape standard tview tags in non-JSON text to prevent formatting corruption
		displayValue = strings.ReplaceAll(cellValue, "[", "[[")
	}

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetText(displayValue)
	textView.SetBorder(true).SetTitle(title).SetTitleAlign(tview.AlignLeft)

	form := tview.NewForm()
	form.AddButton("Copy to Clipboard", func() {
		if err := writeToClipboard(cellValue); err != nil {
			d.app.statusBar.ShowError(fmt.Sprintf("Failed to copy: %v", err))
		} else {
			d.app.statusBar.ShowSuccess("Copied value to clipboard!")
		}
	})
	form.AddButton("Close", func() {
		d.app.closeDialog()
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(textView, 0, 1, true).
		AddItem(form, 5, 0, false)

	d.app.showDialog(flex)
}

func colorizeJSON(jsonStr string) string {
	var sb strings.Builder
	inString := false
	isKey := false
	escaped := false

	runes := []rune(jsonStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			if r == '[' {
				sb.WriteString("[[")
			} else {
				sb.WriteRune(r)
			}
			escaped = false
			continue
		}

		if r == '\\' {
			sb.WriteRune(r)
			escaped = true
			continue
		}

		if r == '"' {
			inString = !inString
			if inString {
				isKey = checkIsKey(runes, i)
				if isKey {
					sb.WriteString("[lightblue]\"")
				} else {
					sb.WriteString("[green]\"")
				}
			} else {
				sb.WriteString("\"[-]")
			}
			continue
		}

		if inString {
			if r == '[' {
				sb.WriteString("[[")
			} else {
				sb.WriteRune(r)
			}
			continue
		}

		switch r {
		case '{', '}', '[', ']', ':', ',':
			var charStr string
			if r == '[' {
				charStr = "[["
			} else {
				charStr = string(r)
			}
			sb.WriteString(fmt.Sprintf("[white]%s[-]", charStr))
		case 't', 'f', 'n': // true, false, null
			word := ""
			for j := i; j < len(runes) && runes[j] >= 'a' && runes[j] <= 'z'; j++ {
				word += string(runes[j])
			}
			if word == "true" || word == "false" {
				sb.WriteString(fmt.Sprintf("[purple]%s[-]", word))
				i += len(word) - 1
			} else if word == "null" {
				sb.WriteString(fmt.Sprintf("[red]%s[-]", word))
				i += len(word) - 1
			} else {
				sb.WriteRune(r)
			}
		default:
			if (r >= '0' && r <= '9') || r == '-' || r == '.' {
				word := ""
				for j := i; j < len(runes) && ((runes[j] >= '0' && runes[j] <= '9') || runes[j] == '-' || runes[j] == '.' || runes[j] == 'e' || runes[j] == 'E' || runes[j] == '+'); j++ {
					word += string(runes[j])
				}
				if len(word) > 0 {
					sb.WriteString(fmt.Sprintf("[yellow]%s[-]", word))
					i += len(word) - 1
				} else {
					if r == '[' {
						sb.WriteString("[[")
					} else {
						sb.WriteRune(r)
					}
				}
			} else {
				if r == '[' {
					sb.WriteString("[[")
				} else {
					sb.WriteRune(r)
				}
			}
		}
	}
	return sb.String()
}

func checkIsKey(runes []rune, startIdx int) bool {
	escaped := false
	endIdx := -1
	for j := startIdx + 1; j < len(runes); j++ {
		if escaped {
			escaped = false
			continue
		}
		if runes[j] == '\\' {
			escaped = true
			continue
		}
		if runes[j] == '"' {
			endIdx = j
			break
		}
	}
	if endIdx == -1 {
		return false
	}
	for j := endIdx + 1; j < len(runes); j++ {
		r := runes[j]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r == ':' {
			return true
		}
		break
	}
	return false
}
