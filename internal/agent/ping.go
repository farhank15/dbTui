package agent

import "time"

func (s *Server) handlePing(req Request) Response {
	uptime := time.Since(s.startAt).Round(time.Second).String()
	return s.makeResult(req.ID, PingResult{
		OK:      true,
		Version: "1.0.0",
		Uptime:  uptime,
	})
}
