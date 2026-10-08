package httpapi

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/mark/beevibe/internal/db/gen"
)

// --- rooms ------------------------------------------------------------------

func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.core.ListRooms(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Name *string `json:"name"`
	}](w, r)
	if !ok {
		return
	}
	room, err := s.core.CreateRoom(r.Context(), req.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	view, err := s.core.RoomView(r.Context(), room)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room": view})
}

func (s *Server) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.core.GetRoom(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handlePatchRoom(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Name  *string `json:"name"`
		Model *string `json:"model"`
	}](w, r)
	if !ok {
		return
	}
	room, err := s.core.UpdateRoom(r.Context(), mux.Vars(r)["roomId"], req.Name, req.Model)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handleStartRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.core.Start(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handleCloseRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.core.Close(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handleReopenRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.core.Reopen(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handleArchiveRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.core.Archive(r.Context(), mux.Vars(r)["roomId"])
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeRoom(w, r, room, http.StatusOK)
}

func (s *Server) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if err := s.core.Delete(r.Context(), mux.Vars(r)["roomId"]); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeRoom(w http.ResponseWriter, r *http.Request, room dbgen.Room, status int) {
	view, err := s.core.RoomView(r.Context(), room)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, status, map[string]any{"room": view})
}
