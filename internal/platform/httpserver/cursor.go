package httpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Cursor é o payload opaco serializado em base64url e exposto como
// `next_cursor` na resposta. Wrapper sobre offset agora; troca por
// keyset (last seen URN) acontece sem mudança de contrato porque o
// shape é interno.
//
// Fingerprint protege contra uso de cursor de query A em query B —
// quando o consumer paginou e mudou a query, o cursor antigo torna-se
// inválido em vez de pular silenciosamente para outra ordem.
type Cursor struct {
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

// EncodeCursor serializa o cursor como base64url(JSON). Offset=0
// retorna "" — convenção "fim da lista".
func EncodeCursor(c Cursor) string {
	if c.Offset == 0 {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor desserializa um cursor recebido pelo client.
//
// Cursor vazio resolve para Cursor{Fingerprint: wantFingerprint} sem
// erro (consumer chamando primeira página).
//
// Fingerprint inválido devolve erro — controller deve traduzir para
// 400 via WriteError (Errorf "bad_cursor").
func DecodeCursor(raw, wantFingerprint string) (Cursor, error) {
	if raw == "" {
		return Cursor{Fingerprint: wantFingerprint}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor encoding")
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor payload")
	}
	if c.Fingerprint != wantFingerprint {
		return Cursor{}, fmt.Errorf("cursor does not match current query")
	}
	if c.Offset < 0 {
		return Cursor{}, fmt.Errorf("invalid cursor offset")
	}
	return c, nil
}

// FingerprintSearch produz um hash curto, estável e determinístico para
// (q, kind). Suficiente para detectar mudança de query entre páginas
// sem expor parâmetros.
func FingerprintSearch(q, kind string) string {
	h := sha256.Sum256([]byte(q + "\x00" + kind))
	return hex.EncodeToString(h[:8])
}
