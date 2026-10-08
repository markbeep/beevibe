package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/mark/beevibe/internal/core"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.core.ListUsers(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Name       string `json:"name"`
		TokenLimit *int64 `json:"tokenLimit"`
	}](w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "name is required")
		return
	}
	if len(name) > 80 {
		name = name[:80]
	}
	user, err := s.core.CreateUser(r.Context(), mux.Vars(r)["roomId"], name, req.TokenLimit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		TokenLimit *int64 `json:"tokenLimit"`
	}](w, r)
	if !ok {
		return
	}
	user, err := s.core.SetUserTokenLimit(r.Context(), roomID, userID, req.TokenLimit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if err := s.core.DeleteUser(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKickUser(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if err := s.core.KickUser(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelUser(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if err := s.core.CancelUser(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetUser(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if _, err := s.core.GetUserInRoom(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	s.core.ResetAsync(roomID, []int64{userID}, core.ResetScopeBulk)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "resetting"})
}

// handleSetHelp lets the admin clear (or re-raise) a user's raise-hand. The
// user's own Help button toggles the same flag over the WebSocket.
func (s *Server) handleSetHelp(w http.ResponseWriter, r *http.Request) {
	userID, roomID, ok := s.userRoute(w, r)
	if !ok {
		return
	}
	if _, err := s.core.GetUserInRoom(r.Context(), roomID, userID); err != nil {
		s.fail(w, err)
		return
	}
	req, ok := decodeJSON[struct {
		Pending *bool `json:"pending"`
	}](w, r)
	if !ok {
		return
	}
	pending := true
	if req.Pending != nil {
		pending = *req.Pending
	}
	if err := s.core.SetHelp(r.Context(), roomID, userID, pending); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bulkRequest decodes `userIds` as either the literal "all" or an int array.
type bulkRequest struct {
	UserIDs    *json.RawMessage `json:"userIds"`
	Action     string           `json:"action"`
	Text       *string          `json:"text"`
	TokenLimit *int64           `json:"tokenLimit"`
}

func (s *Server) handleBulk(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomId"]
	req, ok := decodeJSON[bulkRequest](w, r)
	if !ok {
		return
	}
	out := core.BulkRequest{Action: req.Action, Text: req.Text, TokenLimit: req.TokenLimit}
	if req.UserIDs == nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "userIds is required")
		return
	}
	raw := strings.TrimSpace(string(*req.UserIDs))
	if raw == `"all"` {
		out.All = true
	} else {
		var ids []int64
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, `userIds must be an array of ints or "all"`)
			return
		}
		out.UserIDs = ids
	}
	affected, err := s.core.Bulk(r.Context(), roomID, out)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"affected": affected})
}

func (s *Server) handleUsersCSV(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomId"]
	body, err := s.core.ExportCSV(r.Context(), roomID)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "beevibe-"+roomID+"-users.csv"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// userRoute parses the {roomId}/{userId} route variables.
func (s *Server) userRoute(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	vars := mux.Vars(r)
	roomID := vars["roomId"]
	userID, err := parseUserID(vars["userId"])
	if err != nil {
		writeError(w, http.StatusNotFound, CodeNotFound, "unknown user")
		return 0, "", false
	}
	return userID, roomID, true
}
