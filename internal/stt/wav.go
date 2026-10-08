// Package stt implements speech-to-text for beevibe on top of the vendored
// whisper.cpp Go binding.
package stt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrUnsupportedAudio is returned when the audio is not 16 kHz mono 16-bit
// PCM, or when the WAV container cannot be parsed.
var ErrUnsupportedAudio = errors.New("stt: audio must be 16 kHz mono 16-bit PCM")

// wavSampleRate is the only accepted sample rate; whisper models are trained
// on 16 kHz input and whisper.cpp does not resample.
const wavSampleRate = 16000

// pcmFormat is the WAVE "fmt " audioFormat value for uncompressed PCM.
const pcmFormat = 1

// wavFormat holds the "fmt " fields that matter for STT.
type wavFormat struct {
	audioFormat   uint16
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
}

// DecodeWAV parses a RIFF/WAVE byte slice and returns normalised mono samples
// plus the sample rate. Only 16 kHz mono 16-bit PCM is accepted; anything else
// (including a malformed container) yields an error wrapping ErrUnsupportedAudio.
//
// Unknown chunks are skipped, odd-sized chunks are treated as padded, and
// trailing bytes after the last complete chunk are ignored.
func DecodeWAV(b []byte) (samples []float32, sampleRate int, err error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%w: not a RIFF/WAVE file", ErrUnsupportedAudio)
	}

	var (
		format *wavFormat
		data   []byte
	)
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		off += 8
		if size > len(b)-off {
			return nil, 0, fmt.Errorf("%w: truncated %q chunk", ErrUnsupportedAudio, id)
		}
		body := b[off : off+size]

		switch id {
		case "fmt ":
			if len(body) < 16 {
				return nil, 0, fmt.Errorf("%w: fmt chunk is %d bytes", ErrUnsupportedAudio, len(body))
			}
			format = &wavFormat{
				audioFormat:   binary.LittleEndian.Uint16(body[0:2]),
				numChannels:   binary.LittleEndian.Uint16(body[2:4]),
				sampleRate:    binary.LittleEndian.Uint32(body[4:8]),
				bitsPerSample: binary.LittleEndian.Uint16(body[14:16]),
			}
		case "data":
			data = body
		}

		off += size
		if size%2 == 1 {
			off++ // RIFF chunks are word-aligned.
		}
	}

	switch {
	case format == nil:
		return nil, 0, fmt.Errorf("%w: missing fmt chunk", ErrUnsupportedAudio)
	case format.audioFormat != pcmFormat:
		return nil, 0, fmt.Errorf("%w: audio format %d is not PCM", ErrUnsupportedAudio, format.audioFormat)
	case format.numChannels != 1:
		return nil, 0, fmt.Errorf("%w: %d channels", ErrUnsupportedAudio, format.numChannels)
	case format.bitsPerSample != 16:
		return nil, 0, fmt.Errorf("%w: %d bits per sample", ErrUnsupportedAudio, format.bitsPerSample)
	case format.sampleRate != wavSampleRate:
		return nil, 0, fmt.Errorf("%w: %d Hz", ErrUnsupportedAudio, format.sampleRate)
	case data == nil:
		return nil, 0, fmt.Errorf("%w: missing data chunk", ErrUnsupportedAudio)
	}

	return PCM16LE(data), int(format.sampleRate), nil
}

// PCM16LE converts little-endian interleaved 16-bit PCM bytes into normalised
// float samples in [-1, 1). A trailing odd byte is ignored.
func PCM16LE(b []byte) []float32 {
	n := len(b) / 2
	out := make([]float32, n)
	for i := range n {
		out[i] = float32(int16(binary.LittleEndian.Uint16(b[i*2:]))) / 32768
	}
	return out
}
