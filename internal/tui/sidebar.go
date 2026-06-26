package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/farhank15/dbTui/internal/model"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Sidebar displays the connection tree and schema browser
type Sidebar struct {
	*tview.Flex
	treeView           *tview.TreeView
	root               *tview.TreeNode
	app                *App
	searchQuery        string
	cachedTables       map[string][]model.TableInfo
	forceFocusTree     bool
	savedExpandedDBs   map[string]bool
	savedExpandedConns map[string]bool
	searchActive       bool
}

// NewSidebar creates a new sidebar with search support
func NewSidebar(app *App) *Sidebar {
	root := tview.NewTreeNode("Connections").
		SetColor(Styles.Accent).
		SetExpanded(true)

	treeView := tview.NewTreeView()
	treeView.SetRoot(root)
	treeView.SetCurrentNode(root)
	treeView.SetTopLevel(1)
	treeView.SetBorder(true)
	treeView.SetTitle(" Explorer ")
	treeView.SetBorderColor(Styles.Border)

	s := &Sidebar{
		Flex:         tview.NewFlex().SetDirection(tview.FlexRow),
		treeView:     treeView,
		root:         root,
		app:          app,
		cachedTables: make(map[string][]model.TableInfo),
	}

	// Build layout: just the tree view
	s.AddItem(treeView, 0, 1, true)

	// ChangedFunc fires on single click AND arrow-key navigation (auto-actions)
	s.treeView.SetChangedFunc(s.onNodeActivated)
	// SelectedFunc fires on Enter key or double-click (explicit connect/disconnect)
	s.treeView.SetSelectedFunc(s.onSelect)
	s.treeView.SetInputCapture(s.onInput)

	// Visual focus indicator: change border color when focused
	s.treeView.SetFocusFunc(func() {
		s.treeView.SetBorderColor(Styles.BorderFocus)
	})
	s.treeView.SetBlurFunc(func() {
		s.treeView.SetBorderColor(Styles.Border)
	})


	return s
}

// RefreshConnections refreshes the connection list
func (s *Sidebar) RefreshConnections() {
	s.RebuildTree()
}

// ShowSearchExplorerDialog shows the search explorer dialog
func (s *Sidebar) ShowSearchExplorerDialog() {
	s.app.dialogs.ShowSearchExplorerDialog(s)
}

func (s *Sidebar) filterTables(query string) {
	s.searchQuery = query
	s.forceFocusTree = true
	s.RebuildTree()
}

func (s *Sidebar) createConnectionNode(conn model.Connection) *tview.TreeNode {
	node := tview.NewTreeNode("").
		SetReference(&sidebarRef{kind: "connection", id: conn.ID}).
		SetSelectable(true).
		SetExpanded(false)
	return node
}

func (s *Sidebar) createDatabaseNode(connID, dbName string) *tview.TreeNode {
	node := tview.NewTreeNode(fmt.Sprintf("[DB] %s", dbName)).
		SetColor(Styles.Text).
		SetReference(&sidebarRef{kind: "database", id: connID, db: dbName}).
		SetSelectable(true).
		SetExpanded(false)
	return node
}

func (s *Sidebar) createTableNode(connID, dbName, tableName string) *tview.TreeNode {
	node := tview.NewTreeNode(fmt.Sprintf("[Tbl] %s", tableName)).
		SetColor(Styles.TextSecondary).
		SetReference(&sidebarRef{kind: "table", id: connID, db: dbName, table: tableName}).
		SetSelectable(true)
	return node
}

// ExpandDatabase expands a database node and loads its tables
func (s *Sidebar) ExpandDatabase(connID, dbName string) {
	conn, err := s.app.dbManager.GetConnector(connID)
	if err != nil {
		s.app.app.QueueUpdateDraw(func() {
			s.app.statusBar.ShowError(fmt.Sprintf("Cannot load tables: %v", err))
		})
		return
	}

	tables, err := conn.GetTables(dbName)
	if err != nil {
		s.app.app.QueueUpdateDraw(func() {
			s.app.statusBar.ShowError(fmt.Sprintf("Failed to load tables: %v", err))
		})
		return
	}

	// Apply tree changes on main goroutine
	s.app.app.QueueUpdateDraw(func() {
		s.updateDatabaseNode(connID, dbName, tables)
	})
}

