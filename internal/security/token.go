package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	PATPrefix      = "kalke_"
	sessionBytes   = 32
	patSecretBytes = 24
	prefixBytes    = 6
)

func RandomToken() (string, error) {
	b := make([]byte, sessionBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashSecret(pepper []byte, secret string) (string, error) {
	sum := sha256.Sum256(append(pepper, []byte(secret)...))
	h, err := bcrypt.GenerateFromPassword(sum[:], bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func CheckSecret(pepper []byte, secret, hash string) bool {
	sum := sha256.Sum256(append(pepper, []byte(secret)...))
	return bcrypt.CompareHashAndPassword([]byte(hash), sum[:]) == nil
}

// NewPAT returns plaintext token, public prefix, and hash.
func NewPAT(pepper []byte) (plaintext, prefix, hash string, err error) {
	pre := make([]byte, prefixBytes)
	sec := make([]byte, patSecretBytes)
	if _, err = rand.Read(pre); err != nil {
		return "", "", "", err
	}
	if _, err = rand.Read(sec); err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(pre)
	secret := base64.RawURLEncoding.EncodeToString(sec)
	plaintext = PATPrefix + prefix + "_" + secret
	hash, err = HashSecret(pepper, plaintext)
	if err != nil {
		return "", "", "", err
	}
	return plaintext, prefix, hash, nil
}

func IsPAT(token string) bool {
	return strings.HasPrefix(token, PATPrefix)
}

func PATPrefixFromToken(token string) (string, error) {
	if !IsPAT(token) {
		return "", fmt.Errorf("not a pat")
	}
	rest := strings.TrimPrefix(token, PATPrefix)
	i := strings.IndexByte(rest, '_')
	if i <= 0 {
		return "", fmt.Errorf("invalid pat")
	}
	return rest[:i], nil
}
