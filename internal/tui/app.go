package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/farhank15/dbTui/internal/config"
	"github.com/farhank15/dbTui/internal/db"
	"github.com/farhank15/dbTui/internal/model"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func debugLog(msg string) {
	f, err := os.OpenFile("/home/mawa/My_File/Development/dev_ink/dbTui/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(time.Now().Format("15:04:05 ") + msg + "\n")
	}
}

// App is the main application orchestrator
type App struct {
	app               *tview.Application
	pages             *tview.Pages
	mainFlex          *tview.Flex
	rightFlex         *tview.Flex
	sidebar           *Sidebar
	queryPanel        *QueryPanel
	resultTable       *ResultTable
	statusBar         *StatusBar
	dialogs           *Dialogs
	dbManager         *db.Manager
	config            *config.ConfigManager
	activeConn        string // currently active connection ID
	dialogOpen        bool   // tracks if a dialog is currently showing
	sidebarHidden     bool
	editorHidden      bool
	queryHistory      []string
	historyIndex      int
	lastSelectedTable string
	lastSelectedDB    string
	focusBeforeDialog tview.Primitive // tracks the primitive focused before opening a dialog
}

// NewApp creates a new TUI application
func NewApp() *App {
	tviewApp := tview.NewApplication()

	a := &App{
		app:          tviewApp,
		pages:        tview.NewPages(),
		dbManager:    db.NewManager(),
		queryHistory: make([]string, 0),
		historyIndex: 0,
	}

	// Load config
	cfg, err := config.NewConfigManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not load config: %v\n", err)
		cfg = &config.ConfigManager{}
	}
	a.config = cfg

	// Load persistent query history
	a.loadHistory()

	// Create UI components
	a.sidebar = NewSidebar(a)
	a.queryPanel = NewQueryPanel(a)
	a.resultTable = NewResultTable(a)
	a.statusBar = NewStatusBar(a)
	a.dialogs = NewDialogs(a)

	// Build layout
	a.buildLayout()

	// Set up key handlers
	a.app.SetInputCapture(a.globalInputHandler)

	return a
}

// Run starts the application
func (a *App) Run() error {
	a.sidebar.RefreshConnections()
	// Set initial focus to sidebar so user can navigate immediately
	a.app.SetFocus(a.sidebar.GetTreeView())
	return a.app.SetRoot(a.pages, true).EnableMouse(true).Run()
}

func (a *App) buildLayout() {
	// Main horizontal layout: sidebar | content
	a.rightFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.queryPanel, 0, 1, true).
		AddItem(a.resultTable, 0, 3, false)

	a.mainFlex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.sidebar, 30, 0, false).
		AddItem(a.rightFlex, 0, 1, true)

	// Main page with status bar
	mainPage := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.mainFlex, 0, 1, true).
		AddItem(a.statusBar, 3, 0, false)

	a.pages.AddPage("main", mainPage, true, true)
}

// globalInputHandler handles global keyboard shortcuts
func (a *App) globalInputHandler(event *tcell.EventKey) *tcell.EventKey {
	// Only handle if no dialog is showing
	if a.dialogOpen {
		return event
	}

	switch event.Key() {
	case tcell.KeyCtrlN:
		a.ShowConnectionDialog(nil)
		return nil
	case tcell.KeyCtrlD:
		if a.activeConn != "" {
			a.Disconnect(a.activeConn)
		}
		return nil
	case tcell.KeyCtrlE:
		a.ExportResults()
		return nil
	case tcell.KeyCtrlL:
		a.queryPanel.Clear()
		return nil
	case tcell.KeyF1:
		a.dialogs.ShowHelpDialog()
		return nil
	case tcell.KeyEscape:
		a.app.SetFocus(a.getFocusTarget())
		return nil
	case tcell.KeyF5:
		// F5 from anywhere refreshes both sidebar tree AND active query
		a.sidebar.ForceRefresh()
		a.RefreshActiveQuery()
		return nil
	case tcell.KeyCtrlB:
		a.ToggleSidebar()
		return nil
	case tcell.KeyCtrlH:
		a.ToggleEditor()
		return nil
	case tcell.KeyCtrlT:
		a.dialogs.ShowSQLTemplatesDialog()
		return nil
	}

	return event
}

