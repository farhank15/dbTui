package agent

import (
	"time"

	"github.com/farhank15/dbTui/internal/version"
)

func (s *Server) handlePing(req Request) Response {
	uptime := time.Since(s.startAt).Round(time.Second).String()
	return s.makeResult(req.ID, PingResult{
		OK:      true,
		Version: version.Version,
		Uptime:  uptime,
	})
}
