package rsa

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type RsaKeyPair struct {
	PrivateKey string `json:"private_key" eru:"required"`
	PublicKey  string `json:"public_key" eru:"required"`
	Bits       int    `json:"bits" eru:"required"`
}
type JWK struct {
	USE string `json:"use"`
	KTY string `json:"kty"`
	KID string `json:"kid"`
	ALG string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func GenerateKeyPair(ctx context.Context, bits int) (rsaKeyPair RsaKeyPair, err error) {
	logs.WithContext(ctx).Debug("GenerateKeyPair - Start")
	rsaKeyPair.Bits = bits
	// generate key
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("Failed to generate RSA key : ", err.Error()))
		return
	}
	publicKey := &privateKey.PublicKey

	// save private key to a file
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privateKey)

	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	pk := pem.EncodeToMemory(privateKeyBlock)

	rsaKeyPair.PrivateKey = string(pk)

	// save public key to file
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to dump public key: ", err))
		return
	}

	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	pubk := pem.EncodeToMemory(publicKeyBlock)

	rsaKeyPair.PublicKey = string(pubk)

	return
}

func EncryptWithCert(ctx context.Context, plainBytes []byte, publicCert string) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("EncryptWithCert - Start")
	block, _ := pem.Decode([]byte(publicCert))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	var cert *x509.Certificate
	cert, _ = x509.ParseCertificate(block.Bytes)
	rsaPublicKey := cert.PublicKey.(*rsa.PublicKey)
	logs.WithContext(ctx).Info(fmt.Sprintf("EncryptWithCert - Public Key: %s", rsaPublicKey.N.String()))
	return Encrypt(ctx, plainBytes, rsaPublicKey)
}

func EncryptWithKey(ctx context.Context, plainBytes []byte, publicKeyStr string) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("EncryptWithKey - Start")
	rsaPublicKey, err := StringToKey(ctx, publicKeyStr)
	if err != nil {
		return
	}
	return Encrypt(ctx, plainBytes, rsaPublicKey)
}

func StringToKey(ctx context.Context, publicKeyStr string) (rsaPublicKey *rsa.PublicKey, err error) {
	block, _ := pem.Decode([]byte(publicKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	ok := false
	rsaPublicKey, ok = publicKey.(*rsa.PublicKey)
	if !ok {
		err = errors.New("Value returned from ParsePKIXPublicKey was not an RSA public key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func Encrypt(ctx context.Context, plainBytes []byte, rsaPublicKey *rsa.PublicKey) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Encrypt - Start")
	keySize := rsaPublicKey.N.BitLen() / 8
	chunkSize := keySize - 11
	encryptedBytes = make([]byte, 0, len(plainBytes))
	/* for i := 0; i < len(plainBytes); i += 117 {
	if i+117 < len(plainBytes) {
		partial, err1 := rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, plainBytes[i:i+117])
		if err1 != nil {
			logs.WithContext(ctx).Error(err1.Error())
		}
		encryptedBytes = append(encryptedBytes, partial...)
	} else {
		partial, err1 := rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, plainBytes[i:])
		if err1 != nil {
			logs.WithContext(ctx).Error(err1.Error())
		}
		encryptedBytes = append(encryptedBytes, partial...)
	}
		} */
	for i := 0; i < len(plainBytes); i += chunkSize {
		end := i + chunkSize
		if end > len(plainBytes) {
			end = len(plainBytes)
		}
		partial, err1 := rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, plainBytes[i:end])
		if err1 != nil {
			logs.WithContext(ctx).Error(err1.Error())
			return nil, err1
		}
		encryptedBytes = append(encryptedBytes, partial...)
	}
	return
}

func Decrypt(ctx context.Context, encryptedBytes []byte, privateKeyStr string) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Decrypt - Start")
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	decryptedBytes, err = privateKey.Decrypt(nil, encryptedBytes, &rsa.OAEPOptions{Hash: crypto.SHA256})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func DecryptWithKey(ctx context.Context, encryptedBytes []byte, privateKeyStr string) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Decrypt - Start")
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	privateKeyRaw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	privateKey, ok := privateKeyRaw.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected RSA private key in PKCS#8, got %T", privateKeyRaw)
	}

	decryptedBytes, err = privateKey.Decrypt(nil, encryptedBytes, &rsa.OAEPOptions{Hash: crypto.SHA256})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func DecryptPKCS1v15(ctx context.Context, encryptedBytes []byte, privateKeyStr string) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("DecryptPKCS1v15 - Start")
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	privateKeyRaw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	privateKey, ok := privateKeyRaw.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected RSA private key in PKCS#8, got %T", privateKeyRaw)
	}

	keySize := privateKey.N.BitLen() / 8
	decryptedBytes = make([]byte, 0, len(encryptedBytes))

	for i := 0; i < len(encryptedBytes); i += keySize {
		end := i + keySize
		if end > len(encryptedBytes) {
			end = len(encryptedBytes)
		}
		chunk := encryptedBytes[i:end]
		partial, err1 := rsa.DecryptPKCS1v15(rand.Reader, privateKey, chunk)
		if err1 != nil {
			logs.WithContext(ctx).Error(err1.Error())
			return nil, err1
		}
		decryptedBytes = append(decryptedBytes, partial...)
	}
	return
}