// ConnectTo establishes a database connection
func (a *App) ConnectTo(conn *model.Connection) {
	a.statusBar.ShowInfo(fmt.Sprintf("Connecting to %s...", conn.Name))

	go func() {
		// Close existing connection with same ID
		if a.dbManager.IsConnected(conn.ID) {
			a.dbManager.Disconnect(conn.ID)
		}

		start := time.Now()
		err := a.dbManager.Connect(*conn)
		elapsed := time.Since(start)

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.statusBar.ShowError(fmt.Sprintf("Connection failed: %v", err))
				return
			}

			a.activeConn = conn.ID
			a.statusBar.ShowSuccess(fmt.Sprintf("Connected to %s (%s) in %s", conn.Name, conn.Type, elapsed))

			// Refresh databases
			if err := a.dbManager.RefreshDatabases(conn.ID); err != nil {
				a.statusBar.ShowError(fmt.Sprintf("Failed to load databases: %v", err))
			}

			a.sidebar.RefreshConnections()

			// If databases were loaded, show success status
			state := a.dbManager.GetConnectionState(conn.ID)
			if state != nil && len(state.Databases) > 0 {
				a.statusBar.ShowSuccess(fmt.Sprintf("Connected! Found %d databases.", len(state.Databases)))
			}

			// Focus sidebar so user can navigate the tree immediately
			a.app.SetFocus(a.sidebar.GetTreeView())
		})
	}()
}

// Disconnect closes a connection
func (a *App) Disconnect(id string) {
	a.statusBar.ShowInfo("Disconnecting...")

	go func() {
		if err := a.dbManager.Disconnect(id); err != nil {
			a.app.QueueUpdateDraw(func() {
				a.statusBar.ShowError(fmt.Sprintf("Disconnect error: %v", err))
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			if a.activeConn == id {
				a.activeConn = ""
			}
			a.statusBar.ShowInfo("Disconnected")
			a.sidebar.RefreshConnections()
			a.app.SetFocus(a.sidebar.GetTreeView())
		})
	}()
}

// ExecuteQuery runs the current query in the query panel (or selection if any)
func (a *App) ExecuteQuery() {
	var query string
	if a.queryPanel.HasSelection() {
		selText, _, _ := a.queryPanel.GetSelection()
		query = selText
	} else {
		query = a.queryPanel.GetQueryText()
	}
	query = strings.TrimSpace(query)

	if query == "" {
		a.statusBar.ShowError("No query to execute")
		return
	}

	if a.activeConn == "" {
		a.statusBar.ShowError("No active connection")
		return
	}

	conn, err := a.dbManager.GetConnector(a.activeConn)
	if err != nil {
		a.statusBar.ShowError(fmt.Sprintf("Error: %v", err))
		return
	}

	a.statusBar.ShowInfo("Executing query...")

	go func() {
		start := time.Now()
		result, err := conn.ExecuteQuery(query)
		elapsed := time.Since(start)

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.statusBar.ShowError(fmt.Sprintf("Query error: %v", err))
				a.resultTable.DisplayResult(&model.QueryResult{
					Error:    err.Error(),
					Duration: elapsed.String(),
				})
				return
			}

			// Add successfully executed query to history
			historyLen := len(a.queryHistory)
			if historyLen == 0 || a.queryHistory[historyLen-1] != query {
				a.queryHistory = append(a.queryHistory, query)
				a.saveHistory()
			}
			a.historyIndex = len(a.queryHistory)

			result.Duration = elapsed.String()
			
			// Try to populate context for inline editing
			parsedTable := parseTableName(query)
			if parsedTable != "" {
				result.ConnID = a.activeConn
				if state := a.dbManager.GetConnectionState(a.activeConn); state != nil {
					result.Database = state.Connection.Database
				}
				result.Table = parsedTable
			}
			
			a.resultTable.DisplayResult(result)

			if result.Error != "" {
				a.statusBar.ShowError(fmt.Sprintf("Query error: %s", result.Error))
			} else if !result.IsSelect {
				a.statusBar.ShowSuccess(result.Message)
			} else {
				rows := result.RowCount()
				if result.Duration != "" {
					a.statusBar.ShowSuccess(fmt.Sprintf("%d rows returned in %s", rows, elapsed))
				} else {
					a.statusBar.ShowSuccess(fmt.Sprintf("%d rows returned", rows))
				}
			}

			// Focus results
			a.app.SetFocus(a.resultTable)
		})
	}()
}