func (s *Sidebar) updateDatabaseNode(connID, dbName string, tables []model.TableInfo) {
	if s.cachedTables == nil {
		s.cachedTables = make(map[string][]model.TableInfo)
	}
	s.cachedTables[connID+"_"+dbName] = tables
	s.RebuildTree()
}

// RebuildTree builds the sidebar tree based on active search parameters
func (s *Sidebar) RebuildTree() {
	wasFocused := s.treeView.HasFocus()
	// Save the selected node's reference to restore it after rebuild
	var selectedKind, selectedID, selectedDB, selectedTable string
	selectedNode := s.treeView.GetCurrentNode()
	if selectedNode != nil {
		ref, ok := selectedNode.GetReference().(*sidebarRef)
		if ok {
			selectedKind = ref.kind
			selectedID = ref.id
			selectedDB = ref.db
			selectedTable = ref.table
		} else if selectedNode.GetText() == "[New Connection]" {
			selectedKind = "new_connection"
		}
	}

	// Save expanded states before rebuilding
	expandedDBs := make(map[string]bool)
	expandedConns := make(map[string]bool)
	for _, connNode := range s.root.GetChildren() {
		ref, ok := connNode.GetReference().(*sidebarRef)
		if ok && ref.kind == "connection" {
			if connNode.IsExpanded() {
				expandedConns[ref.id] = true
			}
			for _, dbNode := range connNode.GetChildren() {
				dbRef, ok := dbNode.GetReference().(*sidebarRef)
				if ok && dbRef.kind == "database" {
					if dbNode.IsExpanded() {
						expandedDBs[ref.id+"_"+dbRef.db] = true
					}
				}
			}
		}
	}

	// Handle search transition state to restore correct expanded states when search is cleared
	searchQueryLower := strings.ToLower(strings.TrimSpace(s.searchQuery))
	if searchQueryLower != "" {
		if !s.searchActive {
			s.searchActive = true
			s.savedExpandedConns = make(map[string]bool)
			for k, v := range expandedConns {
				s.savedExpandedConns[k] = v
			}
			s.savedExpandedDBs = make(map[string]bool)
			for k, v := range expandedDBs {
				s.savedExpandedDBs[k] = v
			}
		}
	} else {
		if s.searchActive {
			s.searchActive = false
			expandedConns = s.savedExpandedConns
			expandedDBs = s.savedExpandedDBs
			s.savedExpandedConns = nil
			s.savedExpandedDBs = nil
		}
	}

	s.root.ClearChildren()

	var nodeToSelect *tview.TreeNode

	// Add "[New Connection]" node
	newNode := tview.NewTreeNode("[New Connection]").
		SetColor(Styles.Success).
		SetSelectable(true)
	s.root.AddChild(newNode)
	if selectedKind == "new_connection" {
		nodeToSelect = newNode
	}

	query := strings.TrimSpace(s.searchQuery)
	queryLower := strings.ToLower(query)

	var dbQuery, tableQuery string
	hasDbQuery := false

	if queryLower != "" {
		if strings.Contains(queryLower, ".") {
			parts := strings.SplitN(queryLower, ".", 2)
			dbQuery = strings.TrimSpace(parts[0])
			tableQuery = strings.TrimSpace(parts[1])
			hasDbQuery = true
		} else if strings.Contains(queryLower, "/") {
			parts := strings.SplitN(queryLower, "/", 2)
			dbQuery = strings.TrimSpace(parts[0])
			tableQuery = strings.TrimSpace(parts[1])
			hasDbQuery = true
		} else if strings.Contains(queryLower, " ") {
			parts := strings.SplitN(queryLower, " ", 2)
			dbQuery = strings.TrimSpace(parts[0])
			tableQuery = strings.TrimSpace(parts[1])
			hasDbQuery = true
		} else {
			tableQuery = queryLower
		}
	}

	connections := s.app.config.GetConnections()
	for _, conn := range connections {
		isConnected := s.app.dbManager.IsConnected(conn.ID)
		var dbNodesToAdd []*tview.TreeNode
		var matchedTableNode *tview.TreeNode

		if isConnected {
			state := s.app.dbManager.GetConnectionState(conn.ID)
			if state != nil && state.Databases != nil {
				for _, dbName := range state.Databases {
					dbNameLower := strings.ToLower(dbName)

					// If there is dbQuery, check if dbName matches
					if hasDbQuery && dbQuery != "" && !strings.Contains(dbNameLower, dbQuery) {
						continue
					}

					dbNode := s.createDatabaseNode(conn.ID, dbName)

					// Get cached tables for this database
					cacheKey := conn.ID + "_" + dbName
					tables, isCached := s.cachedTables[cacheKey]

					// Expanded state logic:
					// If we are filtering, auto-expand database node
					expanded := false
					if queryLower != "" {
						expanded = true
					} else if val, exists := expandedDBs[conn.ID+"_"+dbName]; exists {
						expanded = val
					}
					dbNode.SetExpanded(expanded)

					// Auto-load tables in background if filtering or database is expanded, and not cached yet
					if !isCached && (queryLower != "" || expanded) {
						if s.cachedTables == nil {
							s.cachedTables = make(map[string][]model.TableInfo)
						}
						s.cachedTables[cacheKey] = []model.TableInfo{} // Mark as loading
						go s.ExpandDatabase(conn.ID, dbName)
					}

					var tableNodesToAdd []*tview.TreeNode
					var tempMatchedTable *tview.TreeNode
					if isCached {
						for _, table := range tables {
							tableNameLower := strings.ToLower(table.Name)
							if tableQuery == "" || strings.Contains(tableNameLower, tableQuery) {
								tableNode := s.createTableNode(conn.ID, dbName, table.Name)
								if selectedKind == "table" && selectedID == conn.ID && selectedDB == dbName && selectedTable == table.Name {
									tempMatchedTable = tableNode
								}
								tableNodesToAdd = append(tableNodesToAdd, tableNode)
							}
						}
					}

					// If we searched for a table, and this database has no matching tables,
					// and we have already loaded the tables (isCached is true)
					if tableQuery != "" && len(tableNodesToAdd) == 0 && isCached {
						continue
					}

					// Add matching tables to database node
					for _, tn := range tableNodesToAdd {
						dbNode.AddChild(tn)
					}

					if tempMatchedTable != nil {
						matchedTableNode = tempMatchedTable
					}

					dbNodesToAdd = append(dbNodesToAdd, dbNode)

					// Set database selection only if database is actually added to list
					if selectedKind == "database" && selectedID == conn.ID && selectedDB == dbName {
						nodeToSelect = dbNode
					}
				}
			}
		}

		// If we are filtering, and this connection has no matching databases (and it is connected)
		if queryLower != "" && isConnected && len(dbNodesToAdd) == 0 {
			continue
		}

		connNode := s.createConnectionNode(conn)
		if isConnected {
			connNode.SetColor(Styles.Success)
			connNode.SetText(fmt.Sprintf("[Conn] %s", conn.Name))
			
			// Default connection node to expanded unless explicitly closed in previous state
			expanded := true
			if queryLower != "" {
				expanded = true
			} else if val, exists := expandedConns[conn.ID]; exists {
				expanded = val
			}
			connNode.SetExpanded(expanded)
			
			for _, dbn := range dbNodesToAdd {
				connNode.AddChild(dbn)
			}
		} else {
			// If we are filtering, do not show disconnected connections
			if queryLower != "" {
				continue
			}
			connNode.SetColor(Styles.TextSecondary)
			connNode.SetText(fmt.Sprintf("  [Conn] %s", conn.Name))
		}

		s.root.AddChild(connNode)

		// Set connection or table selection only after parent connection is added to tree
		if selectedKind == "connection" && selectedID == conn.ID {
			nodeToSelect = connNode
		}
		if matchedTableNode != nil {
			nodeToSelect = matchedTableNode
		}
	}

	s.root.SetExpanded(true)

	if nodeToSelect != nil {
		s.treeView.SetCurrentNode(nodeToSelect)
	} else {
		if len(s.root.GetChildren()) > 0 {
			s.treeView.SetCurrentNode(s.root.GetChildren()[0])
		} else {
			s.treeView.SetCurrentNode(s.root)
		}
	}

	if wasFocused || s.forceFocusTree {
		if !s.app.dialogOpen {
			s.app.app.SetFocus(s.treeView)
		}
		s.forceFocusTree = false
	}
}

