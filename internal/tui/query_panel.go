package tui

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// QueryPanel provides a text area for writing SQL queries
type QueryPanel struct {
	*tview.TextArea
	app *App
}

// NewQueryPanel creates a new query panel
func NewQueryPanel(app *App) *QueryPanel {
	textArea := tview.NewTextArea()
	textArea.SetPlaceholder("Write your SQL query here... (Ctrl+Enter to execute | Ctrl+F to Format | Ctrl+P/N for History)")
	textArea.SetBorder(true)
	textArea.SetTitle(" SQL Editor ")
	textArea.SetBorderColor(Styles.Border)

	// Visual focus indicator: change border color when focused
	textArea.SetFocusFunc(func() {
		textArea.SetBorderColor(Styles.BorderFocus)
	})
	textArea.SetBlurFunc(func() {
		textArea.SetBorderColor(Styles.Border)
	})

	// Bind textarea clipboard to system clipboard
	textArea.SetClipboard(func(text string) {
		_ = writeToClipboard(text)
	}, func() string {
		return readFromClipboard()
	})

	textArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Ctrl+Enter to execute query
		isCtrlEnter := event.Key() == tcell.KeyCtrlJ ||
			(event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModCtrl)
		if isCtrlEnter {
			app.ExecuteQuery()
			return nil
		}
		// Tab to navigate to results
		if event.Key() == tcell.KeyTab && event.Modifiers() == tcell.ModNone {
			app.FocusResultTable()
			return nil
		}
		// Ctrl+P / Ctrl+N to cycle query history
		if event.Key() == tcell.KeyCtrlP {
			app.HistoryPrev()
			return nil
		}
		if event.Key() == tcell.KeyCtrlN {
			app.HistoryNext()
			return nil
		}
		// Ctrl+F to format SQL query
		if event.Key() == tcell.KeyCtrlF {
			currentText := textArea.GetText()
			formatted := FormatSQL(currentText)
			textArea.SetText(formatted, true)
			return nil
		}
		return event
	})

	return &QueryPanel{
		TextArea: textArea,
		app:      app,
	}
}

// GetQueryText returns the current query text
func (qp *QueryPanel) GetQueryText() string {
	return qp.TextArea.GetText()
}

// SetQueryText sets the query text
func (qp *QueryPanel) SetQueryText(text string) {
	qp.TextArea.SetText(text, true)
}

// Clear clears the query panel
func (qp *QueryPanel) Clear() {
	qp.TextArea.SetText("", true)
}

// readFromClipboard reads text from system clipboard using native commands
func readFromClipboard() string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return ""
		}
	default:
		return ""
	}
	out, _ := cmd.Output()
	return string(out)
}

// FormatSQL formats and capitalizes SQL keywords
func FormatSQL(query string) string {
	// Keywords that should start on a new line
	newLineKeywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "JOIN": true,
		"LEFT": true, "RIGHT": true, "INNER": true, "OUTER": true,
		"GROUP": true, "ORDER": true, "HAVING": true, "LIMIT": true,
		"INSERT": true, "VALUES": true, "UPDATE": true, "SET": true,
		"DELETE": true, "UNION": true,
	}

	// All keywords to uppercase
	keywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "JOIN": true,
		"LEFT": true, "RIGHT": true, "INNER": true, "OUTER": true,
		"GROUP": true, "ORDER": true, "HAVING": true, "LIMIT": true,
		"INSERT": true, "VALUES": true, "UPDATE": true, "SET": true,
		"DELETE": true, "UNION": true, "AND": true, "OR": true,
		"ON": true, "AS": true, "IN": true, "BY": true, "INTO": true,
	}

	// Clean up whitespaces
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}

	var formatted []string
	for i, word := range words {
		cleanWord := strings.Trim(word, ",;()")
		cleanWordUpper := strings.ToUpper(cleanWord)

		// Determine if this is a keyword we should uppercase
		if keywords[cleanWordUpper] {
			word = strings.Replace(word, cleanWord, cleanWordUpper, 1)
		}

		// Determine if we should prepend a newline
		if i > 0 && newLineKeywords[cleanWordUpper] {
			prevWord := strings.ToUpper(strings.Trim(words[i-1], ",;()"))
			isCompound := (prevWord == "GROUP" && cleanWordUpper == "BY") ||
				(prevWord == "ORDER" && cleanWordUpper == "BY") ||
				(prevWord == "LEFT" && cleanWordUpper == "JOIN") ||
				(prevWord == "RIGHT" && cleanWordUpper == "JOIN") ||
				(prevWord == "INNER" && cleanWordUpper == "JOIN") ||
				(prevWord == "OUTER" && cleanWordUpper == "JOIN")

			if !isCompound {
				formatted = append(formatted, "\n")
			}
		} else if i > 0 && (cleanWordUpper == "AND" || cleanWordUpper == "OR") {
			formatted = append(formatted, "\n  ")
		}

		formatted = append(formatted, word)
	}

	return strings.Join(formatted, " ")
}
