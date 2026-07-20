package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/farhank15/dbTui/internal/agent"
	"github.com/farhank15/dbTui/internal/config"
	"github.com/farhank15/dbTui/internal/tui"
	"github.com/farhank15/dbTui/internal/version"
)

var Version = version.Version

func main() {
	var showVersion, showHelp, showList, agentMode, persistMode bool

	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--version":
			showVersion = true
		case "-h", "--help", "-?":
			showHelp = true
		case "--list":
			showList = true
		case "--agent":
			agentMode = true
		case "--persist":
			persistMode = true
		}
	}

	if showVersion {
		fmt.Printf("dbTui version %s\n", Version)
		return
	}

	if showHelp {
		printUsage()
		return
	}

	if showList {
		printConnections()
		return
	}

	if agentMode {
		server := agent.NewServer()
		if persistMode {
			server.SetPersist(true)
		}
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

func printUsage() {
	fmt.Println(`dbTui - Database TUI & Agent

Usage:
  dbTui                         Launch interactive TUI
  dbTui -v, --version           Show version
  dbTui -h, --help              Show this help
  dbTui --agent                 Run in agent mode (JSON-RPC over stdin/stdout)
  dbTui --agent --persist       Run agent in persist mode (survives stdin EOF)
  dbTui --list                  List saved connections

Agent mode accepts JSON-RPC 2.0 line-delimited messages on stdin.
Methods: ping, connect, disconnect, query, list, schema, tables, explain, stats

Use --persist when running as a detached background agent (e.g. from fennec db start).
Without --persist the agent exits when stdin closes. With --persist it stays alive
until SIGTERM/SIGINT.`)
}

func printConnections() {
	cfg, err := config.NewConfigManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	conns := cfg.GetConnections()
	if len(conns) == 0 {
		fmt.Println("No saved connections.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "  NAME\tTYPE\tHOST:PORT\tDATABASE/FILE")
	fmt.Fprintln(w, "  ----\t----\t---------\t-------------")
	for _, c := range conns {
		hostPort := c.Host
		if c.Port > 0 {
			hostPort = fmt.Sprintf("%s:%d", c.Host, c.Port)
		}
		if hostPort == "" || hostPort == ":0" {
			hostPort = "-"
		}
		db := c.Database
		if db == "" && c.File != "" {
			db = c.File
		}
		if db == "" {
			db = "-"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", c.Name, string(c.Type), hostPort, db)
	}
	w.Flush()
	fmt.Printf("\n  %d connection(s)\n", len(conns))
}
