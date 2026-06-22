package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// StatusBar displays connection status and messages
type StatusBar struct {
	*tview.TextView
	app        *App
	resetTimer *time.Timer
}

// NewStatusBar creates a new status bar
func NewStatusBar(app *App) *StatusBar {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetTextAlign(tview.AlignLeft)
	tv.SetText(" [::b]dbTui[::-] | No active connection | Ctrl+N: new conn | Ctrl+J: run SQL")
	tv.SetBorder(true)
	tv.SetBorderColor(tcell.ColorGray)

	sb := &StatusBar{
		TextView: tv,
		app:      app,
	}

	return sb
}

// scheduleReset schedules a reset to the default status after 5 seconds
func (sb *StatusBar) scheduleReset() {
	if sb.resetTimer != nil {
		sb.resetTimer.Stop()
	}
	sb.resetTimer = time.AfterFunc(5*time.Second, func() {
		sb.app.app.QueueUpdateDraw(func() {
			sb.UpdateStatus()
		})
	})
}

// UpdateStatus updates the status bar with current connection info
func (sb *StatusBar) UpdateStatus() {
	connections := sb.app.dbManager.GetActiveConnections()
	if len(connections) == 0 {
		sb.SetText(" [::b]dbTui[::-] | No active connection | [green]Ctrl+N[::-] new conn | [green]Ctrl+J[::-] run SQL")
		return
	}

	// Show first active connection
	conn := connections[0]
	info := fmt.Sprintf(" [::b]dbTui[::-] | [green]🔗 %s[::-] (%s) | Database: %s | [green]Ctrl+J[::-] run | [green]Tab[::-] nav | [green]Esc[::-] back",
		conn.Connection.Name, string(conn.Connection.Type), conn.Connection.Database)
	sb.SetText(info)
}

// ShowInfo shows an info message
func (sb *StatusBar) ShowInfo(msg string) {
	sb.SetText(fmt.Sprintf(" [::b]ℹ️ %s[::-]", msg))
	sb.scheduleReset()
}

// ShowError shows an error message
func (sb *StatusBar) ShowError(msg string) {
	sb.SetText(fmt.Sprintf(" [red]❌ %s[::-]", msg))
	sb.scheduleReset()
}

// ShowSuccess shows a success message
func (sb *StatusBar) ShowSuccess(msg string) {
	sb.SetText(fmt.Sprintf(" [green]✅ %s[::-]", msg))
	sb.scheduleReset()
}
