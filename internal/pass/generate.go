package pass

import (
	"crypto/rand"
	"errors"
	"math/big"
)

const (
	alnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// punct is POSIX [:punct:], the same class `pass generate` draws from when
	// symbols are enabled.
	punct = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
)

// MaxPasswordLength bounds generation. It is far above any real requirement and
// exists only so a mistyped length can't ask for a megabyte of entropy.
const MaxPasswordLength = 256

// NewPassword returns a random password of the given length.
//
// passer generates in-process rather than shelling out to `pass generate`
// because the add dialog needs the value in hand — to show it, and to write it
// in the same `pass insert` as the entry's other fields. The character set
// matches pass's ([:alnum:], plus [:punct:] unless symbols are off), and
// crypto/rand is the same kernel CSPRNG that pass reads from /dev/urandom.
func NewPassword(length int, symbols bool) (string, error) {
	if length <= 0 {
		return "", errors.New("password length must be positive")
	}
	if length > MaxPasswordLength {
		return "", errors.New("password length is too large")
	}

	alphabet := alnum
	if symbols {
		alphabet += punct
	}

	// Draw each character with crypto/rand.Int, which rejection-samples
	// internally — taking a raw byte modulo the alphabet size would skew the
	// distribution toward its first characters.
	max := big.NewInt(int64(len(alphabet)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}
