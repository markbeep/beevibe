package stt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go"
)

///////////////////////////////////////////////////////////////////////////////
// WAV FIXTURES

type wavSpec struct {
	sampleRate int
	channels   int
	bits       int
	format     int
	data       []byte
	extra      []byte // raw chunks inserted between fmt and data
	trailer    []byte // raw bytes appended after the data chunk
}

func pcmSpec(data []byte) wavSpec {
	return wavSpec{sampleRate: 16000, channels: 1, bits: 16, format: 1, data: data}
}

func (s wavSpec) bytes() []byte {
	format := s.format
	if format == 0 {
		format = 1
	}
	chunk := func(id string, body []byte) []byte {
		out := make([]byte, 8+len(body)+len(body)%2)
		copy(out, id)
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(body)))
		copy(out[8:], body)
		return out
	}

	fmtBody := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtBody[0:2], uint16(format))
	binary.LittleEndian.PutUint16(fmtBody[2:4], uint16(s.channels))
	binary.LittleEndian.PutUint32(fmtBody[4:8], uint32(s.sampleRate))
	binary.LittleEndian.PutUint32(fmtBody[8:12], uint32(s.sampleRate*s.channels*s.bits/8))
	binary.LittleEndian.PutUint16(fmtBody[12:14], uint16(s.channels*s.bits/8))
	binary.LittleEndian.PutUint16(fmtBody[14:16], uint16(s.bits))

	body := chunk("fmt ", fmtBody)
	body = append(body, s.extra...)
	body = append(body, chunk("data", s.data)...)
	body = append(body, s.trailer...)

	out := make([]byte, 12+len(body))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(body)+4))
	copy(out[8:], "WAVE")
	copy(out[12:], body)
	return out
}

func pcm16(samples ...int16) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}

///////////////////////////////////////////////////////////////////////////////
// DECODEWAV

func TestDecodeWAVAcceptsPCM16Mono16k(t *testing.T) {
	raw := pcmSpec(pcm16(0, -32768, 32767)).bytes()
	samples, rate, err := DecodeWAV(raw)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if rate != 16000 {
		t.Errorf("sample rate = %d, want 16000", rate)
	}
	want := []float32{0, -1, 32767.0 / 32768.0}
	if len(samples) != len(want) {
		t.Fatalf("got %d samples, want %d", len(samples), len(want))
	}
	for i := range want {
		if math.Abs(float64(samples[i]-want[i])) > 1e-6 {
			t.Errorf("sample %d = %v, want %v", i, samples[i], want[i])
		}
	}
}

func TestDecodeWAVSkipsUnknownChunksAndTrailingBytes(t *testing.T) {
	spec := pcmSpec(pcm16(1, 2))
	// An odd-sized chunk exercises the word-alignment padding, and the trailer
	// exercises "more bytes than chunks" tolerance.
	spec.extra = append([]byte("LIST"), 3, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0x00)
	spec.trailer = []byte{0xde, 0xad, 0xbe, 0xef}
	samples, rate, err := DecodeWAV(spec.bytes())
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if rate != 16000 || len(samples) != 2 {
		t.Fatalf("got rate=%d samples=%d, want 16000/2", rate, len(samples))
	}
}

func TestDecodeWAVRejectsUnsupportedFormats(t *testing.T) {
	tests := map[string]wavSpec{
		"stereo":   {sampleRate: 16000, channels: 2, bits: 16, data: pcm16(1, 2, 3, 4)},
		"8 bit":    {sampleRate: 16000, channels: 1, bits: 8, data: []byte{1, 2, 3, 4}},
		"44.1 kHz": {sampleRate: 44100, channels: 1, bits: 16, data: pcm16(1, 2)},
		"float":    {sampleRate: 16000, channels: 1, bits: 32, format: 3, data: make([]byte, 8)},
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := DecodeWAV(spec.bytes()); !errors.Is(err, ErrUnsupportedAudio) {
				t.Fatalf("err = %v, want ErrUnsupportedAudio", err)
			}
		})
	}
}

func TestDecodeWAVRejectsMalformedContainers(t *testing.T) {
	valid := pcmSpec(pcm16(1, 2, 3, 4)).bytes()

	// The fmt chunk ends at offset 12+8+16 = 36, so this keeps fmt only.
	onlyFmt := valid[:36]
	truncated := valid[:len(valid)-4]

	tests := map[string][]byte{
		"empty":            nil,
		"not riff":         []byte("this is not audio at all"),
		"wav magic only":   []byte("RIFF\x04\x00\x00\x00WAVE"),
		"missing fmt":      append([]byte("RIFF\x0c\x00\x00\x00WAVE"), append([]byte("data"), pcm16(1)...)...),
		"missing data":     onlyFmt,
		"truncated data":   truncated,
		"short fmt chunk":  []byte("RIFF\x0c\x00\x00\x00WAVEfmt \x04\x00\x00\x00\x01\x00\x01\x00"),
		"junk after waves": append([]byte("RIFF\x04\x00\x00\x00WAVE"), 1, 2, 3, 4, 5, 6, 7, 8),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := DecodeWAV(raw); !errors.Is(err, ErrUnsupportedAudio) {
				t.Fatalf("err = %v, want ErrUnsupportedAudio", err)
			}
		})
	}
}

