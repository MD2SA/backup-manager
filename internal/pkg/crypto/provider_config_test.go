package crypto

import (
	"testing"
)

func TestDeriveConfigKey(t *testing.T) {
	k1 := DeriveConfigKey("secret")
	k2 := DeriveConfigKey("secret")
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	if string(k1) != string(k2) {
		t.Fatal("same passphrase should produce same key")
	}
	if string(k1) == string(DeriveConfigKey("other")) {
		t.Fatal("different passphrases should produce different keys")
	}
}

func TestEncodeDecodeProviderConfig_Roundtrip(t *testing.T) {
	key := DeriveConfigKey("test-passphrase")
	plain := []byte(`{"region":"us-east-1","bucket":"b","access_key":"ak","secret_key":"sk"}`)

	encoded, err := EncodeProviderConfig(key, plain)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded) == string(plain) {
		t.Fatal("encoded should differ from plain")
	}

	decoded, err := DecodeProviderConfig(key, encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %s", decoded)
	}
}

func TestEncodeProviderConfig_PlaintextMode(t *testing.T) {
	plain := []byte(`{"key":"value"}`)

	encoded, err := EncodeProviderConfig(nil, plain)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded) != string(plain) {
		t.Fatal("nil key should return plain unchanged")
	}
}

func TestDecodeProviderConfig_LegacyPlaintext(t *testing.T) {
	key := DeriveConfigKey("test")
	plain := []byte(`{"old":"config"}`)

	decoded, err := DecodeProviderConfig(key, plain)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(plain) {
		t.Fatal("legacy plaintext should pass through unchanged")
	}
}

func TestDecodeProviderConfig_WrongKey(t *testing.T) {
	key1 := DeriveConfigKey("key1")
	key2 := DeriveConfigKey("key2")
	plain := []byte(`{"data":"value"}`)

	encoded, err := EncodeProviderConfig(key1, plain)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	_, err = DecodeProviderConfig(key2, encoded)
	if err == nil {
		t.Fatal("expected error decoding with wrong key")
	}
}

func TestDecodeProviderConfig_EmptyData(t *testing.T) {
	key := DeriveConfigKey("key")
	decoded, err := DecodeProviderConfig(key, nil)
	if err != nil {
		t.Fatalf("decode nil: %v", err)
	}
	if decoded != nil {
		t.Fatal("nil input should return nil")
	}

	decoded, err = DecodeProviderConfig(key, []byte{})
	if err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(decoded) != 0 {
		t.Fatal("empty input should return empty")
	}
}
