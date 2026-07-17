package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type StatusBar struct {
	*tview.TextView
	app        *App
	resetTimer *time.Timer
}

func NewStatusBar(app *App) *StatusBar {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetTextAlign(tview.AlignLeft)
	tv.SetText(" [::b]dbTui[::-]  |  No active connection  |  Ctrl+N: new conn  |  Ctrl+J: run SQL")
	tv.SetBorder(true)
	tv.SetBorderColor(tcell.ColorGray)
	tv.SetBackgroundColor(Styles.Background)

	sb := &StatusBar{
		TextView: tv,
		app:      app,
	}

	return sb
}

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

func (sb *StatusBar) UpdateStatus() {
	connections := sb.app.dbManager.GetActiveConnections()
	if len(connections) == 0 {
		sb.SetText(" [::b]dbTui[::-]  |  No active connection  |  [green]Ctrl+N[::-] new conn  |  [green]Ctrl+J[::-] run SQL")
		return
	}

	conn := connections[0]
	info := fmt.Sprintf(" [::b]dbTui[::-]  |  [green]%s[::-] (%s)  |  DB: %s  |  [green]Ctrl+J[::-] run  |  [green]Tab[::-] nav  |  [green]Esc[::-] back",
		conn.Connection.Name, string(conn.Connection.Type), conn.Connection.Database)
	sb.SetText(info)
}

func (sb *StatusBar) ShowInfo(msg string) {
	sb.SetText(fmt.Sprintf(" [::b]i[::-]  %s", msg))
	sb.scheduleReset()
}

func (sb *StatusBar) ShowError(msg string) {
	sb.SetText(fmt.Sprintf(" [red]!  %s[::-]", msg))
	sb.scheduleReset()
}

func (sb *StatusBar) ShowSuccess(msg string) {
	sb.SetText(fmt.Sprintf(" [green]v  %s[::-]", msg))
	sb.scheduleReset()
}
