package httpapi

import (
	"net/http"

	"github.com/gorilla/mux"
)

// handleUserMessages is the admin drawer's chat reader (D3).
func (s *Server) handleUserMessages(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if _, err := s.core.GetUserInRoom(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	s.writeChat(w, r, roomID, userID)
}

func (s *Server) handleUserStats(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	stats, err := s.core.StatsFor(r.Context(), roomID, userID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomId"]
	req, ok := decodeJSON[struct {
		Text string `json:"text"`
	}](w, r)
	if !ok {
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "text is required")
		return
	}
	if _, err := s.core.GetRoom(r.Context(), roomID); err != nil {
		s.fail(w, err)
		return
	}
	msg, err := s.core.Broadcast(r.Context(), roomID, req.Text)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg})
}

// handleMyMessages is the user's own chat reader; same paging as the admin one.
func (s *Server) handleMyMessages(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionOf(w, r)
	if !ok {
		return
	}
	s.writeChat(w, r, sess.RoomID, sess.UserID)
}

func (s *Server) handleMyStats(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionOf(w, r)
	if !ok {
		return
	}
	stats, err := s.core.StatsFor(r.Context(), sess.RoomID, sess.UserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

func (s *Server) writeChat(w http.ResponseWriter, r *http.Request, roomID string, userID int64) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	before := atoi64Default(r.URL.Query().Get("before"), 0)
	messages, err := s.core.ListUserChat(r.Context(), roomID, userID, limit, before)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}
