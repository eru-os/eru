package sha

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
)

func NewSHA1(data []byte) []byte {
	hash := sha1.Sum(data)
	return hash[:]
}

func NewSHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func NewSHA512(data []byte) []byte {
	hash := sha512.Sum512(data)
	return hash[:]
}
