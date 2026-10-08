package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/mark/beevibe/internal/auth"
)

// sessionOf returns the session the middleware stored, or answers 401.
func sessionOf(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	sess, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "not signed in")
		return auth.Session{}, false
	}
	return sess, true
}

// uploadLimit bounds one template upload body.
const uploadLimit = 8 << 20

// handlePutTemplate stores the room template and re-seeds every user
// (AREDIT-10: one bad file rejects the whole upload).
func (s *Server) handlePutTemplate(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomId"]
	if _, err := s.core.GetRoom(r.Context(), roomID); err != nil {
		s.fail(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)
	if err := r.ParseMultipartForm(uploadLimit); err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "malformed multipart body")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	uploads := map[string][]byte{}
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			name := header.Filename
			f, err := header.Open()
			if err != nil {
				writeError(w, http.StatusBadRequest, CodeBadRequest, "could not read uploaded file")
				return
			}
			data, err := io.ReadAll(io.LimitReader(f, uploadLimit))
			f.Close()
			if err != nil {
				writeError(w, http.StatusBadRequest, CodeBadRequest, "could not read uploaded file")
				return
			}
			uploads[name] = data
		}
	}
	if len(uploads) == 0 {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "no files were uploaded")
		return
	}
	if err := s.core.Files().PutTemplate(roomID, uploads); err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	userIDs, err := s.core.UserIDs(r.Context(), roomID)
	if err != nil {
		s.fail(w, err)
		return
	}
	// The template only takes effect once every subdirectory is re-seeded; the
	// work outlives the request (the route answers 202), so it must not use the
	// request context.
	go s.core.ReseedForTemplate(context.Background(), roomID, userIDs)
	s.core.PushRoomUpdate(r.Context(), roomID)
	writeJSON(w, http.StatusAccepted, map[string]int{"usersReset": len(userIDs)})
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	roomID := mux.Vars(r)["roomId"]
	if _, err := s.core.GetRoom(r.Context(), roomID); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.core.Files().DeleteTemplate(roomID); err != nil {
		s.fail(w, err)
		return
	}
	s.core.PushRoomUpdate(r.Context(), roomID)
	w.WriteHeader(http.StatusNoContent)
}