// Base64URL encodes a byte slice using the base64 URL encoding scheme.
func base64URL(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}

// Convert RSA Public Key to JWK
func RsaPublicKeyToJWK(ctx context.Context, publicKeyStr string, kid string) (jwk JWK, err error) {
	rsaPublicKey, err := StringToKey(ctx, publicKeyStr)
	if err != nil {
		return
	}
	jwk = JWK{
		KTY: "RSA",
		USE: "sig",
		N:   base64URL(rsaPublicKey.N.Bytes()),        // Modulus
		E:   base64URL(bigIntToBytes(rsaPublicKey.E)), // Exponent
		KID: kid,                                      // Key ID
	}
	return
}

// Convert big.Int to bytes
func bigIntToBytes(i int) []byte {
	return []byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
}

func EncryptOAEP(ctx context.Context, plainBytes []byte, publicKeyStr string, label []byte) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("EncryptOAEP - Start")
	block, _ := pem.Decode([]byte(publicKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	var rsaPublicKey *rsa.PublicKey
	if block.Type == "CERTIFICATE" {
		var cert *x509.Certificate
		cert, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		var ok bool
		rsaPublicKey, ok = cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			err = errors.New("certificate does not contain an RSA public key")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
	} else {
		publicKey, err1 := x509.ParsePKIXPublicKey(block.Bytes)
		if err1 != nil {
			rsaPublicKey, err = x509.ParsePKCS1PublicKey(block.Bytes)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return
			}
		} else {
			var ok bool
			rsaPublicKey, ok = publicKey.(*rsa.PublicKey)
			if !ok {
				err = errors.New("Value returned from ParsePKIXPublicKey was not an RSA public key")
				logs.WithContext(ctx).Error(err.Error())
				return
			}
		}
	}
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPublicKey, plainBytes, label)
}

func Sign(ctx context.Context, data []byte, privateKeyStr string, hash crypto.Hash) (signature []byte, err error) {
	logs.WithContext(ctx).Debug("Sign - Start")
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	var privateKey *rsa.PrivateKey
	privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		privateKeyRaw, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			logs.WithContext(ctx).Error(err8.Error())
			return nil, err8
		}
		var ok bool
		privateKey, ok = privateKeyRaw.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected RSA private key, got %T", privateKeyRaw)
		}
	}

	h := hash.New()
	h.Write(data)
	digest := h.Sum(nil)

	return rsa.SignPKCS1v15(rand.Reader, privateKey, hash, digest)
}

func DecryptOAEP(ctx context.Context, encryptedBytes []byte, privateKeyStr string, label []byte) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("DecryptOAEP - Start")
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		err = errors.New("failed to parse PEM block containing the key")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	var privateKey *rsa.PrivateKey
	privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		privateKeyRaw, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			logs.WithContext(ctx).Error(err8.Error())
			return nil, err8
		}
		var ok bool
		privateKey, ok = privateKeyRaw.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected RSA private key, got %T", privateKeyRaw)
		}
	}

	return rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedBytes, label)
}

func Verify(ctx context.Context, data []byte, signature []byte, publicKeyStr string, hash crypto.Hash) (err error) {
	logs.WithContext(ctx).Debug("Verify - Start")
	rsaPublicKey, err := StringToKey(ctx, publicKeyStr)
	if err != nil {
		return
	}
	h := hash.New()
	h.Write(data)
	digest := h.Sum(nil)
	return rsa.VerifyPKCS1v15(rsaPublicKey, hash, digest, signature)
}

func VerifyWithCert(ctx context.Context, data []byte, signature []byte, publicCert string, hash crypto.Hash) (err error) {
	logs.WithContext(ctx).Debug("VerifyWithCert - Start")
	block, _ := pem.Decode([]byte(publicCert))
	if block == nil {
		return errors.New("failed to parse PEM block containing the key")
	}
	var cert *x509.Certificate
	cert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	rsaPublicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("certificate does not contain an RSA public key")
	}
	h := hash.New()
	h.Write(data)
	digest := h.Sum(nil)
	return rsa.VerifyPKCS1v15(rsaPublicKey, hash, digest, signature)
}
