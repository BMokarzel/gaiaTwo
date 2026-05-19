package typescript

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed sidecar/dist/sidecar.bundle.js
var sidecarBundle []byte

// ErrSidecarStub é retornado quando o bundle embedded é só o stub
// (build do sidecar não rodou). Faz o coletor falhar cedo em vez de
// invocar Node com um script que não funciona.
var ErrSidecarStub = errors.New(
	"typescript collector: sidecar bundle is the stub; run `npm install && npm run build` in internal/modules/code/collector/typescript/sidecar to produce dist/sidecar.bundle.js",
)

const stubMarker = "// COSTENGINE_SIDECAR_STUB_v1"

// SidecarBundleHash é o sha256 do bundle embedded, usado como
// chave de cache no tmpdir (re-uso entre invocações).
func SidecarBundleHash() string {
	sum := sha256.Sum256(sidecarBundle)
	return hex.EncodeToString(sum[:])[:16]
}

// materializeSidecar extrai o bundle para
// `<tmp>/costengine-sidecar-<hash>/sidecar.js`. Faz isso uma vez por
// hash — invocações subsequentes pulam a escrita se o arquivo já bate.
func materializeSidecar() (string, error) {
	if isStub() {
		return "", ErrSidecarStub
	}
	hash := SidecarBundleHash()
	dir := filepath.Join(os.TempDir(), "costengine-sidecar-"+hash)
	path := filepath.Join(dir, "sidecar.js")

	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(sidecarBundle)) {
		return path, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, sidecarBundle, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func isStub() bool {
	if len(sidecarBundle) > 4096 {
		return false
	}
	return containsBytes(sidecarBundle, []byte(stubMarker))
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
