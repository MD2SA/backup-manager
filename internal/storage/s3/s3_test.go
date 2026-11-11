package s3

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		useSSL   bool
		want     string
	}{
		{"minio:9000", false, "http://minio:9000"},
		{"minio:9000", true, "https://minio:9000"},
		{"http://minio:9000", true, "http://minio:9000"},
		{"https://s3.example.com", false, "https://s3.example.com"},
		{"", true, ""},
	}
	for _, tt := range tests {
		if got := normalizeEndpoint(tt.endpoint, tt.useSSL); got != tt.want {
			t.Errorf("normalizeEndpoint(%q,%v) = %q, want %q", tt.endpoint, tt.useSSL, got, tt.want)
		}
	}
}
