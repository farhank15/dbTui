# dbTui Agent Mode — PRD

**Status**: Draft v2  
**Versi Target**: v1.0.0 (pertama dengan agent mode)  
**Integrasi Dengan**: Fennec DB Observation (v1.16.0)

---

## 1. Ringkasan

Menambahkan mode **agent** ke dbTui — mode headless yang komunikasi via JSON-RPC over stdin/stdout. Mode ini memungkinkan dbTui dipanggil sebagai child process oleh fennec (atau tools lain) untuk eksekusi query, schema inspection, dan health check, tanpa perlu TUI.

dbTui tetap jadi binary tunggal: `dbTui` → TUI (default), `dbTui --agent` → headless JSON-RPC.

---

## 2. Filosofi

| Prinsip | Implementasi |
|---|---|
| **Satu binary, dua mode** | Dispatch via flag `--agent`, gak perlu build terpisah |
| **Zero additional dependency** | Agent mode cuma pake stdlib + existing DB drivers |
| **Reuse, not rewrite** | Agent pake `db.Manager`, `model`, `Connector` yang sama |
| **Stateless per request** | Connection state di memory, gak ada file config |
| **Death cleanup** | Kill process → semua connection nutup otomatis |
| **Defense in depth** | Strict mode di-enforce di agent level, bukan cuma di client |

---

## 3. Arsitektur

```
┌──────────────────────────────────────────────────────────────┐
│ dbTui Binary                                                  │
│                                                               │
│  main.go                                                      │
│    ├── tanpa flag → tui.NewApp().Run()    (TUI mode)          │
│    └── --agent     → agent.NewServer().Run() (Agent mode)     │
│                                                               │
│  ┌──────────────────────┐   ┌──────────────────────────────┐  │
│  │    TUI Mode           │   │    Agent Mode                 │  │
│  │  (tview + tcell)      │   │  (JSON-RPC stdin/stdout)      │  │
│  │                       │   │                               │  │
│  │  App                  │   │  AgentServer                  │  │
│  │   ├── Sidebar         │   │   ├── readLine(stdin)         │  │
│  │   ├── QueryPanel      │   │   ├── parseRequest            │  │
│  │   ├── ResultTable     │   │   ├── dispatch → Manager      │  │
│  │   └── Dialogs         │   │   └── writeLine(stdout)       │  │
│  └──────────────────────┘   └──────────────────────────────┘  │
│                ┌──────────────────────────────┐                │
│                │    Shared Core                │                │
│                │  (internal/db + internal/model)│               │
│                │                               │                │
│                │  Manager                      │                │
│                │   ├── Connect(config)          │                │
│                │   ├── Disconnect(id)           │                │
│                │   ├── GetConnector(id)         │                │
│                │   └── GetActiveConnections()   │                │
│                │                               │                │
│                │  Connectors (reuse 100%)       │                │
│                │   ├── PostgresConnector       │                │
│                │   ├── MySQLConnector          │                │
│                │   └── SQLiteConnector         │                │
│                │                               │                │
│                │  New: internal/db/dsn.go      │                │
│                │   └── ParseDSN(url)            │                │
│                └──────────────────────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

### Alur Hidup Satu Request

```
Agent Running (blocking on stdin)
    ↓
Read line from stdin (bufio.Scanner)
    ↓
json.Unmarshal → Request struct
    ├─ JSON parse error → kirim error -32005, lanjut
    └─ OK → lanjut
    ↓
Match method name
    ├─ unknown method → kirim error -32601 (Method not found)
    └─ known → dispatch
    ↓
Validate params (json.Unmarshal into method-specific struct)
    ├─ missing required field → kirim error -32005
    └─ OK → execute
    ↓
Execute via db.Manager / Connector
    ├─ error → kirim error code sesuai tipe
    └─ OK → kirim result
    ↓
Write response line to stdout (json.Encoder)
    ↓
Loop back to read
```

---

## 4. Mode Dispatch (main.go)

Current `main.go`:
```go
func main() {
    app := tui.NewApp()
    if err := app.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

New `main.go`:
```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "--agent" {
        server := agent.NewServer()
        if err := server.Run(); err != nil {
            fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
            os.Exit(1)
        }
        return
    }
    app := tui.NewApp()
    if err := app.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

Flag `--agent` adalah flag pertama. Argument lain setelah `--agent`:
- `--agent` → stdin/stdout JSON-RPC (default, untuk fennec)
- `--agent --tcp :9876` → TCP listener (future)

---

## 5. Agent Package (`internal/agent/`)

### 5.1 Go Types (`types.go`)

```go
package agent

// --- JSON-RPC 2.0 ---

// Request represents a JSON-RPC 2.0 request
type Request struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      json.Number     `json:"id"`          // number or string, use json.Number
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params"`       // raw for flexible dispatch
}

