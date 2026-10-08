package stt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go"
)

// maxClipSamples is the 30-second cap accepted by the STT endpoint.
const maxClipSamples = 30 * whisper.SampleRate

// maxThreads bounds the worker threads per transcription.
const maxThreads = 8

var (
	// ErrTooLong is returned for clips longer than 30 seconds.
	ErrTooLong = errors.New("stt: audio clip is longer than 30 seconds")

	// errClosed is returned when the transcriber has been closed or was never
	// initialised with a model.
	errClosed = errors.New("stt: transcriber is closed")
)

// Transcriber owns a loaded whisper model. The underlying whisper context is
// stateful and cannot be used concurrently, so transcriptions are serialised.
type Transcriber struct {
	model   *whisper.Context
	threads int

	mu sync.Mutex
}

// New loads the whisper model from modelPath. It fails when the file is
// missing, unreadable or not a usable whisper model.
func New(modelPath string) (*Transcriber, error) {
	if strings.TrimSpace(modelPath) == "" {
		return nil, errors.New("stt: whisper model path is empty")
	}
	fi, err := os.Stat(modelPath)
	if err != nil {
		return nil, fmt.Errorf("stt: whisper model %q: %w", modelPath, err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("stt: whisper model %q is a directory", modelPath)
	}
	if fi.Size() == 0 {
		return nil, fmt.Errorf("stt: whisper model %q is empty", modelPath)
	}

	model := whisper.Whisper_init(modelPath)
	if model == nil {
		return nil, fmt.Errorf("stt: cannot load whisper model %q", modelPath)
	}

	return &Transcriber{model: model, threads: transcriptionThreads()}, nil
}

// transcriptionThreads returns min(runtime.NumCPU(), 8), never below 1.
func transcriptionThreads() int {
	n := runtime.NumCPU()
	if n > maxThreads {
		n = maxThreads
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Close frees the whisper model. It is safe to call more than once.
func (t *Transcriber) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.model == nil {
		return nil
	}
	t.model.Whisper_free()
	t.model = nil
	return nil
}

// Transcribe decodes a 16 kHz mono 16-bit PCM WAV buffer and returns the
// transcript, the clip duration and an error. It is a thin wrapper around
// TranscribeSamples.
func (t *Transcriber) Transcribe(ctx context.Context, wav []byte) (string, time.Duration, error) {
	if t == nil {
		return "", 0, errClosed
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	samples, _, err := DecodeWAV(wav)
	if err != nil {
		return "", 0, err
	}

	return t.TranscribeSamples(ctx, samples)
}

// TranscribeSamples transcribes already-decoded 16 kHz mono float samples. It
// returns the transcript, the clip duration and an error. Clips longer than
// 30 seconds are rejected with ErrTooLong and an empty slice yields an empty
// transcript without touching the model.
//
// Cancelling ctx aborts the call before it starts and is reported once the
// (uninterruptible) C call returns.
func (t *Transcriber) TranscribeSamples(ctx context.Context, samples []float32) (string, time.Duration, error) {
	if t == nil {
		return "", 0, errClosed
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	duration := time.Duration(len(samples)) * time.Second / whisper.SampleRate
	if len(samples) > maxClipSamples {
		return "", 0, ErrTooLong
	}
	if len(samples) == 0 {
		return "", 0, nil
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.model == nil {
		return "", 0, errClosed
	}
	// Acquiring the mutex may block behind another transcription, so check
	// cancellation once more before entering the C call.
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	params := t.model.Whisper_full_default_params(whisper.SAMPLING_GREEDY)
	params.SetThreads(t.threads)
	params.SetNoContext(true)
	params.SetSingleSegment(false)
	params.SetPrintSpecial(false)
	params.SetPrintProgress(false)
	params.SetPrintRealtime(false)
	params.SetPrintTimestamps(false)

	if err := t.model.Whisper_full(params, samples, nil, nil, nil); err != nil {
		return "", 0, fmt.Errorf("stt: whisper: %w", err)
	}
	// The C call cannot be interrupted; honour cancellation once it returns.
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	segments := t.model.Whisper_full_n_segments()
	parts := make([]string, 0, segments)
	for i := range segments {
		if segment := strings.TrimSpace(t.model.Whisper_full_get_segment_text(i)); segment != "" {
			parts = append(parts, segment)
		}
	}

	text := filterTranscript(strings.TrimSpace(strings.Join(parts, " ")))
	return text, duration, nil
}

// filterTranscript returns a transcript unchanged when it carries speech, and
// "" when it carries none. whisper.cpp emits bracketed non-speech markers
// ([BLANK_AUDIO], [SOUND], (silence), ...) for silent or non-speech audio;
// those must not reach the caller, while markers embedded in real speech are
// left alone.
func filterTranscript(text string) string {
	if !hasSpeech(text) {
		return ""
	}
	return text
}

// nonSpeechMarkers are whisper.cpp's non-speech outputs, keyed by their
// normalised form (see speechKey).
var nonSpeechMarkers = map[string]bool{
	"blank_audio": true,
	"blank audio": true,
	"sound":       true,
	"music":       true,
	"noise":       true,
	"silence":     true,
	"laughter":    true,
	"applause":    true,
	"inaudible":   true,
}

// speechKey normalises a transcript for the non-speech test: brackets and
// parentheses are dropped, whitespace runs collapse to single spaces, the text
// is lowercased, and surrounding dots/ellipses are trimmed.
func speechKey(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	pendingSpace := false
	for _, r := range strings.ToLower(text) {
		switch r {
		case '[', ']', '(', ')':
			continue
		}
		if unicode.IsSpace(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), ".\u2026 ")
}

// hasSpeech reports whether a transcript carries actual speech. Empty output,
// dots-only output and output made up entirely of whisper non-speech markers
// are all considered speechless.
func hasSpeech(text string) bool {
	key := speechKey(text)
	if key == "" {
		return false
	}
	if nonSpeechMarkers[key] {
		return false
	}
	for _, token := range strings.Fields(key) {
		if !nonSpeechMarkers[token] {
			return true
		}
	}
	return false
}
