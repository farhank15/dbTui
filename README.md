# dbTui ⚡🔌

[![Go Version](https://img.shields.io/github/go-mod/go-version/farhank15/dbTui?color=00ADD8&style=flat-square)](https://go.dev)
[![License](https://img.shields.io/github/license/farhank15/dbTui?color=blue&style=flat-square)](LICENSE)
[![Platform Support](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-0078d4?style=flat-square)](#-installation--distribution)
[![CGO Free](https://img.shields.io/badge/CGO-free-success?style=flat-square)](#)

**dbTui** is a blazing-fast, ultra-lightweight Terminal Database Client designed for seamless database management directly from your shell. Forget heavy, memory-hogging GUI applications—manage PostgreSQL, MySQL, and SQLite databases instantly with high performance and zero bloat.

> **Why dbTui?** Portable single-binary (~21MB), minimal memory footprint (~16MB idle), fully keyboard-driven, and packed with protective safety guardrails.

---

## ⚡ Key Features

- 🔌 **Multi-Engine Support** – Native, high-performance connectors for **PostgreSQL**, **MySQL**, and **SQLite** (CGO-free).
- 🎨 **Responsive TUI Layout** – Clean, color-coded schema explorer showing connection ➔ database ➔ table structure.
- ⌨️ **Keyboard-Driven Workflow** – Ergonomic shortcuts, global inputs, and multi-line SQL editor with command history.
- 🗄️ **Schema Navigator** – Seamlessly explore columns, data types, indexes, and foreign keys without manual metadata queries.
- 🛠️ **Visual Table Creator** – Interactive form to build tables including column definitions, types, nullability, primary keys, and auto-increment.
- 🛡️ **Built-in Safety Guardrails** – Automated 1000-row limit safety to prevent Out-Of-Memory (OOM) crashes, along with protective execution timeouts.
- 💾 **Self-Healing Config** – All database profiles are stored in `~/.dbTui/config.json`. Auto-repairs duplicate or corrupt connection IDs on startup.
- 📋 **Universal Clipboard Integration** – Copy table/column/cell data to system clipboard. Features **OSC 52 terminal copy fallback** if no clipboard utility (`xclip`/`wl-copy`) is installed.
- 📤 **Export to CSV** – Instantly save query results to a CSV file with a single keyboard shortcut.

---

## 📦 Installation & Distribution

### 1. Using `go install` (Fastest for Go Developers)
If you have Go 1.23+ installed, build and install the binary directly to your `$GOPATH/bin` with:
```bash
go install github.com/farhank15/dbTui/cmd/dbTui@latest
```

### 2. Using Automated Installer Script (`curl | sh`)
For Linux and macOS users, install `dbTui` instantly without installing Go or manual configuration:
```bash
curl -sSfL https://raw.githubusercontent.com/farhank15/dbTui/main/install.sh | sh
```
*This script auto-detects your OS and CPU architecture, downloads the latest pre-built binary release, and installs it into `/usr/local/bin` (falls back to `~/.local/bin` if run without root privileges).*

### 3. Pre-Compiled Binaries (No Source Code)
Download pre-compiled binaries from the **GitHub Releases** page:
1. Go to the repository's Releases page.
2. Download the archive for your OS (e.g. `dbTui_Linux_x86_64.tar.gz` or `dbTui_Windows_x86_64.zip`).
3. Extract and place the binary in your system `PATH` (e.g., `/usr/local/bin` or `C:\Windows\system32`).

### 4. Build from Source
```bash
# Clone the repository
git clone https://github.com/farhank15/dbTui
cd dbTui

# Clean and download dependencies
go mod tidy

# Run directly
go run ./cmd/dbTui

# Build local binary
go build -o dbTui ./cmd/dbTui
```

---

## ⌨️ Keyboard Shortcuts

### 🌐 Global Navigation
| Shortcut | Function |
| :--- | :--- |
| `Tab` | Switch focus between panels (Sidebar ⇄ Query Editor ⇄ Result Table) |
| `Esc` | Return focus to Sidebar / Close active modal dialog |
| `Ctrl + N` | Open "New Connection" form dialog |
| `Ctrl + D` | Disconnect the currently active database connection |
| `Ctrl + H` | Toggle active help information modal |
| `F5` | Refresh the explorer tree sidebar |

### 📁 Explorer Sidebar
| Shortcut | Function |
| :--- | :--- |
| `↑` / `↓` | Select connection, database, or table nodes |
| `→` / `←` | Expand / Collapse directories |
| `Enter` | Connect/Disconnect or load table structure detail |
| `/` | **Filter tables in database by name** |
| `y` / `Y` | **Copy selected database or table name to system clipboard** |
| `v` / `V` | **View Table DDL / schema DDL** |
| `f` / `F` | **Search table rows matching a specific column value** |
| `d` / `D` | Disconnect selected connection node |
| `Delete` | Remove connection from config / **Drop database with confirmation modal** |

### 📝 SQL Query Editor
| Shortcut | Function |
| :--- | :--- |
| `Ctrl + J` | Execute the written SQL query |
| `Ctrl + P` / `Ctrl + N` | Cycle backward / forward through query command history |
| `Ctrl + T` | **Open SQL templates / snippets list dialog** |
| `Ctrl + F` | Auto-format SQL query text |
| `Tab` | Focus results table |

### 📊 Result Table
| Shortcut | Function |
| :--- | :--- |
| `↑` / `↓` / `←` / `→` | Scroll through query results |
| `Enter` | **Inspect/View full cell value in a scrollable modal** |
| `e` / `E` | **Edit selected cell value inline (performs database UPDATE)** |
| `Ctrl + E` | Export active query results to CSV |
| `/` | Filter columns by name (when viewing table detail metadata) |

---

## 🏗️ Architecture

dbTui enforces clean architectural boundaries separating the TUI presentation layer from driver connectors:

```
dbTui/
├── cmd/
│   └── dbTui/main.go       # Application Entry Point
├── internal/
│   ├── tui/                 # 🎨 UI & Event Handlers (tview + tcell)
│   │   ├── app.go           # Central layout orchestrator & global shortcut listener
│   │   ├── sidebar.go       # Tree view schema navigator (Conn ➔ DB ➔ Table)
│   │   ├── query_panel.go   # Multi-line SQL text editor
│   │   ├── result_table.go  # Query result grid & metadata details viewer
│   │   └── dialogs.go       # Form modals: new connection, table creator, confirms
│   ├── db/                  # 🗄️ Thread-Safe Database Drivers
│   │   ├── connector.go     # Universal driver interface
│   │   ├── postgres.go      # PostgreSQL adapter (via pgx)
│   │   ├── mysql.go         # MySQL adapter (via go-sql-driver)
│   │   └── sqlite.go        # SQLite CGO-Free adapter
│   └── config/              # ⚙️ Configuration Manager
│       └── config.go        # JSON parser with unique ID Self-Healing validation
```

---

## 🛡️ Safety & Stability Features

| Feature | Technical Detail |
| :--- | :--- |
| **Row Limit Safety** | Restricts `MaxDisplayRows = 1000` to prevent terminal memory exhaust (OOM) on massive tables. |
| **Query Timeout** | Automatic query execution timeout at 30 seconds to prevent hanging zombie database processes. |
| **Connection Locks** | Safe concurrency operations under multiple background tasks using `sync.RWMutex`. |
| **Self-Healing Config** | Automatically detects and repairs overlapping connection IDs in `config.json` on app startup. |
| **OSC 52 Clipboard** | Copy data natively even on headless servers/containers or SSH sessions without `xclip`. |

---

## 📊 Feature Comparison

| Feature / Metric | **dbTui** ⚡ | DBeaver | lazysql | usql |
| :--- | :---: | :---: | :---: | :---: |
| **Interface** | **TUI (Terminal)** | GUI (Desktop) | TUI (Terminal) | CLI (Command Line) |
| **Binary Size** | **~21 MB** | >300 MB | ~15 MB | ~25 MB |
| **RAM (Idle)** | **~16 MB** | >500 MB | ~20 MB | ~10 MB |
| **PostgreSQL & MySQL** | ✅ | ✅ | ✅ | ✅ |
| **SQLite (CGO-free)** | ✅ | ✅ | ❌ | ❌ |
| **Visual Table Creator**| ✅ | ✅ | ❌ | ❌ |
| **CSV Exporter** | ✅ | ✅ | ❌ | ✅ |
| **Auto-Healing Config** | ✅ | ❌ | ❌ | ❌ |

---

## 📝 License

Distributed under the **MIT** License. See the [LICENSE](LICENSE) file for more information.

---

> Built with ❤️ using **Go** and **tview**. Fast, lightweight, and robust on any system!
