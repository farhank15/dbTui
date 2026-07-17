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

type Sidebar struct {
	*tview.Flex
	treeView     *tview.TreeView
	root         *tview.TreeNode
	app          *App
	cachedTables map[string][]model.TableInfo
	filterText   string
}

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
	treeView.SetTitleColor(Styles.TextDim)
	treeView.SetBorderColor(Styles.Border)

	s := &Sidebar{
		Flex:         tview.NewFlex().SetDirection(tview.FlexRow),
		treeView:     treeView,
		root:         root,
		app:          app,
		cachedTables: make(map[string][]model.TableInfo),
	}

	actionBar := s.buildActionBar()

	s.AddItem(treeView, 0, 1, true)
	s.AddItem(actionBar, 1, 0, false)

	s.treeView.SetChangedFunc(s.onNodeActivated)
	s.treeView.SetSelectedFunc(s.onSelect)
	s.treeView.SetInputCapture(s.onInput)

	s.treeView.SetFocusFunc(func() {
		treeView.SetBorderColor(Styles.BorderFocus)
	})
	s.treeView.SetBlurFunc(func() {
		treeView.SetBorderColor(Styles.Border)
	})

	return s
}

func (s *Sidebar) buildActionBar() *tview.Flex {
	bar := tview.NewFlex().SetDirection(tview.FlexColumn)

	addBtn := tview.NewButton("[ Add DB ]")
	addBtn.SetStyle(tcell.StyleDefault.Background(Styles.Surface).Foreground(Styles.Success))
	addBtn.SetActivatedStyle(tcell.StyleDefault.Background(Styles.Success).Foreground(Styles.Background))
	addBtn.SetSelectedFunc(func() {
		node := s.treeView.GetCurrentNode()
		ref, ok := node.GetReference().(*sidebarRef)
		connID := ""
		if ok {
			connID = ref.id
		}
		if connID == "" {
			for _, child := range s.root.GetChildren() {
				if ref2, ok2 := child.GetReference().(*sidebarRef); ok2 && ref2.kind == "connection" {
					if s.app.dbManager.IsConnected(ref2.id) {
						connID = ref2.id
						break
					}
				}
			}
		}
		if connID != "" {
			s.app.dialogs.ShowCreateDBDialog(connID)
		} else {
			s.app.statusBar.ShowError("No connected database selected")
		}
	})

	dropBtn := tview.NewButton("[ Drop DB ]")
	dropBtn.SetStyle(tcell.StyleDefault.Background(Styles.Surface).Foreground(Styles.Error))
	dropBtn.SetActivatedStyle(tcell.StyleDefault.Background(Styles.Error).Foreground(Styles.Background))
	dropBtn.SetSelectedFunc(func() {
		node := s.treeView.GetCurrentNode()
		if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "database" {
			s.promptDropDB(ref)
		} else {
			s.app.statusBar.ShowError("Select a database first")
		}
	})

	refreshBtn := tview.NewButton("[ Refresh ]")
	refreshBtn.SetStyle(tcell.StyleDefault.Background(Styles.Surface).Foreground(Styles.Primary))
	refreshBtn.SetActivatedStyle(tcell.StyleDefault.Background(Styles.Primary).Foreground(Styles.Background))
	refreshBtn.SetSelectedFunc(func() {
		s.ForceRefresh()
	})

	bar.AddItem(addBtn, 0, 1, false)
	bar.AddItem(dropBtn, 0, 1, false)
	bar.AddItem(refreshBtn, 0, 1, false)

	return bar
}

func (s *Sidebar) RefreshConnections() {
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
	node := tview.NewTreeNode(dbName).
		SetColor(Styles.Text).
		SetReference(&sidebarRef{kind: "database", id: connID, db: dbName}).
		SetSelectable(true).
		SetExpanded(false)
	return node
}

func (s *Sidebar) createTableNode(connID, dbName, tableName string) *tview.TreeNode {
	node := tview.NewTreeNode(tableName).
		SetColor(Styles.Text).
		SetReference(&sidebarRef{kind: "table", id: connID, db: dbName, table: tableName}).
		SetSelectable(true)
	return node
}