// onNodeActivated fires on single click OR arrow-key navigation.
// Table → auto-show table detail structure, Connection/Database → ignored.
func (s *Sidebar) onNodeActivated(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*sidebarRef)
	if !ok {
		// Root or "New Connection" — do nothing on activation
		return
	}

	switch ref.kind {
	case "table":
		s.app.statusBar.ShowInfo(fmt.Sprintf("Table: %s.%s", ref.db, ref.table))
		go func() {
			conn, err := s.app.dbManager.GetConnector(ref.id)
			if err != nil {
				return
			}
			detail, err := conn.GetTableDetail(ref.db, ref.table)
			if err != nil {
				return
			}
			s.app.app.QueueUpdateDraw(func() {
				s.app.ShowTableDetail(detail)
			})
		}()
	}
}

// onSelect fires on Enter key or double-click.
// Connection → connect/disconnect, New Connection → dialog.
func (s *Sidebar) onSelect(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*sidebarRef)
	if !ok {
		// "New Connection" node
		if node.GetText() == "[New Connection]" {
			s.app.ShowConnectionDialog(nil)
		}
		return
	}

	switch ref.kind {
	case "connection":
		if s.app.dbManager.IsConnected(ref.id) {
			node.SetExpanded(!node.IsExpanded())
		} else {
			conn := s.app.config.GetConnectionByID(ref.id)
			if conn != nil {
				s.app.ConnectTo(conn)
			} else {
				s.app.statusBar.ShowError("Connection not found in config — refreshing sidebar")
				s.RefreshConnections()
			}
		}
	case "database":
		if node.IsExpanded() {
			node.Collapse()
		} else {
			node.SetExpanded(true)
			s.app.statusBar.ShowInfo(fmt.Sprintf("Loading tables from %s...", ref.db))
			go func() {
				s.ExpandDatabase(ref.id, ref.db)
				s.app.app.QueueUpdateDraw(func() {
					s.app.statusBar.ShowInfo(fmt.Sprintf("Loaded tables from %s", ref.db))
				})
			}()
		}
	case "table":
		s.app.QueryTable(ref.id, ref.db, ref.table)
	}
}

