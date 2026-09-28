package aes

import (
	"context"
	caes "crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const nonceSize = 16

type AesKey struct {
	KeyHex    string `json:"key_string" eru:"required"`
	Key       []byte `json:"key" eru:"required"`
	VectorHex string `json:"iv_string" eru:"required"`
	Vector    []byte `json:"iv" eru:"required"`
	Bits      int    `json:"bits" eru:"required"`
}

type ecb struct {
	block     cipher.Block
	blockSize int
}

func newECB(block cipher.Block) *ecb {
	return &ecb{
		block:     block,
		blockSize: block.BlockSize(),
	}
}

type ecbEncrypter ecb

func NewECBEncrypter(block cipher.Block) cipher.BlockMode {
	return (*ecbEncrypter)(newECB(block))
}

func (e *ecbEncrypter) BlockSize() int { return e.blockSize }

func (e *ecbEncrypter) CryptBlocks(dst, src []byte) {
	if len(src)%e.blockSize != 0 {
		logs.WithContext(context.Background()).Error("crypto/cipher: input blocks are not full")
	}
	if len(dst) < len(src) {
		logs.WithContext(context.Background()).Error("crypto/cipher: output is smaller than input")
	}
	for len(src) > 0 {
		e.block.Encrypt(dst, src[:e.blockSize])
		src = src[e.blockSize:]
		dst = dst[e.blockSize:]
	}
}

type ecbDecrypter ecb

func NewECBDecrypter(block cipher.Block) cipher.BlockMode {
	return (*ecbDecrypter)(newECB(block))
}

func (d *ecbDecrypter) BlockSize() int { return d.blockSize }

func (d *ecbDecrypter) CryptBlocks(dst, src []byte) {
	if len(src)%d.blockSize != 0 {
		logs.WithContext(context.Background()).Error("crypto/cipher: input blocks are not full")
	}
	if len(dst) < len(src) {
		logs.WithContext(context.Background()).Error("crypto/cipher: output is smaller than input")
	}
	for len(src) > 0 {
		d.block.Decrypt(dst, src[:d.blockSize])
		src = src[d.blockSize:]
		dst = dst[d.blockSize:]
	}
}

func GenerateKey(ctx context.Context, bits int) (aesKey AesKey, err error) {
	logs.WithContext(ctx).Debug("GenerateKey - Start")

	aesKey.Bits = bits
	bytes := make([]byte, bits) //generate a random xx byte key for AES-256
	if _, err := rand.Read(bytes); err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	iv := make([]byte, 16) //generate a random 16 byte vector
	if _, err := rand.Read(iv); err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	aesKey.KeyHex = hex.EncodeToString(bytes)
	aesKey.Key = bytes
	aesKey.VectorHex = hex.EncodeToString(iv)
	aesKey.Vector = iv
	return
}

func EncryptECB(ctx context.Context, plainBytes []byte, aesKey []byte) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("EncryptECB - Start")
	block, err := caes.NewCipher(aesKey)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	plaintext := Pad(plainBytes, block.BlockSize())

	ecbObj := NewECBEncrypter(block)
	encryptedBytes = make([]byte, len(plaintext))
	ecbObj.CryptBlocks(encryptedBytes, plaintext)

	return encryptedBytes, err
}

func DecryptECB(ctx context.Context, encryptedBytes []byte, aesKey []byte) (plainBytes []byte, err error) {
	logs.WithContext(ctx).Debug("DecryptECB - Start")
	block, err := caes.NewCipher(aesKey)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	ecbdObj := NewECBDecrypter(block)
	plainBytesBeforeUnpad := make([]byte, len(encryptedBytes))
	ecbdObj.CryptBlocks(plainBytesBeforeUnpad, encryptedBytes)
	plainBytes, err = Unpad(plainBytesBeforeUnpad)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return plainBytes, err
}

func EncryptCBC(ctx context.Context, plainText []byte, bKey []byte, bIV []byte) (encryptedString []byte, err error) {
	logs.WithContext(ctx).Debug("EncryptCBC - Start")
	block, err := caes.NewCipher(bKey)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	bCipherText := make([]byte, len(plainText))
	mode := cipher.NewCBCEncrypter(block, bIV)
	mode.CryptBlocks(bCipherText, (plainText))
	return bCipherText, nil
}
func DecryptCBC(ctx context.Context, cipherText []byte, bKey []byte, bIV []byte) (decryptedString []byte, err error) {
	logs.WithContext(ctx).Debug("DecryptCBC - Start")
	block, err := caes.NewCipher(bKey)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	bPlaintext := make([]byte, len(cipherText))
	mode := cipher.NewCBCDecrypter(block, bIV)
	mode.CryptBlocks(bPlaintext, cipherText)
	return bPlaintext, nil
}

func Encrypt(ctx context.Context, plainBytes []byte, key []byte) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Encrypt - Start")
	block, err := caes.NewCipher(key)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	nonce := make([]byte, aesGCM.NonceSize())
	encryptedBytes = aesGCM.Seal(nonce, nonce, plainBytes, nil)
	return
}

func Decrypt(ctx context.Context, encryptedBytes []byte, key []byte) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Decrypt - Start")

	//Create a new Cipher Block from the key
	block, err := caes.NewCipher(key)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	//Create a new GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	//Get the nonce size
	nonceSize := aesGCM.NonceSize()
	if len(encryptedBytes) < nonceSize {
		err = errors.New("length of encryptedBytes is less then nonce size")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	//Extract the nonce from the encrypted data
	nonce, ciphertext := encryptedBytes[:nonceSize], encryptedBytes[nonceSize:]

	//Decrypt the data
	decryptedBytes, err = aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}
func EncryptGCM(ctx context.Context, plainBytes []byte, key []byte, iv []byte) (encryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("Encrypt - Start")

	flippedIV := make([]byte, len(iv))
	for i, b := range iv {
		flippedIV[i] = ^b
	}

	block, err := caes.NewCipher(key)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	aesGCM, err := cipher.NewGCMWithNonceSize(block, nonceSize)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	encryptedBytes = aesGCM.Seal(nil, flippedIV, plainBytes, nil)
	return
}

func DecryptGCM(ctx context.Context, encryptedBytes []byte, key []byte, iv []byte) (decryptedBytes []byte, err error) {
	logs.WithContext(ctx).Debug("DecryptGCM - Start")

	//Create a new Cipher Block from the key
	block, err := caes.NewCipher(key)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	//Create a new GCM
	aesGCM, err := cipher.NewGCMWithNonceSize(block, nonceSize)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	//Decrypt the data
	decryptedBytes, err = aesGCM.Open(nil, iv, encryptedBytes, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}
