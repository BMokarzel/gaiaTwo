package github_webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrBadSignature é retornado por VerifySignature quando o header está
// ausente, malformado ou o digest não bate. O caller deve responder
// 401 Unauthorized sem revelar qual ramo falhou.
var ErrBadSignature = errors.New("github_webhook: bad signature")

// signaturePrefix é o esquema usado pelo GitHub no header
// `X-Hub-Signature-256` (formato `sha256=<hex>`).
const signaturePrefix = "sha256="

// VerifySignature confere o HMAC SHA-256 enviado pelo GitHub.
//
//	header  — valor de X-Hub-Signature-256 ("sha256=<hex>").
//	body    — bytes brutos do payload, antes de qualquer parse JSON.
//	secret  — segredo compartilhado configurado no webhook.
//
// Comparação é constant-time (crypto/subtle).
// Retorna nil em match, ErrBadSignature caso contrário.
func VerifySignature(body []byte, header, secret string) error {
	if secret == "" || header == "" {
		return ErrBadSignature
	}
	if !strings.HasPrefix(header, signaturePrefix) {
		return ErrBadSignature
	}
	got, err := hex.DecodeString(header[len(signaturePrefix):])
	if err != nil {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(got, expected) != 1 {
		return ErrBadSignature
	}
	return nil
}