///////////////////////////////////////////////////////////////////////////////
// PCM16LE

func TestPCM16LEBounds(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []float32
	}{
		{"empty", nil, []float32{}},
		{"zero", []byte{0x00, 0x00}, []float32{0}},
		{"min", []byte{0x00, 0x80}, []float32{-1}},
		{"max", []byte{0xff, 0x7f}, []float32{32767.0 / 32768.0}},
		{"odd trailing byte", []byte{0x00, 0x40, 0x7f}, []float32{0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PCM16LE(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d samples, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if math.Abs(float64(got[i]-tt.want[i])) > 1e-6 {
					t.Errorf("sample %d = %v, want %v", i, got[i], tt.want[i])
				}
				if got[i] < -1 || got[i] >= 1 {
					t.Errorf("sample %d = %v, outside [-1, 1)", i, got[i])
				}
			}
		})
	}
}

///////////////////////////////////////////////////////////////////////////////
// TRANSCRIBER (no model required)

func TestNewRejectsMissingModel(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "absent.bin")); err == nil {
		t.Fatal("New accepted a missing model file")
	}
	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("New accepted a directory as a model file")
	}
	if _, err := New("   "); err == nil {
		t.Fatal("New accepted an empty model path")
	}
}

func TestTranscribeErrorPathsWithoutModel(t *testing.T) {
	// A zero-value Transcriber has no model; every path below must be decided
	// before the C call is attempted.
	tr := &Transcriber{threads: 1}

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := tr.Transcribe(ctx, pcmSpec(pcm16(1)).bytes()); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})

	t.Run("unsupported audio", func(t *testing.T) {
		spec := pcmSpec(pcm16(1, 2, 3, 4))
		spec.channels = 2
		if _, _, err := tr.Transcribe(context.Background(), spec.bytes()); !errors.Is(err, ErrUnsupportedAudio) {
			t.Fatalf("err = %v, want ErrUnsupportedAudio", err)
		}
	})

	t.Run("too long", func(t *testing.T) {
		raw := pcmSpec(make([]byte, 2*(maxClipSamples+1))).bytes()
		_, _, err := tr.Transcribe(context.Background(), raw)
		if !errors.Is(err, ErrTooLong) {
			t.Fatalf("err = %v, want ErrTooLong", err)
		}
	})

	t.Run("exactly 30 seconds is not too long", func(t *testing.T) {
		_, _, err := tr.Transcribe(context.Background(), pcmSpec(make([]byte, 2*maxClipSamples)).bytes())
		if errors.Is(err, ErrTooLong) {
			t.Fatalf("a 30 second clip was rejected as too long (%v)", err)
		}
		if err != nil && !errors.Is(err, errClosed) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty audio", func(t *testing.T) {
		text, dur, err := tr.Transcribe(context.Background(), pcmSpec(nil).bytes())
		if err != nil || text != "" || dur != 0 {
			t.Fatalf("got (%q, %s, %v), want (\"\", 0, nil)", text, dur, err)
		}
	})

	t.Run("closed", func(t *testing.T) {
		if _, _, err := tr.Transcribe(context.Background(), pcmSpec(pcm16(1, 2)).bytes()); !errors.Is(err, errClosed) {
			t.Fatalf("err = %v, want errClosed", err)
		}
	})
}

func TestTranscribeAfterClose(t *testing.T) {
	model := findModel(t)
	tr, err := New(model)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, _, err := tr.Transcribe(context.Background(), pcmSpec(pcm16(1, 2)).bytes()); !errors.Is(err, errClosed) {
		t.Fatalf("err = %v, want errClosed", err)
	}
}

///////////////////////////////////////////////////////////////////////////////
// REAL TRANSCRIPTION

// jfkWords is the spoken content of samples/jfk.wav from whisper.cpp.
var jfkWords = []string{
	"and", "so", "my", "fellow", "americans", "ask", "not", "what", "your",
	"country", "can", "do", "for", "you", "ask", "what", "you", "can", "do",
	"for", "your", "country",
}

