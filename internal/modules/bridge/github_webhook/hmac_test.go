package github_webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature_Happy(t *testing.T) {
	body := []byte(`{"action":"closed"}`)
	secret := "shh"
	if err := VerifySignature(body, sign(body, secret), secret); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestVerifySignature_Failures(t *testing.T) {
	body := []byte(`{}`)
	cases := []struct {
		name        string
		header      string
		secret      string
		mutateBody  bool
	}{
		{"empty header", "", "s", false},
		{"empty secret", sign(body, "s"), "", false},
		{"missing prefix", "abcd", "s", false},
		{"not hex", "sha256=zzz", "s", false},
		{"tampered body", sign(body, "s"), "s", true},
		{"wrong secret", sign(body, "other"), "s", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := body
			if c.mutateBody {
				b = []byte(`{"x":1}`)
			}
			if err := VerifySignature(b, c.header, c.secret); !errors.Is(err, ErrBadSignature) {
				t.Fatalf("expected ErrBadSignature, got %v", err)
			}
		})
	}
}
