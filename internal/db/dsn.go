package db

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/farhank15/dbTui/internal/model"
)

// ParseDSN parses a DATABASE_URL into a model.Connection.
// Supported formats:
//
//	postgres://user:pass@host:port/dbname?sslmode=disable
//	mysql://user:pass@host:port/dbname?charset=utf8mb4
//	sqlite:///path/to/file.db
//	sqlite://./relative.db
func ParseDSN(rawURL string) (model.Connection, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return model.Connection{}, fmt.Errorf("invalid URL: %w", err)
	}

	var conn model.Connection

	switch u.Scheme {
	case "postgres", "postgresql":
		conn.Type = model.TypePostgres
	case "mysql":
		conn.Type = model.TypeMySQL
	case "mariadb":
		conn.Type = model.TypeMariaDB
	case "sqlite", "sqlite3":
		conn.Type = model.TypeSQLite
	default:
		return model.Connection{}, fmt.Errorf("unsupported database type: %s", u.Scheme)
	}

	if conn.Type == model.TypeSQLite {
		path := u.Path
		if path == "" || path == "/" {
			return model.Connection{}, fmt.Errorf("sqlite requires a file path")
		}
		// url.Parse keeps leading / for absolute paths in sqlite:///tmp/foo.db
		// For relative paths like sqlite://./relative.db, path is already ./relative.db
		conn.File = path
		conn.Database = path
		return conn, nil
	}

	// postgres/mysql
	conn.Host = u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		switch conn.Type {
		case model.TypePostgres:
			conn.Port = 5432
		case model.TypeMySQL:
			conn.Port = 3306
		}
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return model.Connection{}, fmt.Errorf("invalid port: %s", portStr)
		}
		conn.Port = port
	}

	if u.User != nil {
		conn.User = u.User.Username()
		conn.Password, _ = u.User.Password()
	}

	conn.Database = strings.TrimPrefix(u.Path, "/")
	conn.SSLMode = u.Query().Get("sslmode")

	return conn, nil
}