func (s *Sidebar) onInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyTab:
		s.app.FocusQueryPanel()
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case '/':
			s.ShowSearchExplorerDialog()
			return nil
		case '-':
			s.CollapseAll()
			return nil
		case '+', '=':
			s.ExpandAll()
			return nil
		case 'c', 'C':
			s.app.ShowConnectionDialog(nil)
			return nil
		case 'n', 'N':
			// New database
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && (ref.kind == "connection" || ref.kind == "database") {
				s.app.dialogs.ShowCreateDBDialog(ref.id)
			}
			return nil
		case 'r', 'R':
			s.ForceRefresh()
			return nil
		case 'd', 'D':
			node := s.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "connection" {
				s.app.Disconnect(ref.id)
			}
			return nil
		case 'f', 'F':
			// Find data in column
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowSearchDataDialog(ref)
			}
			return nil
		case 'a', 'A':
			// Add column to table
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowAddColumnDialog(ref.id, ref.db, ref.table)
			}
			return nil
		case 'm', 'M':
			// Modify column
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowModifyColumnDialog(ref.id, ref.db, ref.table)
			}
			return nil
		case 'x', 'X':
			// Drop column
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowDropColumnDialog(ref.id, ref.db, ref.table)
			}
			return nil
		case 'v', 'V':
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.ShowTableDDL(ref.id, ref.db, ref.table)
			}
			return nil
		case 'y', 'Y':
			// Copy database or table name to clipboard
			node := s.GetCurrentNode()
			if node != nil {
				if ref, ok := node.GetReference().(*sidebarRef); ok {
					var toCopy string
					var nameType string
					if ref.kind == "table" {
						toCopy = ref.table
						nameType = "table"
					} else if ref.kind == "database" {
						toCopy = ref.db
						nameType = "database"
					}

					if toCopy != "" {
						if err := writeToClipboard(toCopy); err != nil {
							s.app.statusBar.ShowError(fmt.Sprintf("Failed to copy: %v", err))
						} else {
							s.app.statusBar.ShowSuccess(fmt.Sprintf("Copied %s '%s' to clipboard!", nameType, toCopy))
						}
						return nil
					}
				}
			}
			return nil
		}
	case tcell.KeyDelete, tcell.KeyBackspace2:
		node := s.GetCurrentNode()
		if node != nil {
			if ref, ok := node.GetReference().(*sidebarRef); ok {
				if ref.kind == "connection" {
					connConfig := s.app.config.GetConnectionByID(ref.id)
					name := ref.id
					if connConfig != nil {
						name = connConfig.Name
					}
					msg := fmt.Sprintf("Are you sure you want to remove connection '%s' from your saved list?", name)
					s.app.dialogs.ShowConfirmDialog(msg, func() {
						if s.app.dbManager.IsConnected(ref.id) {
							s.app.Disconnect(ref.id)
						}
						s.app.config.DeleteConnection(ref.id)
						s.RefreshConnections()
						s.app.statusBar.ShowSuccess(fmt.Sprintf("Connection '%s' removed.", name))
					})
					return nil
				} else if ref.kind == "database" {
					connName := ""
					connConfig := s.app.config.GetConnectionByID(ref.id)
					if connConfig != nil {
						connName = connConfig.Name
					}
					msg := fmt.Sprintf("Are you sure you want to DROP database '%s' on connection '%s'?\nThis is a destructive action and cannot be undone!", ref.db, connName)
					s.app.dialogs.ShowConfirmDialog(msg, func() {
						s.app.statusBar.ShowInfo(fmt.Sprintf("Dropping database '%s'...", ref.db))
						go func() {
							connector, err := s.app.dbManager.GetConnector(ref.id)
							if err != nil {
								s.app.app.QueueUpdateDraw(func() {
									s.app.statusBar.ShowError(fmt.Sprintf("Failed to get connector: %v", err))
								})
								return
							}
							
							err = connector.DropDatabase(ref.db)
							s.app.app.QueueUpdateDraw(func() {
								if err != nil {
									s.app.statusBar.ShowError(fmt.Sprintf("Failed to drop database: %v", err))
								} else {
									s.app.statusBar.ShowSuccess(fmt.Sprintf("Database '%s' dropped successfully!", ref.db))
									_ = s.app.dbManager.RefreshDatabases(ref.id)
									s.RefreshConnections()
								}
							})
						}()
					})
					return nil
				}
			}
		}
	}

	return event
}

