package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mark/beevibe/internal/core"
	"github.com/mark/beevibe/internal/stt"
)

// maxClipSeconds is the accepted clip length (Q-STT-7).
const maxClipSeconds = 30

// sttSampleRate is the only rate the browser records and whisper accepts.
const sttSampleRate = 16000

// maxSTTBytes bounds a request body: 30 s of 16 kHz mono PCM16 plus slack for
// the WAV header.
const maxSTTBytes = int64(maxClipSeconds*sttSampleRate*2 + 4096)

// handleSTT transcribes one push-to-talk clip. Every failure path persists a
// system chat line (D6) before answering.
func (s *Server) handleSTT(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionOf(w, r)
	if !ok {
		return
	}
	if s.stt == nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "speech-to-text is unavailable")
		return
	}

	mediaType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	if mediaType != "audio/wav" && mediaType != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be audio/wav or application/octet-stream")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSTTBytes))
	if err != nil {
		s.sttFailure(r, sess.RoomID, sess.UserID)
		writeError(w, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "audio clip is larger than 30 seconds")
		return
	}

	var samples []float32
	switch mediaType {
	case "audio/wav":
		samples, _, err = stt.DecodeWAV(body)
		if err != nil {
			s.sttFailure(r, sess.RoomID, sess.UserID)
			writeError(w, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "audio must be 16 kHz mono 16-bit PCM")
			return
		}
	default:
		samples = stt.PCM16LE(body)
	}
	if len(samples) > maxClipSeconds*sttSampleRate {
		s.sttFailure(r, sess.RoomID, sess.UserID)
		writeError(w, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "audio clip is larger than 30 seconds")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	text, duration, err := s.stt.TranscribeSamples(ctx, samples)
	if err != nil {
		s.sttFailure(r, sess.RoomID, sess.UserID)
		if errors.Is(err, stt.ErrTooLong) {
			writeError(w, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "audio clip is larger than 30 seconds")
			return
		}
		writeError(w, http.StatusBadRequest, CodeBadRequest, "transcription failed")
		return
	}
	if strings.TrimSpace(text) == "" {
		s.sttFailure(r, sess.RoomID, sess.UserID)
		writeError(w, http.StatusBadRequest, CodeBadRequest, "empty transcript")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"text":       text,
		"durationMs": duration.Milliseconds(),
	})
}

// sttFailure records the canned failure line in the user's chat (D6).
func (s *Server) sttFailure(r *http.Request, roomID string, userID int64) {
	if _, err := s.core.InsertMessage(r.Context(), roomID, &userID, core.KindSystem, core.MsgSTTFailed); err != nil {
		s.log.Warn("stt: persist failure notice failed")
	}
}