// Response represents a JSON-RPC 2.0 response
type Response struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      json.Number `json:"id"`
    Result  interface{} `json:"result,omitempty"`
    Error   *ErrorObj   `json:"error,omitempty"`
}

// ErrorObj represents a JSON-RPC 2.0 error object
type ErrorObj struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes
const (
    ErrParse          = -32700  // Invalid JSON
    ErrInvalidRequest = -32600  // Invalid Request object
    ErrMethodNotFound = -32601  // Method not found
    ErrInvalidParams  = -32602  // Invalid method parameter(s)
    ErrInternal       = -32603  // Internal JSON-RPC error
)

// Custom error codes (dbTui-specific)
const (
    ErrConnectionFailed   = -32000  // connect gagal
    ErrUnsupportedType    = -32001  // URL scheme bukan postgres/mysql/sqlite
    ErrNotConnected       = -32002  // query tanpa connect dulu
    ErrWriteQueryBlocked  = -32003  // strict mode
    ErrQueryTimeout       = -32004  // query >30s
    ErrInvalidParams      = -32005  // params validation failed
    ErrInternalError      = -32006  // panic/unexpected
)

// --- Method-specific param structs ---

type PingParams struct{}

type ConnectParams struct {
    Name string `json:"name"`     // connection name (required)
    URL  string `json:"url"`      // DATABASE_URL format (required)
}

type DisconnectParams struct {
    Name string `json:"name"`     // connection name (required)
}

type ListParams struct{}

type QueryParams struct {
    Name    string `json:"name"`     // connection name (required)
    SQL     string `json:"sql"`      // query string (required)
    MaxRows *int   `json:"maxRows"`  // optional, default 1000
    Strict  *bool  `json:"strict"`   // optional, default true
}

type SchemaParams struct {
    Name     string `json:"name"`     // connection name (required)
    Database string `json:"database"` // optional, filter specific database
}

type TablesParams struct {
    Name     string `json:"name"`     // connection name (required)
    Database string `json:"database"` // optional, filter specific database
}

type ExplainParams struct {
    Name string `json:"name"`     // connection name (required)
    SQL  string `json:"sql"`      // query string (required)
}

type StatsParams struct {
    Name string `json:"name"`     // connection name (required)
}

// --- Response result structs ---

type PingResult struct {
    Ok      bool   `json:"ok"`
    Version string `json:"version"`
    Uptime  string `json:"uptime"`
}

type ConnectResult struct {
    Connected     bool   `json:"connected"`
    Type          string `json:"type"`
    Version       string `json:"version,omitempty"`
    Database      string `json:"database,omitempty"`
    DefaultSchema string `json:"defaultSchema,omitempty"`
}

type DisconnectResult struct {
    Disconnected bool `json:"disconnected"`
}

type ConnectionInfo struct {
    Name     string `json:"name"`
    Type     string `json:"type"`
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Database string `json:"database"`
    Connected bool  `json:"connected"`
}

type ListResult struct {
    Connections []ConnectionInfo `json:"connections"`
}

// QueryRow represents a single row with nullable values
// Using *string so that NULL becomes JSON null
type QueryRow []*string

type QueryResult struct {
    Columns   []string  `json:"columns"`
    Rows      []QueryRow `json:"rows"`
    Duration  string    `json:"duration"`
    RowCount  int       `json:"rowCount"`
    IsSelect  bool      `json:"isSelect"`
    Truncated bool      `json:"truncated"`
}

type ColumnResult struct {
    Name    string `json:"name"`
    Type    string `json:"type"`
    Nullable string `json:"nullable"`
    Key     string `json:"key"`
    Default *string `json:"default"`
    Extra   string `json:"extra"`
}

type IndexResult struct {
    Name    string   `json:"name"`
    Columns []string `json:"columns"`
    Unique  bool     `json:"unique"`
    Primary bool     `json:"primary"`
}

type FKResult struct {
    Name      string `json:"name"`
    Column    string `json:"column"`
    RefTable  string `json:"refTable"`
    RefColumn string `json:"refColumn"`
    OnDelete  string `json:"onDelete"`
    OnUpdate  string `json:"onUpdate"`
}

type TableResult struct {
    Name        string         `json:"name"`
    Columns     []ColumnResult `json:"columns"`
    Indexes     []IndexResult  `json:"indexes"`
    ForeignKeys []FKResult     `json:"foreignKeys"`
    RowCount    int64          `json:"rowCount"`
}

type DatabaseResult struct {
    Name   string         `json:"name"`
    Tables []TableResult  `json:"tables"`
}

type SchemaResult struct {
    Databases []DatabaseResult `json:"databases"`
}

type TableInfoResult struct {
    Name     string `json:"name"`
    RowCount int64  `json:"rowCount"`
}

type TablesResult struct {
    Tables []TableInfoResult `json:"tables"`
}

