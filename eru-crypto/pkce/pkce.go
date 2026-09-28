package pkce

import (
	"context"
	"crypto/rand"
	b64 "encoding/base64"
	"github.com/eru-os/eru/eru-crypto/sha"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"strings"
)

func NewPKCE(ctx context.Context) (codeVerifier string, codeChallenge string, err error) {
	bytes := make([]byte, 43) //generate a random 43 byte key for
	_, err = rand.Read(bytes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	baseStr := b64.StdEncoding.EncodeToString(bytes)
	codeVerifier = strings.Replace(strings.Replace(strings.Replace(baseStr, "=", "", -1), "+", "-", -1), "/", "_", -1)
	codeVerifierHash := sha.NewSHA256([]byte(codeVerifier))
	codeVerifierHashBase := b64.StdEncoding.EncodeToString(codeVerifierHash)
	codeChallenge = strings.Replace(strings.Replace(strings.Replace(codeVerifierHashBase, "=", "", -1), "+", "-", -1), "/", "_", -1)
	return
}
