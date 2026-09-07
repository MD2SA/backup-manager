package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// DeriveConfigKey derives a 256-bit AES key from a passphrase.
func DeriveConfigKey(passphrase string) []byte {
	sum := sha256.Sum256([]byte(passphrase))
	return sum[:]
}

type encryptedConfig struct {
	Enc string `json:"enc_v1"`
}

// EncodeProviderConfig encrypts provider configuration at rest.
// If key is nil or empty, the plaintext is returned unchanged (dev mode).
func EncodeProviderConfig(key []byte, plain []byte) ([]byte, error) {
	if len(key) == 0 {
		return plain, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, plain, nil)
	payload := append(nonce, sealed...)

	encoded, err := json.Marshal(encryptedConfig{Enc: base64.StdEncoding.EncodeToString(payload)})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal encrypted config: %w", err)
	}

	return encoded, nil
}

// DecodeProviderConfig decrypts provider configuration stored by
// EncodeProviderConfig. Legacy plaintext values are returned unchanged.
func DecodeProviderConfig(key []byte, data []byte) ([]byte, error) {
	if len(data) == 0 || len(key) == 0 {
		return data, nil
	}

	var enc encryptedConfig
	if err := json.Unmarshal(data, &enc); err != nil || enc.Enc == "" {
		return data, nil
	}

	payload, err := base64.StdEncoding.DecodeString(enc.Enc)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted provider config: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return nil, errors.New("encrypted provider config is truncated")
	}

	nonce, sealed := payload[:nonceSize], payload[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt provider config (wrong APP_CONFIG_ENCRYPT_KEY?): %w", err)
	}

	return plain, nil
}
