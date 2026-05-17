package golang

import (
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestExtractTypes_StructInterfaceAndAlias(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "userservice", "types.go"), `package userservice

type Base struct {
	ID string
}

type User struct {
	Base   // embedded
	Name   string ` + "`json:\"name\"`" + `
	Email  string
}

type Repo interface {
	Save(u *User) error
	Load(id string) (*User, error)
}

type UserID = string

const DefaultLimit = 50

var GlobalConfig = "x"
`)
	cfg := Config{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()}
	res, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	bySymbol := map[string]node.Type{}
	for _, ty := range res.Types {
		bySymbol[ty.Symbol] = ty
	}

	user, ok := bySymbol["User"]
	if !ok {
		t.Fatalf("User não emitido: %+v", res.Types)
	}
	if user.Kind_ != node.TypeKindStruct {
		t.Errorf("User.Kind_=%q want struct", user.Kind_)
	}
	if len(user.Fields) != 3 {
		t.Errorf("User.Fields=%d want 3", len(user.Fields))
	}
	// Tag json:"name" capturado.
	var nameField *node.FieldSlot
	for i := range user.Fields {
		if user.Fields[i].Name == "Name" {
			nameField = &user.Fields[i]
		}
	}
	if nameField == nil || nameField.Tags["json"] != "name" {
		t.Errorf("Tag json=name não capturada: %+v", nameField)
	}

	// Repo é interface.
	repo, ok := bySymbol["Repo"]
	if !ok || repo.Kind_ != node.TypeKindInterface {
		t.Errorf("Repo inesperado: %+v", repo)
	}
	if len(repo.Methods) != 2 {
		t.Errorf("Repo.Methods=%d want 2", len(repo.Methods))
	}

	// UserID é alias.
	uid, ok := bySymbol["UserID"]
	if !ok || uid.Kind_ != node.TypeKindAlias {
		t.Errorf("UserID inesperado: %+v", uid)
	}

	// EXTENDS User→Base.
	var foundExt bool
	for _, e := range res.Extends {
		if e.From() == user.URN() && e.To() == bySymbol["Base"].URN() {
			foundExt = true
		}
	}
	if !foundExt {
		t.Errorf("EXTENDS User→Base ausente; %+v", res.Extends)
	}

	// Variables: GlobalConfig (var) + DefaultLimit (const).
	byVar := map[string]node.Variable{}
	for _, v := range res.Variables {
		byVar[v.Symbol] = v
	}
	if v, ok := byVar["DefaultLimit"]; !ok || v.Mutability != node.VariableConst {
		t.Errorf("DefaultLimit inesperado: %+v", v)
	}
	if v, ok := byVar["GlobalConfig"]; !ok || v.Mutability != node.VariableMut {
		t.Errorf("GlobalConfig inesperado: %+v", v)
	}
}

func TestExtractTypes_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "userservice", "t.go"), `package userservice
type Order struct{ ID string }
`)
	a, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if len(a.Types) != len(b.Types) {
		t.Fatalf("len Types diverge")
	}
	if a.Types[0].URN() != b.Types[0].URN() {
		t.Errorf("URN diverge")
	}
	if a.Types[0].ContentHash() != b.Types[0].ContentHash() {
		t.Errorf("ContentHash diverge")
	}
}

func TestExtractTypes_EdgesValidate(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "t.go"), `package service
type A struct{}
type B struct{ A }
type C = A
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, e := range res.Extends {
		if err := edge.Validate(e, node.KindType, node.KindType); err != nil {
			t.Errorf("Validate EXTENDS: %v", err)
		}
	}
	for _, e := range res.Aliases {
		if err := edge.Validate(e, node.KindType, node.KindType); err != nil {
			t.Errorf("Validate ALIASES: %v", err)
		}
	}
}
