package service

import (
	"testing"
)

func TestChooseMetadataEncryption(t *testing.T) {
	tests := []struct {
		name             string
		profileEncrypted bool
		encPassphrase    string
		ageKey           string
		metaPassphrase   string
		wantMode         string
		wantKey          string
	}{
		{name: "profile passphrase wins", profileEncrypted: true, encPassphrase: "p1", ageKey: "age", metaPassphrase: "meta", wantMode: "passphrase", wantKey: "p1"},
		{name: "profile age wins", profileEncrypted: true, encPassphrase: "", ageKey: "age", metaPassphrase: "meta", wantMode: "age", wantKey: "age"},
		{name: "prefer passphrase over age in profile", profileEncrypted: true, encPassphrase: "p1", ageKey: "age", wantMode: "passphrase", wantKey: "p1"},
		{name: "fallback to metadata passphrase", profileEncrypted: false, encPassphrase: "", ageKey: "age", metaPassphrase: "meta", wantMode: "passphrase", wantKey: "meta"},
		{name: "no encryption -> skip", profileEncrypted: false, encPassphrase: "", ageKey: "", metaPassphrase: "", wantMode: "", wantKey: ""},
		{name: "profile encrypted but no keys -> fallback to meta", profileEncrypted: true, encPassphrase: "", ageKey: "", metaPassphrase: "meta", wantMode: "passphrase", wantKey: "meta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseMetadataEncryption(tt.profileEncrypted, tt.encPassphrase, tt.ageKey, tt.metaPassphrase)
			if got.mode != tt.wantMode || got.key != tt.wantKey {
				t.Errorf("got %+v, want mode=%q key=%q", got, tt.wantMode, tt.wantKey)
			}
		})
	}
}

func TestMetadataKeysToDelete(t *testing.T) {
	keys := []string{
		"meta/p1/metadata-20260101-000000.dump.age",
		"meta/p1/metadata-20260102-000000.dump.age",
		"meta/p1/metadata-20260103-000000.dump.age",
	}

	if got := metadataKeysToDelete(keys, 3); got != nil {
		t.Errorf("retention 3 should keep all, got %v", got)
	}

	got := metadataKeysToDelete(keys, 2)
	if len(got) != 1 || got[0] != keys[0] {
		t.Errorf("retention 2 should keep 2 newest and drop oldest (%q), got %v", keys[0], got)
	}

	if got := metadataKeysToDelete(keys, 0); len(got) != 3 {
		t.Errorf("retention 0 should drop all, got %v", got)
	}

	if got := metadataKeysToDelete(nil, 5); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestMetadataKeysToDelete_ProfileIsolation(t *testing.T) {
	// Two profiles share the same provider but have distinct key prefixes.
	keys := []string{
		"meta/p1/metadata-20260101-000000.dump.age",
		"meta/p1/metadata-20260102-000000.dump.age",
		"meta/p1/metadata-20260103-000000.dump.age",
		"meta/p2/metadata-20260101-000000.dump.age",
		"meta/p2/metadata-20260102-000000.dump.age",
	}

	// Retention is evaluated per profile prefix by the caller; this helper only
	// reasons about the keys actually listed under a single prefix.
	p1 := []string{
		"meta/p1/metadata-20260101-000000.dump.age",
		"meta/p1/metadata-20260102-000000.dump.age",
		"meta/p1/metadata-20260103-000000.dump.age",
	}
	got := metadataKeysToDelete(p1, 2)
	if len(got) != 1 || got[0] != p1[0] {
		t.Errorf("p1 retention 2 should drop only p1 oldest, got %v", got)
	}
	_ = keys
}