// ShowTableDetail displays table information in the results panel
func (a *App) ShowTableDetail(detail *model.TableDetail) {
	a.resultTable.DisplayTableDetail(detail)
}

// QueryTable generates and runs a SELECT query for the specified table
func (a *App) QueryTable(connID, dbName, tableName string) {
	connConfig := a.config.GetConnectionByID(connID)
	if connConfig == nil {
		a.statusBar.ShowError("Connection config not found")
		return
	}

	var quotedTable string
	switch connConfig.Type {
	case model.TypeMySQL:
		parts := strings.Split(tableName, ".")
		for i, part := range parts {
			parts[i] = fmt.Sprintf("`%s`", part)
		}
		quotedTable = strings.Join(parts, ".")
	default:
		parts := strings.Split(tableName, ".")
		for i, part := range parts {
			parts[i] = fmt.Sprintf("\"%s\"", part)
		}
		quotedTable = strings.Join(parts, ".")
	}

	query := fmt.Sprintf("SELECT * FROM %s", quotedTable)
	a.queryPanel.SetQueryText(query)

	connector, err := a.dbManager.GetConnector(connID)
	if err != nil {
		a.statusBar.ShowError(fmt.Sprintf("Error: %v", err))
		return
	}

	a.lastSelectedTable = tableName
	a.lastSelectedDB = dbName

	a.statusBar.ShowInfo(fmt.Sprintf("Querying table %s...", tableName))

	go func() {
		start := time.Now()
		result, err := connector.ExecuteQueryWithDB(dbName, query)
		elapsed := time.Since(start)

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.statusBar.ShowError(fmt.Sprintf("Query error: %v", err))
				a.resultTable.DisplayResult(&model.QueryResult{
					Error:    err.Error(),
					Duration: elapsed.String(),
				})
				return
			}

			result.Duration = elapsed.String()
			result.ConnID = connID
			result.Database = dbName
			result.Table = tableName
			a.resultTable.DisplayResult(result)

			if result.Error != "" {
				a.statusBar.ShowError(fmt.Sprintf("Query error: %s", result.Error))
			} else {
				rows := result.RowCount()
				a.statusBar.ShowSuccess(fmt.Sprintf("%d rows returned in %s", rows, elapsed))
			}

			// Focus results
			a.app.SetFocus(a.resultTable)
		})
	}()
}

// ExportResults exports the current results to CSV
func (a *App) ExportResults() {
	if a.resultTable.result == nil {
		a.statusBar.ShowError("No results to export")
		return
	}

	csv := a.resultTable.ExportToCSV()

	// Save to file
	homeDir, _ := os.UserHomeDir()
	filename := fmt.Sprintf("dbTui_export_%d.csv", time.Now().Unix())
	if homeDir != "" {
		altPath := homeDir + string(os.PathSeparator) + "Downloads" + string(os.PathSeparator) + filename
		if err := os.WriteFile(altPath, []byte(csv), 0644); err == nil {
			filename = altPath
			a.statusBar.ShowSuccess(fmt.Sprintf("Exported to %s (%d rows)", filename, len(a.resultTable.result.Rows)))
			return
		}
		altPath = homeDir + string(os.PathSeparator) + filename
		if err := os.WriteFile(altPath, []byte(csv), 0644); err == nil {
			filename = altPath
			a.statusBar.ShowSuccess(fmt.Sprintf("Exported to %s (%d rows)", filename, len(a.resultTable.result.Rows)))
			return
		}
	}
	// Fallback to current dir
	if err := os.WriteFile(filename, []byte(csv), 0644); err != nil {
		a.statusBar.ShowError(fmt.Sprintf("Export failed: %v", err))
		return
	}

	a.statusBar.ShowSuccess(fmt.Sprintf("Exported to %s (%d rows)", filename, len(a.resultTable.result.Rows)))
}

