package session

import (
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
)

// DefaultSetup is the singleton session setup, initialized by Init().
var DefaultSetup = &Setup{}

// Setup holds the initialized session infrastructure needed by the app layer.
type Setup struct {
	Store     *Store
	SessionID string
}

// Init creates a session store and generates a fresh session ID.
// Sets DefaultSetup as a side effect.
func Initialize(cwd string) {
	DefaultSetup.SessionID = NewSessionID()
	store, err := NewStore(cwd)
	if err != nil {
		log.Logger().Warn("session store initialization failed, sessions will not be persisted", zap.Error(err))
	}
	DefaultSetup.Store = store
}

// TranscriptPath returns the transcript file path for the current session,
// or empty string if the store is nil.
func (s *Setup) TranscriptPath() string {
	if s.Store != nil {
		return s.Store.SessionPath(s.SessionID)
	}
	return ""
}
