package core

import "sync"

// Registry holds the live, never-persisted per-user state (schema.md §3).
type Registry struct {
	mu    sync.Mutex
	users map[int64]*liveUser
}

type liveUser struct {
	mic       bool
	help      bool
	resetting bool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{users: make(map[int64]*liveUser)}
}

func (r *Registry) get(userID int64) *liveUser {
	l := r.users[userID]
	if l == nil {
		l = &liveUser{}
		r.users[userID] = l
	}
	return l
}

// SetMic records the user's push-to-talk indicator.
func (r *Registry) SetMic(userID int64, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.get(userID).mic = on
}

// MicOn reports whether the user is holding the mic button.
func (r *Registry) MicOn(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(userID).mic
}

// SetHelp records whether the user is waiting for the admin (badge + drawer).
func (r *Registry) SetHelp(userID int64, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.get(userID).help = on
}

// HelpPending reports whether the user's raise-hand is still unacknowledged.
func (r *Registry) HelpPending(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(userID).help
}

// SetResetting marks a user whose subdirectory is being re-seeded.
func (r *Registry) SetResetting(userID int64, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.get(userID).resetting = on
}

// Resetting reports whether a reset is in progress for the user.
func (r *Registry) Resetting(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(userID).resetting
}

// Forget drops all live state for a user (delete).
func (r *Registry) Forget(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.users, userID)
}
