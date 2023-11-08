package asset

import (
	"encoding/hex"

	"github.com/kevinburke/nacl"
	"github.com/kevinburke/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
)

// makeEncryptionKey loads a nacl.Key using a cryptographic key generated from
// the provided passphrase via scrypt.Key.
func makeEncryptionKey(pass []byte) (nacl.Key, error) {
	const N, r, p, keyLength = 1 << 15, 8, 1, 32
	keyBytes, err := scrypt.Key(pass, nil, N, r, p, keyLength)
	if err != nil {
		return nil, err
	}
	return nacl.Load(hex.EncodeToString(keyBytes))
}

// EncryptData encrypts the provided data with the provided passphrase.
func EncryptData(data, passphrase []byte) ([]byte, error) {
	key, err := makeEncryptionKey(passphrase)
	if err != nil {
		return nil, err
	}
	return secretbox.EasySeal(data, key), nil
}

// DecryptData uses the provided passphrase to decrypt the provided data.
func DecryptData(data, passphrase []byte) ([]byte, error) {
	key, err := makeEncryptionKey(passphrase)
	if err != nil {
		return nil, err
	}

	decryptedData, err := secretbox.EasyOpen(data, key)
	if err != nil {
		return nil, ErrInvalidPassphrase
	}

	return decryptedData, nil
}

// ReEncryptData decrypts the provided data using the oldPass and re-encrypts
// the data using newPass.
func ReEncryptData(data, oldPass, newPass []byte) ([]byte, error) {
	data, err := DecryptData(data, oldPass)
	if err != nil {
		return nil, err
	}
	return EncryptData(data, newPass)
}