func TestTranscribeJFK(t *testing.T) {
	model := findModel(t)
	wav := findFixture(t)

	raw, err := os.ReadFile(wav)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	tr, err := New(model)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if err := tr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	start := time.Now()
	text, dur, err := tr.Transcribe(context.Background(), raw)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	t.Logf("transcript: %q", text)
	t.Logf("durationMs: %d", dur.Milliseconds())
	t.Logf("wall clock: %s", elapsed.Round(time.Millisecond))

	// jfk.wav carries exactly 176000 samples at 16 kHz.
	if dur.Milliseconds() != 11000 {
		t.Errorf("durationMs = %d, want 11000", dur.Milliseconds())
	}
	if got := normalizeWords(text); !slices.Equal(got, jfkWords) {
		t.Errorf("transcript words = %v, want %v", got, jfkWords)
	}
}

func TestTranscribeSamplesJFK(t *testing.T) {
	model := findModel(t)
	wav := findFixture(t)

	raw, err := os.ReadFile(wav)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	samples, rate, err := DecodeWAV(raw)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if rate != 16000 {
		t.Fatalf("sample rate = %d, want 16000", rate)
	}

	tr, err := New(model)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if err := tr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	start := time.Now()
	text, dur, err := tr.TranscribeSamples(context.Background(), samples)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("TranscribeSamples: %v", err)
	}

	t.Logf("samples: %d", len(samples))
	t.Logf("transcript: %q", text)
	t.Logf("durationMs: %d", dur.Milliseconds())
	t.Logf("wall clock: %s", elapsed.Round(time.Millisecond))

	if dur.Milliseconds() != 11000 {
		t.Errorf("durationMs = %d, want 11000", dur.Milliseconds())
	}
	if got := normalizeWords(text); !slices.Equal(got, jfkWords) {
		t.Errorf("transcript words = %v, want %v", got, jfkWords)
	}
}

func TestTranscribeSamplesErrorPathsWithoutModel(t *testing.T) {
	// A zero-value Transcriber has no model; these paths must be decided
	// before the C call is attempted.
	tr := &Transcriber{threads: 1}

	t.Run("empty slice", func(t *testing.T) {
		text, dur, err := tr.TranscribeSamples(context.Background(), nil)
		if err != nil || text != "" || dur != 0 {
			t.Fatalf("got (%q, %s, %v), want (\"\", 0, nil)", text, dur, err)
		}
	})

	t.Run("too long", func(t *testing.T) {
		// One sample past 30 seconds.
		if _, _, err := tr.TranscribeSamples(context.Background(), make([]float32, maxClipSamples+1)); !errors.Is(err, ErrTooLong) {
			t.Fatalf("err = %v, want ErrTooLong", err)
		}
	})

	t.Run("exactly 30 seconds is not too long", func(t *testing.T) {
		_, _, err := tr.TranscribeSamples(context.Background(), make([]float32, maxClipSamples))
		if errors.Is(err, ErrTooLong) {
			t.Fatalf("a 30 second clip was rejected as too long (%v)", err)
		}
		if err != nil && !errors.Is(err, errClosed) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := tr.TranscribeSamples(ctx, []float32{0, 0.5}); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})

	t.Run("closed", func(t *testing.T) {
		if _, _, err := tr.TranscribeSamples(context.Background(), []float32{0, 0.5}); !errors.Is(err, errClosed) {
			t.Fatalf("err = %v, want errClosed", err)
		}
	})
}

func TestHasSpeechRejectsNonSpeechTranscripts(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"blank audio", "[BLANK_AUDIO]"},
		{"lowercase marker", "[blank_audio]"},
		{"padded marker", "[ Noise ]"},
		{"paren marker", "(silence)"},
		{"sound", "[SOUND]"},
		{"music", "[MUSIC]"},
		{"laughter", "[LAUGHTER]"},
		{"applause", "(applause)"},
		{"inaudible", "[INAUDIBLE]"},
		{"marker with dots", "[BLANK_AUDIO]."},
		{"marker without underscore", "blank audio"},
		{"marker with only spaces", "  "},
		{"newline only", "\n\t"},
		{"empty", ""},
		{"dots", "..."},
		{"ellipsis", "  \u2026  "},
		{"repeated markers", "[BLANK_AUDIO] [SOUND]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if hasSpeech(tt.text) {
				t.Errorf("hasSpeech(%q) = true, want false", tt.text)
			}
			if got := filterTranscript(tt.text); got != "" {
				t.Errorf("filterTranscript(%q) = %q, want \"\"", tt.text, got)
			}
		})
	}
}