// ShowConnectionDialog shows the connection dialog
func (a *App) ShowConnectionDialog(conn *model.Connection) {
	a.dialogs.ShowConnectionDialog(conn)
}

// ShowSearchRowsDialog shows the row filter dialog
func (a *App) ShowSearchRowsDialog() {
	a.dialogs.ShowSearchRowsDialog()
}

// FocusQueryPanel sets focus to the query panel
func (a *App) FocusQueryPanel() {
	a.app.SetFocus(a.queryPanel)
}

// FocusResultTable sets focus to the result table
func (a *App) FocusResultTable() {
	a.app.SetFocus(a.resultTable)
}

// isFocusable returns whether the primitive is safe to focus (not nil and not hidden)
func (a *App) isFocusable(p tview.Primitive) bool {
	if p == nil {
		return false
	}
	if a.sidebarHidden {
		if p == a.sidebar.GetTreeView() || p == a.sidebar {
			return false
		}
	}
	if a.editorHidden {
		if p == a.queryPanel || (a.queryPanel != nil && p == a.queryPanel.TextArea) {
			return false
		}
	}
	return true
}

// ShowDialog shows a modal dialog
func (a *App) showDialog(primitive tview.Primitive) {
	// Remember the currently focused primitive before showing the dialog
	if !a.dialogOpen {
		if currentFocus := a.app.GetFocus(); currentFocus != nil {
			a.focusBeforeDialog = currentFocus
			debugLog(fmt.Sprintf("showDialog: saved focusBeforeDialog = %T", currentFocus))
		} else {
			a.focusBeforeDialog = nil
			debugLog("showDialog: focusBeforeDialog = nil")
		}
	}

	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(primitive, 0, 3, true).
			AddItem(nil, 0, 1, false),
			0, 2, true).
		AddItem(nil, 0, 1, false)

	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			a.closeDialog()
			return nil
		}
		return event
	})

	a.dialogOpen = true
	a.pages.AddPage("dialog", flex, true, true)
	a.app.SetFocus(primitive)
}

// closeDialog closes the current dialog
func (a *App) closeDialog() {
	target := a.getFocusTarget()
	wasSaved := a.focusBeforeDialog != nil
	debugLog(fmt.Sprintf("closeDialog: focusBeforeDialog exists = %v (%T), isFocusable = %v", wasSaved, a.focusBeforeDialog, a.isFocusable(a.focusBeforeDialog)))
	if a.isFocusable(a.focusBeforeDialog) {
		target = a.focusBeforeDialog
	}
	a.focusBeforeDialog = nil

	debugLog(fmt.Sprintf("closeDialog: target resolved to = %T", target))
	go func() {
		a.app.QueueUpdateDraw(func() {
			debugLog("closeDialog QueueUpdateDraw: removing dialog page")
			a.pages.RemovePage("dialog")
			a.dialogOpen = false
			debugLog(fmt.Sprintf("closeDialog QueueUpdateDraw: calling SetFocus on %T", target))
			a.app.SetFocus(target)
			debugLog(fmt.Sprintf("closeDialog QueueUpdateDraw: current focus after SetFocus = %T", a.app.GetFocus()))
		})
	}()
}

// getFocusTarget returns the appropriate visible focus target (sidebar or query panel fallback)
func (a *App) getFocusTarget() tview.Primitive {
	if a.sidebarHidden {
		return a.queryPanel
	}
	return a.sidebar.GetTreeView()
}

// ToggleSidebar shows or hides the sidebar explorer
func (a *App) ToggleSidebar() {
	a.sidebarHidden = !a.sidebarHidden
	if a.sidebarHidden {
		a.mainFlex.ResizeItem(a.sidebar, 0, 0)
		if a.sidebar.GetTreeView().HasFocus() {
			a.app.SetFocus(a.queryPanel)
		}
	} else {
		a.mainFlex.ResizeItem(a.sidebar, 30, 0)
		a.app.SetFocus(a.sidebar.GetTreeView())
	}
}