type ExplainResult struct {
    Plan     string `json:"plan"`
    Duration string `json:"duration"`
}

type StatsResult struct {
    Database          string  `json:"database"`
    SizeMB            float64 `json:"sizeMB"`
    TableCount        int     `json:"tableCount"`
    ActiveConnections int     `json:"activeConnections"`
    Uptime            string  `json:"uptime,omitempty"`
}
```

### 5.2 Server (`server.go`)

```go
package agent

import (
    "bufio"
    "encoding/json"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/farhank15/dbTui/internal/db"
)

const (
    AgentVersion  = "1.0.0"
    DefaultMaxRows = 1000
    QueryTimeout   = 30 * time.Second
)

type Server struct {
    manager *db.Manager
    scanner *bufio.Scanner  // stdin
    writer  *json.Encoder   // stdout
    logger  *log.Logger     // stderr
    startAt time.Time
}

func NewServer() *Server {
    return &Server{
        manager: db.NewManager(),
        scanner: bufio.NewScanner(os.Stdin),
        writer:  json.NewEncoder(os.Stdout),
        logger:  log.New(os.Stderr, "", log.LstdFlags),
        startAt: time.Now(),
    }
}

func (s *Server) Run() error {
    s.log("[INFO] agent started (pid=%d)", os.Getpid())

    // Signal handling for graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        sig := <-sigCh
        s.log("[INFO] received signal: %v, shutting down", sig)
        s.cleanup()
        os.Exit(0)
    }()

    // Increase scanner buffer for large queries (default 64KB, max 10MB)
    s.scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

    // Main read loop
    for s.scanner.Scan() {
        line := s.scanner.Text()
        if line == "" {
            continue
        }
        resp := s.handleRaw(line)
        if err := s.writer.Encode(resp); err != nil {
            s.log("[ERROR] failed to write response: %v", err)
        }
    }

    // stdin EOF → parent process exited
    if err := s.scanner.Err(); err != nil {
        s.log("[ERROR] scanner error: %v", err)
    }
    s.log("[INFO] stdin closed, shutting down")
    s.cleanup()
    return nil
}

func (s *Server) handleRaw(line string) Response {
    defer func() {
        if r := recover(); r != nil {
            s.log("[PANIC] %v", r)
        }
    }()

    var req Request
    if err := json.Unmarshal([]byte(line), &req); err != nil {
        return newError(ErrParse, "invalid JSON", err.Error())
    }

    if req.JSONRPC != "2.0" {
        return newError(ErrInvalidRequest, "invalid jsonrpc version", nil)
    }

    return s.handleRequest(req)
}

func (s *Server) handleRequest(req Request) Response {
    switch req.Method {
    case "ping":
        return handlePing(s)
    case "connect":
        return handleConnect(s, req.Params)
    case "disconnect":
        return handleDisconnect(s, req.Params)
    case "list":
        return handleList(s)
    case "query":
        return handleQuery(s, req.Params)
    case "schema":
        return handleSchema(s, req.Params)
    case "tables":
        return handleTables(s, req.Params)
    case "explain":
        return handleExplain(s, req.Params)
    case "stats":
        return handleStats(s, req.Params)
    default:
        return newError(ErrMethodNotFound, "unknown method: "+req.Method, nil)
    }
}

func (s *Server) cleanup() {
    for _, state := range s.manager.GetActiveConnections() {
        name := state.Connection.Name
        if err := s.manager.Disconnect(state.Connection.ID); err != nil {
            s.log("[WARN] disconnect %s: %v", name, err)
        } else {
            s.log("[INFO] disconnected %s", name)
        }
    }
}

func (s *Server) log(format string, v ...interface{}) {
    s.logger.Printf(format, v...)
}

// Helpers

func newError(code int, message string, data interface{}) Response {
    return Response{
        JSONRPC: "2.0",
        ID:      "0",
        Error: &ErrorObj{
            Code:    code,
            Message: message,
            Data:    data,
        },
    }
}

func success(id json.Number, result interface{}) Response {
    return Response{
        JSONRPC: "2.0",
        ID:      id,
        Result:  result,
    }
}

