package proto

import (
	"os"
	"path/filepath"
	"testing"

	"costEngine/internal/entity/node"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollect_F022Proto cobre o caminho feliz do collector .proto:
// package directive, message simples, nested message, modifiers
// (repeated/optional), map<K,V> e diretivas ignoradas (option/enum/oneof).
func TestCollect_F022Proto(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "payments", "v1", "payment.proto"), `
syntax = "proto3";

package payments.v1;

// Comentário ignorado.
option go_package = "ex/payments/v1";

enum Status {
  UNKNOWN = 0;
  PAID = 1;
}

message Payment {
  string id = 1;
  optional string note = 2;
  repeated string tags = 3;
  map<string, int64> attrs = 4;

  message Line {
    string sku = 1;
    int32 qty = 2;
  }

  oneof source {
    string credit_card = 10;
    string pix_key = 11;
  }
}
`)

	res, err := Collect(root, Config{Repo: "ex", ServiceModulePath: "payments"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// 2 messages: Payment + Payment.Line.
	if len(res.Schemas) != 2 {
		t.Fatalf("Schemas=%d want 2", len(res.Schemas))
	}

	byName := map[string]node.Schema{}
	for _, s := range res.Schemas {
		byName[s.Symbol] = s
	}

	pay, ok := byName["Payment"]
	if !ok {
		t.Fatalf("missing Payment schema; got %v", keys(byName))
	}
	if pay.Format != node.SchemaProto {
		t.Errorf("Format=%q want proto", pay.Format)
	}
	if pay.Namespace != "payments.v1" {
		t.Errorf("Namespace=%q want payments.v1", pay.Namespace)
	}
	// Fields: id, note, tags, attrs, credit_card, pix_key (oneof flatten) = 6.
	if len(pay.Fields) != 6 {
		t.Errorf("Payment.Fields=%d want 6: %+v", len(pay.Fields), pay.Fields)
	}
	// Confere que map<K,V> ficou íntegro como TypeRef.
	var attrs node.FieldSlot
	for _, f := range pay.Fields {
		if f.Name == "attrs" {
			attrs = f
		}
	}
	if attrs.TypeRef != "map<string,int64>" && attrs.TypeRef != "map<string, int64>" {
		t.Errorf("attrs.TypeRef=%q want map<string,int64>", attrs.TypeRef)
	}

	line, ok := byName["Payment.Line"]
	if !ok {
		t.Fatalf("missing nested Payment.Line schema; got %v", keys(byName))
	}
	if len(line.Fields) != 2 {
		t.Errorf("Payment.Line.Fields=%d want 2", len(line.Fields))
	}
}

func keys(m map[string]node.Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
