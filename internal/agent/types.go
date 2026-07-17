package agent

// JSON-RPC 2.0 types

type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *ErrorObj   `json:"error,omitempty"`
}

type ErrorObj struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Method names
const (
	MethodPing       = "ping"
	MethodConnect    = "connect"
	MethodDisconnect = "disconnect"
	MethodList       = "list"
	MethodQuery      = "query"
	MethodSchema     = "schema"
	MethodTables     = "tables"
	MethodExplain    = "explain"
	MethodStats      = "stats"
)

// Error codes
const (
	ErrConnectionFailed  = -32000
	ErrUnsupportedType   = -32001
	ErrNotConnected      = -32002
	ErrWriteQueryBlocked = -32003
	ErrQueryTimeout      = -32004
	ErrInvalidParams     = -32005
	ErrInternal          = -32006
)

// Method params

type ConnectParams struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type DisconnectParams struct {
	Name string `json:"name"`
}

type QueryParams struct {
	Name    string `json:"name"`
	SQL     string `json:"sql"`
	MaxRows int    `json:"maxRows,omitempty"`
	Strict  *bool  `json:"strict,omitempty"`
}

type SchemaParams struct {
	Name     string `json:"name"`
	Database string `json:"database,omitempty"`
}

type TablesParams struct {
	Name     string `json:"name"`
	Database string `json:"database"`
}

type ExplainParams struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

type StatsParams struct {
	Name string `json:"name"`
}

// Response results

type PingResult struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

type ConnectResult struct {
	Connected     bool   `json:"connected"`
	Type          string `json:"type,omitempty"`
	Version       string `json:"version,omitempty"`
	Database      string `json:"database,omitempty"`
	DefaultSchema string `json:"defaultSchema,omitempty"`
}

type DisconnectResult struct {
	Disconnected bool `json:"disconnected"`
}

type ListResult struct {
	Connections []ListConnEntry `json:"connections"`
}

type ListConnEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	File     string `json:"file,omitempty"`
	Connected bool  `json:"connected"`
}

type QueryResult struct {
	Columns  []string   `json:"columns"`
	Rows     [][]string `json:"rows"`
	Duration string     `json:"duration"`
	RowCount int        `json:"rowCount"`
	IsSelect bool       `json:"isSelect"`
	Truncated bool      `json:"truncated,omitempty"`
}

type SchemaResult struct {
	Databases []SchemaDBEntry `json:"databases"`
}

type SchemaDBEntry struct {
	Name   string          `json:"name"`
	Tables []SchemaTable   `json:"tables,omitempty"`
}

type SchemaTable struct {
	Name        string              `json:"name"`
	Columns     []SchemaColumn      `json:"columns"`
	Indexes     []SchemaIndex       `json:"indexes"`
	ForeignKeys []SchemaFK          `json:"foreignKeys"`
	RowCount    int64               `json:"rowCount"`
}

type SchemaColumn struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Nullable string `json:"nullable"`
	Key     string `json:"key"`
	Default string `json:"default,omitempty"`
	Extra   string `json:"extra,omitempty"`
}

type SchemaIndex struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

type SchemaFK struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"refTable"`
	RefColumn string `json:"refColumn"`
	OnDelete  string `json:"onDelete,omitempty"`
	OnUpdate  string `json:"onUpdate,omitempty"`
}

type TablesResult struct {
	Tables []TableEntry `json:"tables"`
}

type TableEntry struct {
	Name    string `json:"name"`
	RowCount int64  `json:"rowCount"`
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
	Uptime            string  `json:"uptime"`
}