// ToggleEditor shows or hides the SQL editor
func (a *App) ToggleEditor() {
	a.editorHidden = !a.editorHidden
	if a.editorHidden {
		a.rightFlex.ResizeItem(a.queryPanel, 0, 0)
		if a.queryPanel.HasFocus() {
			a.app.SetFocus(a.resultTable)
		}
	} else {
		a.rightFlex.ResizeItem(a.queryPanel, 0, 1)
		a.app.SetFocus(a.queryPanel)
	}
}

// HistoryPrev moves to the previous query in history
func (a *App) HistoryPrev() {
	if len(a.queryHistory) == 0 {
		return
	}
	if a.historyIndex > 0 {
		a.historyIndex--
	}
	a.queryPanel.SetQueryText(a.queryHistory[a.historyIndex])
}

// HistoryNext moves to the next query in history
func (a *App) HistoryNext() {
	if len(a.queryHistory) == 0 {
		return
	}
	if a.historyIndex < len(a.queryHistory)-1 {
		a.historyIndex++
		a.queryPanel.SetQueryText(a.queryHistory[a.historyIndex])
	} else if a.historyIndex == len(a.queryHistory)-1 {
		a.historyIndex = len(a.queryHistory)
		a.queryPanel.Clear()
	}
}

func (a *App) getHistoryPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "history.json"
	}
	return filepath.Join(homeDir, ".dbTui", "history.json")
}

func (a *App) loadHistory() {
	path := a.getHistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var history []string
	if err := json.Unmarshal(data, &history); err == nil {
		a.queryHistory = history
		a.historyIndex = len(history)
	}
}

