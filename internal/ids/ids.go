// Package ids generates the public identifiers used by the application.
package ids

import (
	"crypto/rand"
	"math/big"
)

// roomAlphabet is the 31-character unambiguous alphabet for room ids
// (no 0/O/1/I/L).
const roomAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// tokenAlphabet is the 62-character base62 alphabet for user tokens.
const tokenAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const (
	roomIDLen = 6
	tokenLen  = 8
	roomBits  = 5 // ceil(log2(31))
	tokenBits = 6 // log2(64) covers 62
)

// RoomID returns a 6-character room id drawn uniformly from roomAlphabet.
func RoomID() string { return randomString(roomAlphabet, roomBits, roomIDLen) }

// UserToken returns an 8-character base62 user token.
func UserToken() string { return randomString(tokenAlphabet, tokenBits, tokenLen) }

// randomString draws len characters from alphabet using rejection sampling so
// the distribution stays uniform (no modulo bias).
func randomString(alphabet string, bits, length int) string {
	mask := big.NewInt(1)
	mask.Lsh(mask, uint(bits))
	limit := big.NewInt(int64(len(alphabet)))
	out := make([]byte, 0, length)
	for len(out) < length {
		n, err := rand.Int(rand.Reader, mask)
		if err != nil {
			// crypto/rand failure is not recoverable in a useful way; a
			// generated id would be unsafe to hand out.
			panic("ids: crypto/rand failed: " + err.Error())
		}
		if n.Cmp(limit) >= 0 {
			continue // reject values outside the alphabet
		}
		out = append(out, alphabet[n.Int64()])
	}
	return string(out)
}