func redactURL(rawURL string) string {
    // postgres://user:pass@host/db → postgres://user:***@host/db
    u, err := url.Parse(rawURL)
    if err != nil {
        return rawURL
    }
    if u.User != nil {
        u.User = url.User(u.User.Username())
    }
    return u.String()
}
```

Handler signatures:
```go
func handlePing(s *Server) Response
func handleConnect(s *Server, raw json.RawMessage) Response
func handleDisconnect(s *Server, raw json.RawMessage) Response
func handleList(s *Server) Response
func handleQuery(s *Server, raw json.RawMessage) Response
func handleSchema(s *Server, raw json.RawMessage) Response
func handleTables(s *Server, raw json.RawMessage) Response
func handleExplain(s *Server, raw json.RawMessage) Response
func handleStats(s *Server, raw json.RawMessage) Response
```

### 5.3 Protocol: JSON-RPC 2.0 over stdin/stdout

Satu request = satu line JSON. Satu response = satu line JSON.  
Newline (`\n`) sebagai delimiter. Ini penting biar gampang di-parse oleh fennec.

**Request:**
```json
{"jsonrpc":"2.0","id":1,"method":"connect","params":{"name":"mypg","url":"postgres://user:pass@localhost:5432/mydb?sslmode=disable"}}
```

**Response sukses:**
```json
{"jsonrpc":"2.0","id":1,"result":{"connected":true,"type":"postgres","version":"16.4"}}
```

**Response error:**
```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"connection refused","data":"dial tcp 127.0.0.1:5432: connect: connection refused"}}
```

**Notifications (requests tanpa `id`)**: TIDAK didukung. Semua request WAJIB punya `id`. Agent akan return error `-32600` jika `id` kosong.

### 5.4 Method Specifications

#### `ping`
Test koneksi agent (tanpa perlu koneksi DB). Paling cepat — gak pake db.Manager sama sekali.

```
→ {"method":"ping","params":{}}
← {"result":{"ok":true,"version":"1.0.0","uptime":"12s"}}
```

Implementation: hitung `time.Since(s.startAt)` → format duration human-readable.

**Version constant**: `agent.AgentVersion = "1.0.0"`. Dijaga manual, update tiap rilis.

#### `connect`
Membuat koneksi baru ke database. URL dalam format DATABASE_URL.

```
→ {"method":"connect","params":{"name":"mypg","url":"postgres://user:pass@localhost:5432/mydb"}}
← {"result":{"connected":true,"type":"postgres","version":"16.4","database":"mydb","defaultSchema":"public"}}

Error:
← {"error":{"code":-32000,"message":"connection refused"}}
← {"error":{"code":-32001,"message":"unsupported database type","data":"only postgres, mysql, sqlite supported"}}
```

**URL format:**
```
postgres://user:pass@host:port/dbname?sslmode=disable
mysql://user:pass@host:port/dbname?charset=utf8mb4
sqlite:///path/to/file.db
sqlite://./relative.db
```

**Field mapping (URL → model.Connection):**

| URL Part | model.Connection Field | Contoh |
|---|---|---|
| `scheme` | `.Type` | `postgres` → `TypePostgres` |
| `user` | `.User` | `postgres` |
| `password` | `.Password` | `secret123` |
| `host` | `.Host` | `localhost` |
| `port` | `.Port` | `5432` |
| `path` | `.Database` (PG/MySQL) or `.File` (SQLite) | `/mydb` or `/path/to/file.db` |
| `?sslmode=` | `.SSLMode` | `disable` |

**Name → ID mapping**: Agent set `config.ID = config.Name` agar Manager bisa lookup by name.

**Duplicate behavior**: Jika `name` sudah terdaftar sebagai koneksi aktif, agent akan:
1. Disconnect koneksi lama
2. Buat koneksi baru dengan nama yang sama (replace)

**Version detection**: Menggunakan `SELECT version()` (Postgres/MySQL) atau `SELECT sqlite_version()` (SQLite) setelah konek.

**Logging (redacted)**:
```
[INFO] connect mypg -> postgres://postgres:***@localhost:5432/mydb (8ms)
```

#### `disconnect`
Nutup koneksi.

```
→ {"method":"disconnect","params":{"name":"mypg"}}
← {"result":{"disconnected":true}}

Not found:
← {"error":{"code":-32002,"message":"connection not found: mypg"}}
```

#### `list`
Nampilin koneksi aktif. Password NEVER ada di output.

```
→ {"method":"list","params":{}}
← {"result":{"connections":[{"name":"mypg","type":"postgres","host":"localhost","port":5432,"database":"mydb","connected":true}]}}
```

Implementation: iterate `s.manager.GetActiveConnections()` → map ke `ConnectionInfo`.

#### `query`
Execute SQL query. **Read-only by default** (strict mode).

```
→ {"method":"query","params":{"name":"mypg","sql":"SELECT * FROM users LIMIT 10","maxRows":1000,"strict":true}}
← {"result":{"columns":["id","name","email"],"rows":[["1","Alice","a@b.com"],["2","Bob",null]],"duration":"12ms","rowCount":2,"isSelect":true,"truncated":false}}

Write (strict=true → error):
← {"error":{"code":-32003,"message":"write queries disabled in strict mode","data":"DELETE FROM users"}}

