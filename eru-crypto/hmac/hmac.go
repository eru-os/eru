package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
)

func Hmac(data []byte, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return h.Sum(nil)
}