func (s *Sidebar) promptDropDB(ref *sidebarRef) {
	connConfig := s.app.config.GetConnectionByID(ref.id)
	connName := ref.id
	if connConfig != nil {
		connName = connConfig.Name
	}
	msg := fmt.Sprintf("Drop database '%s' on connection '%s'?\nThis cannot be undone!", ref.db, connName)
	s.app.dialogs.ShowConfirmDialog(msg, func() {
		s.app.statusBar.ShowInfo(fmt.Sprintf("Dropping database '%s'...", ref.db))
		go func() {
			connector, err := s.app.dbManager.GetConnector(ref.id)
			if err != nil {
				s.app.app.QueueUpdateDraw(func() {
					s.app.statusBar.ShowError(fmt.Sprintf("Failed: %v", err))
				})
				return
			}
			err = connector.DropDatabase(ref.db)
			s.app.app.QueueUpdateDraw(func() {
				if err != nil {
					s.app.statusBar.ShowError(fmt.Sprintf("Failed: %v", err))
				} else {
					s.app.statusBar.ShowSuccess(fmt.Sprintf("Database '%s' dropped!", ref.db))
					s.app.dbManager.RefreshDatabases(ref.id)
					s.RefreshConnections()
				}
			})
		}()
	})
}

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

func (s *Sidebar) PrefetchTables(connID string) {
	conn, err := s.app.dbManager.GetConnector(connID)
	if err != nil {
		return
	}
	state := s.app.dbManager.GetConnectionState(connID)
	if state == nil || state.Databases == nil {
		return
	}

	for _, dbName := range state.Databases {
		dbName := dbName
		cacheKey := connID + "_" + dbName
		if s.cachedTables == nil {
			s.cachedTables = make(map[string][]model.TableInfo)
		}
		if _, cached := s.cachedTables[cacheKey]; cached {
			continue
		}
		s.cachedTables[cacheKey] = []model.TableInfo{} // Mark as loading
		go func() {
			tables, err := conn.GetTables(dbName)
			s.app.app.QueueUpdateDraw(func() {
				if err == nil {
					s.cachedTables[cacheKey] = tables
					if s.filterText != "" {
						s.RebuildTree()
					}
				} else {
					delete(s.cachedTables, cacheKey)
				}
			})
		}()
	}
}