Timeout:
← {"error":{"code":-32004,"message":"query timeout (30s)"}}
```

**Defaults:**
| Field | Default | Notes |
|---|---|---|
| `strict` | `true` | Block write queries |
| `maxRows` | `1000` | Safety limit, row count cap |

**Strict mode:**
Mendeteksi write query dari kata pertama SQL. Block list:
```
INSERT, UPDATE, DELETE, DROP, CREATE, ALTER, TRUNCATE, REPLACE, CALL
```
SELECT, WITH, PRAGMA, SHOW, DESCRIBE, EXPLAIN → allowed.

Strict check dilakukan di agent handler SEBELUM query dikirim ke connector. Defense in depth.

**maxRows behavior:**
- `maxRows` diterapkan di agent level, BUKAN di connector.
- Query tetap execute di DB, hasilnya di-truncate di agent sebelum dikirim.
- Jika hasil > maxRows, field `truncated: true`.
- Ini berbeda dengan `MaxDisplayRows` (50000) yang ada di connector — agent punya limit sendiri.

**NULL handling:**
- `QueryResult.Rows` menggunakan `[]*string` (bukan `[]string`). 
- NULL dari database → `nil` → `null` di JSON.
- Value non-NULL → `*string` → string di JSON.
- Konversi dilakukan di agent handler saat mapping dari `[][]string` (dari connector) ke `[]*string`.

**Edge cases:**
- Query dengan 0 baris: `{"columns":[],"rows":[],"rowCount":0,"isSelect":true}`
- Query write di non-strict: `{"message":"Query OK, 1 row(s) affected","isSelect":false}`
- Koneksi gak ditemukan: error -32002
- Syntax error balikin dari driver: error -32006

#### `schema`
Full schema inspection — databases, tables, columns, indexes, foreign keys, row count.

```
→ {"method":"schema","params":{"name":"mypg","database":"mydb"}}
← {"result":{"databases":[{"name":"mydb","tables":[{"name":"users","columns":[{"name":"id","type":"integer","nullable":"NO","key":"PRI","default":null,"extra":"auto_increment"}],"indexes":[{"name":"idx_email","columns":["email"],"unique":true}],"foreignKeys":[{"name":"fk_orders_user","column":"user_id","refTable":"users","refColumn":"id","onDelete":"CASCADE"}],"rowCount":100}]}]}}

Tanpa database filter:
→ {"method":"schema","params":{"name":"mypg"}}
← (sama, tapi semua database)
```

Implementation:
1. Panggil `connector.GetDatabases()` → dapat list database
2. Jika `params.database` diisi, filter hanya database itu
3. Loop tiap database → `GetTables(dbName)` → dapat list table
4. Loop tiap table → `GetTableDetail(dbName, tableName)` → dapat columns + indexes + FKs
5. Loop tiap table → `GetRowCount(dbName, tableName)` → dapat row count
6. Gabungin ke `SchemaResult`

**Caching**: Tidak di agent. Caching (30s TTL) dilakukan di fennec side.

**Perf note**: Untuk database dengan banyak tabel (50+), query ini bisa lambat. Agent gak perlu optimasi — fennec yang manage cache.

#### `tables`
List tables aja (lebih ringan dari schema — gak include columns/indexes/FKs).

```
→ {"method":"tables","params":{"name":"mypg","database":"mydb"}}
← {"result":{"tables":[{"name":"users","rowCount":100},{"name":"orders","rowCount":50}]}}

Tanpa database filter:
→ {"method":"tables","params":{"name":"mypg"}}
← {"result":{"tables":[{"name":"users","rowCount":100},{"name":"orders","rowCount":50}]}}
-- list table dari semua database, dengan prefix "dbname.tablename" atau grouped
```

Implementation:
1. Panggil `connector.GetDatabases()`
2. Jika `params.database` diisi, filter
3. Loop tiap database → `GetTables(dbName)` → dapat `[]TableInfo`
4. Loop tiap table → `GetRowCount(dbName, tableName)` → dapat row count
5. Gabungin ke `TablesResult`

#### `explain`
Query execution plan.

```
→ {"method":"explain","params":{"name":"mypg","sql":"SELECT * FROM users WHERE id = 1"}}
← {"result":{"plan":"Index Scan using users_pkey on users  (cost=0.15..8.17 rows=1 width=36)","duration":"3ms"}}
```

Implementation per driver:
- **Postgres**: `EXPLAIN (ANALYZE false, FORMAT JSON) <query>` → parse JSON output, ambil `Plan` node
- **MySQL**: `EXPLAIN FORMAT=JSON <query>` → parse JSON output
- **SQLite**: `EXPLAIN QUERY PLAN <query>` → baca output tabular sebagai string

Agent menggunakan `connector.GetDB().Exec()` untuk explain queries (bukan `ExecuteQuery` karena EXPLAIN bukan query biasa).

Fallback: Jika EXPLAIN gagal (misal query punya syntax error), return raw error message.

#### `stats`
Database statistics.

```
→ {"method":"stats","params":{"name":"mypg"}}
← {"result":{"database":"mydb","sizeMB":12.5,"tableCount":8,"activeConnections":1,"uptime":"5m12s"}}
```

Implementation per driver:

**Postgres:**
```sql
-- ukuran database
SELECT pg_database_size(current_database()) AS bytes;
-- jumlah tabel (public schema)
SELECT count(*) FROM information_schema.tables
  WHERE table_schema NOT IN ('pg_catalog', 'information_schema');
