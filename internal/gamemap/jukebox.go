package gamemap

// TryStartJukebox starts a new jukebox track if the map has a jukebox tile and
// no track is currently active.
func (m *GameMap) TryStartJukebox(trackID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.hasJukebox {
		return false
	}
	if m.jukeboxTicks > 0 {
		return false
	}

	trackTimerTicks := m.cfg.Jukebox.TrackTimer * 8
	if trackTimerTicks <= 0 {
		trackTimerTicks = 1
	}

	m.jukeboxTrackID = trackID
	m.jukeboxTicks = trackTimerTicks
	return true
}

func (m *GameMap) tickJukebox() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.jukeboxTicks <= 0 {
		return
	}

	m.jukeboxTicks--
	if m.jukeboxTicks > 0 {
		return
	}

	m.jukeboxTrackID = 0
	m.jukeboxTicks = 0
}