func (s *Sidebar) RebuildTree() {
	totalMatches := 0
	var selectedKind, selectedID, selectedDB string
	selectedNode := s.treeView.GetCurrentNode()
	if selectedNode != nil {
		ref, ok := selectedNode.GetReference().(*sidebarRef)
		if ok {
			selectedKind = ref.kind
			selectedID = ref.id
			selectedDB = ref.db
		} else if selectedNode.GetText() == "+ New Connection" {
			selectedKind = "new_connection"
		}
	}

	expandedDBs := make(map[string]bool)
	// expandedConns tracks explicit state: true=expanded, false=collapsed
	// We use a separate set to know which conns have been seen in the tree before
	expandedConns := make(map[string]bool)
	seenConns := make(map[string]bool)
	hadChildren := make(map[string]bool)
	for _, connNode := range s.root.GetChildren() {
		ref, ok := connNode.GetReference().(*sidebarRef)
		if ok && ref.kind == "connection" {
			seenConns[ref.id] = true
			expandedConns[ref.id] = connNode.IsExpanded()
			hadChildren[ref.id] = len(connNode.GetChildren()) > 0
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

	s.root.ClearChildren()

	var nodeToSelect *tview.TreeNode

	newNode := tview.NewTreeNode("+ New Connection").
		SetColor(Styles.Success).
		SetSelectable(true)
	s.root.AddChild(newNode)
	if selectedKind == "new_connection" {
		nodeToSelect = newNode
	}

	connections := s.app.config.GetConnections()
	for _, conn := range connections {
		isConnected := s.app.dbManager.IsConnected(conn.ID)
		var dbNodesToAdd []*tview.TreeNode

		if isConnected {
			s.PrefetchTables(conn.ID)
			state := s.app.dbManager.GetConnectionState(conn.ID)
			if state != nil && state.Databases != nil {
				for _, dbName := range state.Databases {
					cacheKey := conn.ID + "_" + dbName
					tables, isCached := s.cachedTables[cacheKey]

					var dbMatches bool
					var matchedTables []model.TableInfo

					filterTextLower := strings.ToLower(s.filterText)
					if strings.Contains(filterTextLower, "/") {
						parts := strings.SplitN(filterTextLower, "/", 2)
						dbPart := parts[0]
						tablePart := parts[1]

						dbMatches = dbPart == "" || strings.Contains(strings.ToLower(dbName), dbPart)

						if isCached && dbMatches {
							for _, table := range tables {
								if tablePart == "" || strings.Contains(strings.ToLower(table.Name), tablePart) {
									matchedTables = append(matchedTables, table)
								}
							}
						}

						if s.filterText != "" && !dbMatches {
							continue
						}
						if s.filterText != "" && dbMatches && tablePart != "" && len(matchedTables) == 0 {
							continue
						}
					} else {
						dbMatches = s.filterText == "" || strings.Contains(strings.ToLower(dbName), filterTextLower)

						if isCached {
							for _, table := range tables {
								if dbMatches || s.filterText == "" || strings.Contains(strings.ToLower(table.Name), filterTextLower) {
									matchedTables = append(matchedTables, table)
								}
							}
						}

						if s.filterText != "" && !dbMatches && len(matchedTables) == 0 {
							continue
						}
					}

					dbNode := s.createDatabaseNode(conn.ID, dbName)

					expanded := false
					if val, exists := expandedDBs[conn.ID+"_"+dbName]; exists {
						expanded = val
					}
					// Auto-expand databases when filtering to show matching tables
					if s.filterText != "" && len(matchedTables) > 0 {
						expanded = true
					}
					dbNode.SetExpanded(expanded)

					if !isCached && expanded {
						if s.cachedTables == nil {
							s.cachedTables = make(map[string][]model.TableInfo)
						}
						s.cachedTables[cacheKey] = []model.TableInfo{}
						go s.ExpandDatabase(conn.ID, dbName)
					}

					if isCached {
						for _, table := range matchedTables {
							tableNode := s.createTableNode(conn.ID, dbName, table.Name)
							dbNode.AddChild(tableNode)
						}
					}

					dbNodesToAdd = append(dbNodesToAdd, dbNode)

					if selectedKind == "database" && selectedID == conn.ID && selectedDB == dbName {
						nodeToSelect = dbNode
					}
				}
			}
		}

		connNode := s.createConnectionNode(conn)
		if isConnected {
			connNode.SetColor(Styles.Success)
			connNode.SetText(fmt.Sprintf("● %s", conn.Name))

			// Default expanded=true only for brand-new connections (not yet seen in tree)
			// or if it was previously in the tree but had no children (i.e., transitioning from disconnected to connected).
			expanded := !seenConns[conn.ID] || !hadChildren[conn.ID]
			if seenConns[conn.ID] && hadChildren[conn.ID] {
				expanded = expandedConns[conn.ID]
			}
			// Auto-expand connection node if filtering and matching DBs/tables are inside
			if s.filterText != "" && len(dbNodesToAdd) > 0 {
				expanded = true
			}
			connNode.SetExpanded(expanded)

			for _, dbn := range dbNodesToAdd {
				totalMatches++
				totalMatches += len(dbn.GetChildren())
				connNode.AddChild(dbn)
			}
		} else {
			connNode.SetColor(Styles.TextSecondary)
			connNode.SetText(fmt.Sprintf("○ %s", conn.Name))
		}

		s.root.AddChild(connNode)

		if selectedKind == "connection" && selectedID == conn.ID {
			nodeToSelect = connNode
		}
	}

	s.treeView.SetTitle(" Explorer ")

	if s.filterText != "" {
		if totalMatches == 0 {
			s.app.statusBar.ShowError(fmt.Sprintf("No matches found for '%s'", s.filterText))
		} else {
			s.app.statusBar.ShowSuccess(fmt.Sprintf("Found %d matches for '%s'", totalMatches, s.filterText))
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

	if !s.app.dialogOpen {
		s.app.app.SetFocus(s.treeView)
	}
}

func (s *Sidebar) onNodeActivated(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*sidebarRef)
	if !ok {
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

func (s *Sidebar) onSelect(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*sidebarRef)
	if !ok {
		if node.GetText() == "+ New Connection" {
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
				s.app.statusBar.ShowError("Connection not found in config")
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
		case '-':
			s.CollapseAll()
			return nil
		case '+', '=':
			s.ExpandAll()
			return nil
		case '/':
			s.app.dialogs.ShowInputDialog("Filter Sidebar", "Filter text...", s.filterText, func(text string) {
				s.filterText = text
				s.RebuildTree()
			})
			return nil
		case 'c', 'C':
			s.app.ShowConnectionDialog(nil)
			return nil
		case 'n', 'N':
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && (ref.kind == "connection" || ref.kind == "database") {
				s.app.dialogs.ShowCreateDBDialog(ref.id)
			}
			return nil
		case 'r', 'R':
			s.ForceRefresh()
			return nil
		case 'd':
			node := s.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "connection" {
				s.app.Disconnect(ref.id)
			}
			return nil
		case 'D':
			node := s.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "database" {
				s.promptDropDB(ref)
			}
			return nil
		case 'f', 'F':
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowSearchDataDialog(ref)
			}
			return nil
		case 'a', 'A':
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowAddColumnDialog(ref.id, ref.db, ref.table)
			}
			return nil
		case 'm', 'M':
			node := s.treeView.GetCurrentNode()
			if ref, ok := node.GetReference().(*sidebarRef); ok && ref.kind == "table" {
				s.app.dialogs.ShowModifyColumnDialog(ref.id, ref.db, ref.table)
			}
			return nil
		case 'x', 'X':
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
					msg := fmt.Sprintf("Remove connection '%s' from saved list?", name)
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
					s.promptDropDB(ref)
					return nil
				}
			}
		}
	}

	return event
}

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