// ExpandAllDatabases auto-expands all databases for a given connection
func (s *Sidebar) ExpandAllDatabases(connID string) {
	for _, connChild := range s.root.GetChildren() {
		ref, ok := connChild.GetReference().(*sidebarRef)
		if !ok || ref.kind != "connection" || ref.id != connID {
			continue
		}
		connChild.SetExpanded(true)
		for _, dbChild := range connChild.GetChildren() {
			dbRef, ok := dbChild.GetReference().(*sidebarRef)
			if !ok || dbRef.kind != "database" {
				continue
			}
			// Optimistically expand, load tables in background
			dbChild.SetExpanded(true)
			s.app.statusBar.ShowInfo(fmt.Sprintf("Loading tables from %s...", dbRef.db))
			go func(id, db string) {
				s.ExpandDatabase(id, db)
				s.app.app.QueueUpdateDraw(func() {
					s.app.statusBar.ShowSuccess(fmt.Sprintf("Loaded tables from %s", db))
				})
			}(dbRef.id, dbRef.db)
		}
	}
}

// GetCurrentNode delegates to the tree view
func (s *Sidebar) GetCurrentNode() *tview.TreeNode {
	return s.treeView.GetCurrentNode()
}

// GetTreeView returns the underlying tree view for focus management
func (s *Sidebar) GetTreeView() *tview.TreeView {
	return s.treeView
}

