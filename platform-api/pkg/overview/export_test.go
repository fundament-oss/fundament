package overview

import "time"

// SetClock lets tests move time past the TTL instead of sleeping through it.
// Call it before the server handles its first request.
func (s *Server) SetClock(clock func() time.Time) {
	s.clock = clock
}