func (s *Sidebar) GetCurrentNode() *tview.TreeNode {
	return s.treeView.GetCurrentNode()
}

func (s *Sidebar) GetTreeView() *tview.TreeView {
	return s.treeView
}

type sidebarRef struct {
	kind  string
	id    string
	db    string
	table string
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
		cmd.Wait()
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

func (s *Sidebar) CollapseAll() {
	for _, connNode := range s.root.GetChildren() {
		connNode.SetExpanded(false)
		for _, dbNode := range connNode.GetChildren() {
			dbNode.SetExpanded(false)
		}
	}
	s.app.statusBar.ShowInfo("Collapsed all explorer nodes")
}

func (s *Sidebar) ExpandAll() {
	for _, connNode := range s.root.GetChildren() {
		connNode.SetExpanded(true)
		for _, dbNode := range connNode.GetChildren() {
			dbNode.SetExpanded(true)
		}
	}
	s.app.statusBar.ShowInfo("Expanded all explorer nodes")
}

func (s *Sidebar) ForceRefresh() {
	s.app.statusBar.ShowInfo("Refreshing databases and tables...")

	s.cachedTables = make(map[string][]model.TableInfo)

	activeConns := s.app.dbManager.GetActiveConnections()
	if len(activeConns) == 0 {
		s.RebuildTree()
		s.app.statusBar.ShowSuccess("Explorer refreshed")
		return
	}

	go func() {
		for _, connState := range activeConns {
			if connState.Connected {
				s.app.dbManager.RefreshDatabases(connState.Connection.ID)
			}
		}
		s.app.app.QueueUpdateDraw(func() {
			s.RebuildTree()

			for _, connState := range activeConns {
				if !connState.Connected {
					continue
				}
				curState := s.app.dbManager.GetConnectionState(connState.Connection.ID)
				if curState != nil {
					for _, dbName := range curState.Databases {
						cacheKey := connState.Connection.ID + "_" + dbName
						if _, isCached := s.cachedTables[cacheKey]; !isCached {
							s.cachedTables[cacheKey] = []model.TableInfo{}
							go s.ExpandDatabase(connState.Connection.ID, dbName)
						}
					}
				}
			}

			s.app.statusBar.ShowSuccess("Explorer refreshed successfully!")
		})
	}()
}