func TestHasSpeechKeepsRealSpeech(t *testing.T) {
	tests := []string{
		"hello [BLANK_AUDIO]",
		"make the heading say Hello",
		"noise cancelling headphones",
		"silence is golden",
		"[SOUND] but then real words",
		"And so my fellow Americans, ask not what your country can do for you, ask what you can do for your country.",
		"ok",
	}
	for _, text := range tests {
		if !hasSpeech(text) {
			t.Errorf("hasSpeech(%q) = false, want true", text)
		}
		// Markers inside real speech are not stripped: the text comes back
		// byte for byte.
		if got := filterTranscript(text); got != text {
			t.Errorf("filterTranscript(%q) = %q, want it unchanged", text, got)
		}
	}
}

func TestTranscribeSamplesSilence(t *testing.T) {
	model := findModel(t)

	tr, err := New(model)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if err := tr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// One second of digital silence, first as samples and then as the WAV the
	// HTTP handler would receive.
	samples := make([]float32, 16000)

	// Evidence that the filter, and not whisper itself, produced the empty
	// transcript: bypass TranscribeSamples to see the raw marker text.
	if raw := rawTranscript(t, tr, samples); raw != "" && hasSpeech(raw) {
		t.Errorf("raw whisper output %q is not a non-speech marker; the silence test is vacuous", raw)
	} else {
		t.Logf("raw whisper output: %q", raw)
	}

	text, dur, err := tr.TranscribeSamples(context.Background(), samples)
	if err != nil {
		t.Fatalf("TranscribeSamples: %v", err)
	}
	t.Logf("silence samples transcript: %q", text)
	if text != "" {
		t.Errorf("silence transcribed as %q, want empty", text)
	}
	if dur != time.Second {
		t.Errorf("duration = %s, want 1s", dur)
	}

	wavText, wavDur, err := tr.Transcribe(context.Background(), pcmSpec(make([]byte, 2*16000)).bytes())
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	t.Logf("silence WAV transcript: %q", wavText)
	if wavText != "" {
		t.Errorf("silent WAV transcribed as %q, want empty", wavText)
	}
	if wavDur != time.Second {
		t.Errorf("WAV duration = %s, want 1s", wavDur)
	}
}

// rawTranscript runs whisper directly, bypassing the non-speech filter, so a
// test can show what the model actually produced.
func rawTranscript(t *testing.T, tr *Transcriber, samples []float32) string {
	t.Helper()
	params := tr.model.Whisper_full_default_params(whisper.SAMPLING_GREEDY)
	params.SetThreads(tr.threads)
	params.SetNoContext(true)
	params.SetSingleSegment(false)
	params.SetPrintSpecial(false)
	params.SetPrintProgress(false)
	params.SetPrintRealtime(false)
	params.SetPrintTimestamps(false)
	if err := tr.model.Whisper_full(params, samples, nil, nil, nil); err != nil {
		t.Fatalf("Whisper_full: %v", err)
	}
	segments := tr.model.Whisper_full_n_segments()
	parts := make([]string, 0, segments)
	for i := range segments {
		if segment := strings.TrimSpace(tr.model.Whisper_full_get_segment_text(i)); segment != "" {
			parts = append(parts, segment)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func normalizeWords(s string) []string {
	var (
		words []string
		cur   strings.Builder
	)
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return words
}

// findModel locates the whisper model, skipping the test when it is absent.
func findModel(t *testing.T) string {
	t.Helper()
	candidates := []string{}
	if env := os.Getenv("WHISPER_MODEL"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates,
		filepath.Join("..", "..", "models", "ggml-base.en.bin"),
		filepath.Join("models", "ggml-base.en.bin"),
	)
	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			return abs
		}
	}
	t.Skipf("whisper model not found (looked in %v); run 'just whisper-model'", candidates)
	return ""
}

const jfkFixtureURL = "https://raw.githubusercontent.com/ggml-org/whisper.cpp/master/samples/jfk.wav"

// findFixture locates the jfk.wav sample, skipping the test when it is absent.
// Set STT_TEST_DOWNLOAD=1 to fetch it into a temporary directory instead.
func findFixture(t *testing.T) string {
	t.Helper()
	candidates := []string{}
	if env := os.Getenv("STT_TEST_WAV"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates,
		filepath.Join(os.TempDir(), "jfk.wav"),
		filepath.Join("testdata", "jfk.wav"),
	)
	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			return path
		}
	}
	if os.Getenv("STT_TEST_DOWNLOAD") == "1" {
		path := filepath.Join(t.TempDir(), "jfk.wav")
		if err := download(jfkFixtureURL, path); err != nil {
			t.Fatalf("download fixture: %v", err)
		}
		return path
	}
	t.Skipf("jfk.wav not found (looked in %v); run: curl -fsS -o %s/%s %s",
		candidates, os.TempDir(), "jfk.wav", jfkFixtureURL)
	return ""
}

func download(url, path string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected status " + resp.Status)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