// sidebarRef is stored in each TreeNode's reference
type sidebarRef struct {
	kind  string // "connection", "database", "table"
	id    string // connection ID
	db    string // database name
	table string // table name
}

func (r *sidebarRef) String() string {
	var parts []string
	parts = append(parts, r.kind)
	parts = append(parts, r.id)
	if r.db != "" {
		parts = append(parts, r.db)
	}
	if r.table != "" {
		parts = append(parts, r.table)
	}
	return strings.Join(parts, "/")
}

// writeToClipboard copies text to system clipboard using native commands, with OSC 52 fallback
func writeToClipboard(text string) error {
	var cmd *exec.Cmd
	var useOSC52 bool

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			useOSC52 = true
		}
	default:
		useOSC52 = true
	}

	if useOSC52 {
		return writeOSC52(text)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return writeOSC52(text)
	}

	if err := cmd.Start(); err != nil {
		return writeOSC52(text)
	}

	if _, err := stdin.Write([]byte(text)); err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return writeOSC52(text)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return writeOSC52(text)
	}

	return nil
}

func writeOSC52(text string) error {
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	osc := fmt.Sprintf("\x1b]52;c;%s\x07", b64)
	_, err := os.Stdout.Write([]byte(osc))
	return err
}

// CollapseAll collapses all connection and database nodes in the tree
func (s *Sidebar) CollapseAll() {
	for _, connNode := range s.root.GetChildren() {
		connNode.SetExpanded(false)
		for _, dbNode := range connNode.GetChildren() {
			dbNode.SetExpanded(false)
		}
	}
	s.app.statusBar.ShowInfo("Collapsed all explorer nodes")
}

// ExpandAll expands all connection and database nodes in the tree
func (s *Sidebar) ExpandAll() {
	for _, connNode := range s.root.GetChildren() {
		connNode.SetExpanded(true)
		for _, dbNode := range connNode.GetChildren() {
			dbNode.SetExpanded(true)
		}
	}
	s.app.statusBar.ShowInfo("Expanded all explorer nodes")
}

// ForceRefresh reloads all databases and tables from the database servers
// and rebuilds the explorer tree.
func (s *Sidebar) ForceRefresh() {
	s.app.statusBar.ShowInfo("Refreshing databases and tables...")

	// Clear the cached tables so they are re-queried from the DB
	s.cachedTables = make(map[string][]model.TableInfo)

	// Fetch active connections and refresh their databases
	activeConns := s.app.dbManager.GetActiveConnections()
	if len(activeConns) == 0 {
		s.RebuildTree()
		s.app.statusBar.ShowSuccess("Explorer refreshed")
		return
	}

	go func() {
		for _, connState := range activeConns {
			if connState.Connected {
				_ = s.app.dbManager.RefreshDatabases(connState.Connection.ID)
			}
		}
		s.app.app.QueueUpdateDraw(func() {
			s.RebuildTree()

			// Auto-load tables for all databases in the background
			// so the user doesn't see an empty tree after refresh
			for _, connState := range activeConns {
				if !connState.Connected {
					continue
				}
				curState := s.app.dbManager.GetConnectionState(connState.Connection.ID)
				if curState != nil {
					for _, dbName := range curState.Databases {
						cacheKey := connState.Connection.ID + "_" + dbName
						if _, isCached := s.cachedTables[cacheKey]; !isCached {
							s.cachedTables[cacheKey] = []model.TableInfo{} // Mark as loading
							go s.ExpandDatabase(connState.Connection.ID, dbName)
						}
					}
				}
			}

			s.app.statusBar.ShowSuccess("Explorer refreshed successfully!")
		})
	}()
}
