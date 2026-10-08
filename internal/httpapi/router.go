package httpapi

import (
	"net/http"

	"github.com/gorilla/mux"

	beevibe "github.com/mark/beevibe"
)

// Router builds the complete route tree from plans/api.md §3.
func (s *Server) Router() http.Handler {
	r := mux.NewRouter()
	// Traversal attempts must reach the handlers (which answer 404) instead of
	// being rewritten into a 301 by mux's path cleaning.
	r.SkipClean(true)
	r.Use(s.withRecovery, s.withLogging)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(s.withOriginCheck, s.withRateLimit)

	api.HandleFunc("/login", s.handleLogin).Methods(http.MethodPost)
	api.HandleFunc("/logout", s.handleLogout).Methods(http.MethodPost)
	api.HandleFunc("/me", s.handleMe).Methods(http.MethodGet)

	api.Handle("/rooms", s.admin(s.handleListRooms)).Methods(http.MethodGet)
	api.Handle("/rooms", s.admin(s.handleCreateRoom)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}", s.admin(s.handleGetRoom)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}", s.admin(s.handlePatchRoom)).Methods(http.MethodPatch)
	api.Handle("/rooms/{roomId}", s.admin(s.handleDeleteRoom)).Methods(http.MethodDelete)
	api.Handle("/rooms/{roomId}/start", s.admin(s.handleStartRoom)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/close", s.admin(s.handleCloseRoom)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/reopen", s.admin(s.handleReopenRoom)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/archive", s.admin(s.handleArchiveRoom)).Methods(http.MethodPost)

	api.Handle("/rooms/{roomId}/users", s.admin(s.handleListUsers)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}/users", s.admin(s.handleCreateUser)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users.csv", s.admin(s.handleUsersCSV)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}/users/bulk", s.admin(s.handleBulk)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users/{userId}", s.admin(s.handlePatchUser)).Methods(http.MethodPatch)
	api.Handle("/rooms/{roomId}/users/{userId}", s.admin(s.handleDeleteUser)).Methods(http.MethodDelete)
	api.Handle("/rooms/{roomId}/users/{userId}/kick", s.admin(s.handleKickUser)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users/{userId}/cancel", s.admin(s.handleCancelUser)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users/{userId}/reset", s.admin(s.handleResetUser)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users/{userId}/help", s.admin(s.handleSetHelp)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/users/{userId}/messages", s.admin(s.handleUserMessages)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}/users/{userId}/stats", s.admin(s.handleUserStats)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}/users/{userId}/preview", s.admin(s.handlePreview)).Methods(http.MethodGet)
	api.Handle("/rooms/{roomId}/users/{userId}/preview/refresh", s.admin(s.handlePreviewRefresh)).Methods(http.MethodPost)

	api.Handle("/rooms/{roomId}/broadcast", s.admin(s.handleBroadcast)).Methods(http.MethodPost)
	api.Handle("/rooms/{roomId}/template", s.admin(s.handlePutTemplate)).Methods(http.MethodPut)
	api.Handle("/rooms/{roomId}/template", s.admin(s.handleDeleteTemplate)).Methods(http.MethodDelete)

	api.Handle("/stt", s.user(s.handleSTT)).Methods(http.MethodPost)
	api.Handle("/prompt", s.user(s.handlePrompt)).Methods(http.MethodPost)
	api.Handle("/me/reset-context", s.user(s.handleResetContext)).Methods(http.MethodPost)
	api.Handle("/me/messages", s.user(s.handleMyMessages)).Methods(http.MethodGet)
	api.Handle("/me/stats", s.user(s.handleMyStats)).Methods(http.MethodGet)

	r.Handle("/ws/user", s.sessions.RequireUser(s.db)(http.HandlerFunc(s.handleWS))).Methods(http.MethodGet)
	r.Handle("/ws/admin", s.sessions.RequireAdmin(s.db)(http.HandlerFunc(s.handleWS))).Methods(http.MethodGet)

	r.HandleFunc("/healthz", s.handleHealthz).Methods(http.MethodGet)
	r.HandleFunc("/readyz", s.handleReadyz).Methods(http.MethodGet)

	r.HandleFunc("/rooms/{roomId}/{userId}", s.handleStaticRedirect).Methods(http.MethodGet)
	r.HandleFunc("/rooms/{roomId}/{userId}/{path:.*}", s.handleStatic).Methods(http.MethodGet)

	// Unknown /api/* and /ws/* paths must not fall through to the SPA shell.
	api.PathPrefix("/").Methods(http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete).
		HandlerFunc(s.handleNotFound)
	r.PathPrefix("/ws").HandlerFunc(s.handleNotFound)

	// Client routes survive a reload: anything else that is a GET serves the SPA.
	r.PathPrefix("/").Methods(http.MethodGet).Handler(beevibe.SPA())

	return r
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, CodeNotFound, "no such endpoint")
}

func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return s.sessions.RequireAdmin(s.db)(h)
}

func (s *Server) user(h http.HandlerFunc) http.Handler {
	return s.sessions.RequireUser(s.db)(h)
}