-- connection count
SELECT count(*) FROM pg_stat_activity WHERE datname = current_database();
```

**MySQL:**
```sql
-- ukuran database
SELECT SUM(data_length + index_length) AS bytes
  FROM information_schema.tables WHERE table_schema = DATABASE();
-- jumlah tabel
SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE();
-- connection count
SELECT count(*) FROM information_schema.processlist;
```

**SQLite:**
```go
// ukuran: page_count * page_size
var pageCount, pageSize int64
db.QueryRow("PRAGMA page_count").Scan(&pageCount)
db.QueryRow("PRAGMA page_size").Scan(&pageSize)
sizeMB := float64(pageCount*pageSize) / (1024 * 1024)

// jumlah tabel (exclude sqlite_*)
rows, _ := db.Query("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
```

Koneksi `activeConnections` untuk SQLite selalu 1 (single connection).

### 5.5 Error Codes

| Code | Standard | Message | Kapan |
|---|---|---|---|
| -32700 | ✅ JSON-RPC | Parse error | JSON gak valid |
| -32600 | ✅ JSON-RPC | Invalid Request | `jsonrpc` bukan "2.0", atau `id` kosong |
| -32601 | ✅ JSON-RPC | Method not found | Method gak dikenal |
| -32000 | ❌ Custom | connection failed | `connect` gagal (ECONNREFUSED, timeout, bad auth) |
| -32001 | ❌ Custom | unsupported type | URL scheme bukan postgres/mysql/sqlite |
| -32002 | ❌ Custom | not connected | `query`/`schema` tanpa `connect` dulu |
| -32003 | ❌ Custom | write query blocked | strict mode + INSERT/UPDATE/DELETE/dll |
| -32004 | ❌ Custom | query timeout | >30s |
| -32005 | ❌ Custom | invalid params | Missing required field, tipe mismatch |
| -32006 | ❌ Custom | internal error | SQL syntax error, panic, unexpected |

### 5.6 Concurrent Request Handling

- Agent blocking per request. Satu request selesai dulu baru baca request berikutnya.
- Gak perlu concurrency — fennec juga blocking (await tiap tool call).
- Manager menggunakan `sync.RWMutex` — aman dari race condition kalo suatu saat butuh concurrent.

---

## 6. File Changes

### New files:

| File | Baris (estimasi) | Isi |
|---|---|---|
| `cmd/dbTui/main.go` | ~40 (edit) | Dispatch TUI vs agent |
| `internal/agent/server.go` | ~130 | Server struct, Run loop, helpers |
| `internal/agent/types.go` | ~160 | Semua type definitions |
| `internal/agent/ping.go` | ~20 | Ping handler |
| `internal/agent/connect.go` | ~60 | Connect + DSN handler |
| `internal/agent/query.go` | ~90 | Query handler, strict detection, NULL conversion |
| `internal/agent/schema.go` | ~50 | Schema explorer handler |
| `internal/agent/tables.go` | ~40 | Tables listing handler |
| `internal/agent/explain.go` | ~60 | EXPLAIN handler per driver |
| `internal/agent/stats.go` | ~100 | Stats handler per driver |
| `internal/db/dsn.go` | ~70 | ParseDSN function |
| **Total** | **~820** | |

### Modified files:

| File | Perubahan |
|---|---|
| `cmd/dbTui/main.go` | Tambah dispatch `--agent` |
| `.goreleaser.yaml` | Gak perlu diubah — binary tetap satu |

### No changes needed:

- `internal/db/postgres.go` — reuse langsung
- `internal/db/mysql.go` — reuse langsung
- `internal/db/sqlite.go` — reuse langsung
- `internal/db/manager.go` — reuse langsung
- `internal/db/connector.go` — interface tetap
- `internal/model/*.go` — reuse langsung
- `internal/config/config.go` — gak dipake di agent mode
- `internal/tui/*.go` — gak dipake di agent mode

---

## 7. Dependencies

### Agent mode dependencies (existing, no new installs):
```
github.com/jackc/pgx/v5          ← Postgres (existing)
github.com/go-sql-driver/mysql   ← MySQL (existing)
modernc.org/sqlite               ← SQLite (existing)
```

### Agent mode stdlib only:
```
bufio       ← read stdin line by line
encoding/json ← JSON-RPC
fmt         ← format
io          ← stdin/stdout
log         ← stderr logging
net/url     ← parse DATABASE_URL
os          ← exit, signal
os/signal   ← graceful shutdown
strings     ← trim, prefix check
sync        ← Manager thread safety (existing)
syscall     ← SIGINT, SIGTERM
time        ← timeout, duration
```

### Agent mode does NOT import:
```
github.com/gdamore/tcell/v2
github.com/rivo/tview
github.com/mattn/go-runewidth
... (semua TUI dependency gak dipakai)
```

Note: Karena Go compile static, TUI deps tetap masuk binary tapi gak di-load ke memory pas `--agent`. Size impact negligible (~100KB dari total ~21MB).

---

## 8. Error Handling

### Connection errors:
```
- ECONNREFUSED → {"code":-32000,"message":"connection refused (localhost:5432)"}
- ETIMEOUT     → {"code":-32000,"message":"connection timed out (10s)"}
- ENOENT (SQLite) → {"code":-32000,"message":"database file not found: /path/to/db"}
- bad auth     → {"code":-32000,"message":"password authentication failed for user \"postgres\""}
```

### Query errors:
```
- syntax error → {"code":-32006,"message":"syntax error at or near \"SELET\""}
- timeout      → {"code":-32004,"message":"query timed out after 30s"}
- strict mode  → {"code":-32003,"message":"write queries disabled in strict mode"}
- not connected→ {"code":-32002,"message":"not connected: mypg"}
```

### JSON errors:
```
- parse error  → {"code":-32700,"message":"Parse error","data":"invalid character '}' ..."}
- unknown method→{"code":-32601,"message":"unknown method: run"}
```

### Panic recovery:
Semua handler dibungkus `defer recover()` di `handleRaw()` → kalo panic, balikin error -32006 plus stack trace (di stderr, gak di JSON response).

```go
func (s *Server) handleRaw(line string) Response {
    defer func() {
        if r := recover(); r != nil {
            s.log("[PANIC] %v", r)
            // stack trace
            buf := make([]byte, 4096)
            n := runtime.Stack(buf, false)
            s.log("[PANIC] stack:\n%s", buf[:n])
        }
    }()
    // ...
}
```

---

## 9. Logging

### Agent mode log ke stderr (stdout reserved untuk JSON-RPC):

Format:
```
2026/07/16 10:00:00 [INFO] agent started (pid=12345)
2026/07/16 10:00:01 [INFO] connect mypg -> postgres://postgres:***@localhost:5432/mydb (8ms)
2026/07/16 10:00:02 [INFO] query mypg: SELECT * FROM users LIMIT 10 (12ms, 10 rows)
2026/07/16 10:00:05 [INFO] disconnect mypg
2026/07/16 10:00:10 [INFO] agent shutdown (uptime=10s)
```

Gunakan `log.New(os.Stderr, "", log.LstdFlags)` — gak perlu library tambahan.

Connection URL di log harus **diredact** — password diganti `***`:
```
postgres://postgres:***@localhost:5432/mydb
```

Gunakan helper `redactURL()` yang parse URL, replace password dengan `***`, lalu `.String()`.

Log level:
- `[INFO]` — started, connect/disconnect success, query success
- `[WARN]` — disconnect gagal, non-fatal error
- `[ERROR]` — write response gagal, scanner error
- `[PANIC]` — panic recovery

---

## 10. Graceful Shutdown

Agent harus nutup semua connection kalo:
1. stdin nutup (EOF) — parent process mati
2. SIGTERM/SIGINT diterima
3. Error fatal

```go
func (s *Server) Run() error {
    s.log("[INFO] agent started (pid=%d)", os.Getpid())

    // Goroutine: listen signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        sig := <-sigCh
        s.log("[INFO] shutdown (signal=%v)", sig)
        s.cleanup()
        os.Exit(0)
    }()

    // Main loop: read stdin
    s.scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
    for s.scanner.Scan() {
        // handle request
    }
    // stdin EOF → parent process exited
    if err := s.scanner.Err(); err != nil {
        s.log("[ERROR] scanner error: %v", err)
    }
    s.cleanup()
    return nil
}

func (s *Server) cleanup() {
    for _, state := range s.manager.GetActiveConnections() {
        name := state.Connection.Name
        if err := s.manager.Disconnect(state.Connection.ID); err != nil {
            s.log("[WARN] disconnect %s: %v", name, err)
        } else {
            s.log("[INFO] disconnected %s", name)
        }
    }
}
```

**Why `os.Exit(0)` di signal handler?** Karena `bufio.Scanner.Scan()` blocking di `os.Stdin.Read()`. Signal handler gak bisa return dari main loop. `os.Exit(0)` setelah cleanup adalah pendekatan paling bersih.

---

## 11. Binary & Release

### Current goreleaser (gak perlu diubah):
```yaml
builds:
  - env: [CGO_ENABLED=0]
    goos: [linux, windows, darwin]
    goarch: [amd64, arm64]
    main: ./cmd/dbTui
    binary: dbTui
```

Binary tetap satu: `dbTui` — mode dispatch via `--agent` flag.

### Release strategy:
```
v1.0.0 → rilis pertama dengan agent mode
         Changelog: Added --agent mode for JSON-RPC over stdin/stdout
         agent.AgentVersion = "1.0.0"
```

### Download URL (untuk fennec auto-download):
```
https://github.com/farhank15/dbTui/releases/download/v1.0.0/dbTui_Linux_x86_64.tar.gz
https://github.com/farhank15/dbTui/releases/download/v1.0.0/dbTui_Linux_arm64.tar.gz
https://github.com/farhank15/dbTui/releases/download/v1.0.0/dbTui_Darwin_x86_64.tar.gz
https://github.com/farhank15/dbTui/releases/download/v1.0.0/dbTui_Darwin_arm64.tar.gz
https://github.com/farhank15/dbTui/releases/download/v1.0.0/dbTui_Windows_x86_64.zip
(arm64 Windows: future)
```

Goreleaser auto-generate pattern:
```
{{ .ProjectName }}_{{ .Os }}_{{ if eq .Arch "amd64" }}x86_64{{ else }}{{ .Arch }}{{ end }}.tar.gz
```

### Checksum:
Goreleaser auto-generate `checksums.txt` — fennec harus verify checksum sebelum pake binary.

```go
// Fennec side
func verifyChecksum(binaryPath string, expectedSha256 string) bool {
    data, _ := os.ReadFile(binaryPath)
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:]) == expectedSha256
}
```

---

## 12. Testing

### Unit test (new):
| File | Test |
|---|---|
| `internal/agent/handler_test.go` | Parse request, validasi params, dispatch method, unknown method |
| `internal/agent/ping_test.go` | Ping response format |
| `internal/agent/connect_test.go` | Parse URL → Connection, duplicate name, invalid URL |
| `internal/agent/query_test.go` | Strict mode detection, maxRows truncation, NULL conversion |
| `internal/db/dsn_test.go` | URL parsing edge cases (no port, no password, sqlite path, query params) |

### Integration test:
```
1. Start dbTui --agent
2. Kirim {"method":"ping"} → expect {"result":{"ok":true}}
3. Kirim connect ke postgres://localhost:5432/test → expect sukses/gagal (tergantung env)
4. Kirim query SELECT 1 → expect {"columns":["?column?"],"rows":[["1"]]}
5. Kirim disconnect → expect {"disconnected":true}
6. Close stdin → agent exit
```

### SQLite test (no server needed, fully isolated):
```
1. Start dbTui --agent
2. Kirim connect sqlite:///tmp/test.db → expect sukses
3. Kirim query CREATE TABLE t(x) → expect error -32003 (strict mode)
4. Kirim query SELECT 1 → expect sukses
5. Kirim query SELECT NULL as n → expect rows: [[null]]
6. Kirim disconnect → expect sukses
7. Cleanup /tmp/test.db
```

### Edge case tests:
```
1. Kirim JSON invalid → expect -32700
2. Kirim method unknown → expect -32601
3. Kirim connect tanpa name → expect -32005
4. Kirim query tanpa connect → expect -32002
5. Kirim query dengan maxRows=1 → expect truncated:true
6. Kirim string yang sangat panjang (1MB+) → expect handle tanpa crash
```

---

## 13. Open Questions (Resolved)

| # | Pertanyaan | Keputusan |
|---|---|---|
| 1 | **Default strict mode?** | ✅ Yes — `strict=true` default |
| 2 | **Max rows default?** | ✅ `1000` |
| 3 | **Connection pool size?** | ✅ 5 max open, 2 idle (sama dengan TUI) |
| 4 | **Query timeout?** | ✅ 30s (sama dengan TUI) |
| 5 | **Concurrent queries?** | ✅ Blocking per request (1 at a time) |
| 6 | **NULL di JSON?** | ✅ `[]*string` → `null` di JSON |
| 7 | **Duplicate connection name?** | ✅ Replace: disconnect existing + connect baru |
| 8 | **Notifications support?** | ❌ Tidak — semua request wajib punya `id` |
| 9 | **TCP listener?** | ❌ Future — v1.0.0 hanya stdin/stdout |
| 10 | **Scanner buffer?** | ✅ 10MB max untuk handle query besar |

---

## 14. Timeline

| Estimasi | Deliverable |
|---|---|
| Hari 1 | `internal/agent/types.go` + `server.go` (scaffold) |
| Hari 2 | `internal/agent/ping.go` + `connect.go` + `internal/db/dsn.go` |
| Hari 3 | `internal/agent/query.go` + strict mode + NULL conversion |
| Hari 4 | `internal/agent/schema.go` + `tables.go` |
| Hari 5 | `internal/agent/explain.go` + `stats.go` |
| Hari 6 | `cmd/dbTui/main.go` dispatch + manual test SQLite |
| Hari 7 | Unit test + integration test + release v1.0.0 |