func (a *App) saveHistory() {
	path := a.getHistoryPath()
	history := a.queryHistory
	if len(history) > 100 {
		history = history[len(history)-100:]
		a.queryHistory = history
	}
	data, err := json.Marshal(history)
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func parseTableName(query string) string {
	query = strings.TrimSpace(query)
	queryLower := strings.ToLower(query)
	if !strings.HasPrefix(queryLower, "select") {
		return ""
	}
	fromIdx := strings.Index(queryLower, " from ")
	if fromIdx == -1 {
		return ""
	}
	afterFrom := strings.TrimSpace(query[fromIdx+6:])
	var sb strings.Builder
	for _, r := range afterFrom {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' || r == ',' || r == ';' || r == '(' || r == ')' || r == '`' || r == '"' {
			if sb.Len() > 0 {
				break
			}
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func (a *App) ShowTableDDL(connID, dbName, tableName string) {
	a.dialogs.ShowTableDDLDialog(connID, dbName, tableName)
}

func (a *App) GetTableDDL(connID, dbName, tableName string) (string, error) {
	connConfig := a.config.GetConnectionByID(connID)
	if connConfig == nil {
		return "", fmt.Errorf("connection config not found")
	}

	connector, err := a.dbManager.GetConnector(connID)
	if err != nil {
		return "", err
	}

	switch connConfig.Type {
	case model.TypeSQLite:
		query := fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name = '%s'", tableName)
		res, err := connector.ExecuteQueryWithDB(dbName, query)
		if err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			return res.Rows[0][0], nil
		}
	case model.TypeMySQL:
		query := fmt.Sprintf("SHOW CREATE TABLE `%s`", tableName)
		res, err := connector.ExecuteQueryWithDB(dbName, query)
		if err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 1 {
			return res.Rows[0][1], nil
		}
	}

	detail, err := connector.GetTableDetail(dbName, tableName)
	if err != nil {
		return "", err
	}

	return ReconstructDDL(detail, connConfig.Type), nil
}

func ReconstructDDL(detail *model.TableDetail, dbType model.ConnectionType) string {
	var sb strings.Builder
	quotedTable := detail.Table.Name
	if dbType == model.TypeMySQL {
		quotedTable = fmt.Sprintf("`%s`", detail.Table.Name)
	} else {
		quotedTable = fmt.Sprintf("\"%s\"", detail.Table.Name)
	}

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", quotedTable))

	for i, col := range detail.Table.Columns {
		var colQuoted string
		if dbType == model.TypeMySQL {
			colQuoted = fmt.Sprintf("  `%s`", col.Name)
		} else {
			colQuoted = fmt.Sprintf("  \"%s\"", col.Name)
		}

		sb.WriteString(fmt.Sprintf("%s %s", colQuoted, col.Type))

		if col.Nullable == "NO" || col.Nullable == "false" {
			sb.WriteString(" NOT NULL")
		}

		if col.Default != "" {
			sb.WriteString(fmt.Sprintf(" DEFAULT %s", col.Default))
		}

		if col.Extra != "" {
			sb.WriteString(fmt.Sprintf(" %s", col.Extra))
		}

		if i < len(detail.Table.Columns)-1 || len(detail.ForeignKeys) > 0 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}

	for i, fk := range detail.ForeignKeys {
		var fkName string
		if fk.Name != "" {
			if dbType == model.TypeMySQL {
				fkName = fmt.Sprintf("CONSTRAINT `%s` ", fk.Name)
			} else {
				fkName = fmt.Sprintf("CONSTRAINT \"%s\" ", fk.Name)
			}
		}

		var colQuoted, refColQuoted, refTableQuoted string
		if dbType == model.TypeMySQL {
			colQuoted = fmt.Sprintf("`%s`", fk.Column)
			refColQuoted = fmt.Sprintf("`%s`", fk.RefColumn)
			refTableQuoted = fmt.Sprintf("`%s`", fk.RefTable)
		} else {
			colQuoted = fmt.Sprintf("\"%s\"", fk.Column)
			refColQuoted = fmt.Sprintf("\"%s\"", fk.RefColumn)
			refTableQuoted = fmt.Sprintf("\"%s\"", fk.RefTable)
		}

		sb.WriteString(fmt.Sprintf("  %sFOREIGN KEY (%s) REFERENCES %s(%s)",
			fkName, colQuoted, refTableQuoted, refColQuoted))

		if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
			sb.WriteString(fmt.Sprintf(" ON DELETE %s", fk.OnDelete))
		}
		if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
			sb.WriteString(fmt.Sprintf(" ON UPDATE %s", fk.OnUpdate))
		}

		if i < len(detail.ForeignKeys)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}

	sb.WriteString(");\n")

	for _, idx := range detail.Indexes {
		if idx.Primary {
			continue
		}
		var uniqueStr string
		if idx.Unique {
			uniqueStr = "UNIQUE "
		}

		var cols []string
		for _, c := range idx.Columns {
			if dbType == model.TypeMySQL {
				cols = append(cols, fmt.Sprintf("`%s`", c))
			} else {
				cols = append(cols, fmt.Sprintf("\"%s\"", c))
			}
		}

		var idxQuoted, tblQuoted string
		if dbType == model.TypeMySQL {
			idxQuoted = fmt.Sprintf("`%s`", idx.Name)
			tblQuoted = fmt.Sprintf("`%s`", detail.Table.Name)
		} else {
			idxQuoted = fmt.Sprintf("\"%s\"", idx.Name)
			tblQuoted = fmt.Sprintf("\"%s\"", detail.Table.Name)
		}

		sb.WriteString(fmt.Sprintf("\nCREATE %sINDEX %s ON %s (%s);",
			uniqueStr, idxQuoted, tblQuoted, strings.Join(cols, ", ")))
	}

	return sb.String()
}

// RefreshActiveQuery re-runs the current query or reloads the active table data in the results view
func (a *App) RefreshActiveQuery() {
	if a.resultTable.result == nil {
		a.statusBar.ShowError("No active query or table to refresh")
		return
	}

	res := a.resultTable.result
	if res.Table != "" && res.Database != "" && res.ConnID != "" {
		// It's a table view, reload it!
		a.statusBar.ShowInfo("Refreshing table data...")
		a.QueryTable(res.ConnID, res.Database, res.Table)
	} else {
		// It's a custom SQL query, execute it again!
		a.statusBar.ShowInfo("Refreshing SQL query...")
		a.ExecuteQuery()
	}
}
