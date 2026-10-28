package validator_test

import (
	"os"
	"testing"

	"github.com/ulasdursun/queueforge/internal/validator"
)

func TestValidateTargetURL(t *testing.T) {
	os.Unsetenv("ALLOW_LOCAL_FETCH")

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid public https URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "unsupported scheme ftp",
			url:     "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "unsupported scheme file",
			url:     "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: loopback IPv4",
			url:     "http://127.0.0.1:8080/metrics",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: private RFC1918 10.x.x.x",
			url:     "http://10.0.0.1/admin",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: private RFC1918 192.168.x.x",
			url:     "http://192.168.1.1/router",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: AWS instance metadata",
			url:     "http://169.254.169.254/latest/meta-data/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTargetURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTargetURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
