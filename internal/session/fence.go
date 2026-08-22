package session

// FenceOf returns the persisted writer fence for a session, or "" when the
// session does not exist or has no fence set. It is used by remote-session
// lease enforcement (internal/daemon) to reject stale writers.
func FenceOf(id string) string {
	if !ValidID(id) {
		return ""
	}
	s, err := Load(id)
	if err != nil || s == nil {
		return ""
	}
	return s.Fence
}

// SetFence records the writer fence on the session in memory. The caller
// persists it with Save.
func (s *Session) SetFence(fence string) {
	if s == nil {
		return
	}
	s.Fence = fence
}
