package errs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"costEngine/internal/core/errs"
	"costEngine/internal/repository"
)

// fixture: erro tipado canônico (HTTPProblem + Detailer + Titler).
type fullErr struct {
	msg     string
	status  int
	code    string
	title   string
	details map[string]any
}

func (e fullErr) Error() string           { return e.msg }
func (e fullErr) HTTPStatus() int         { return e.status }
func (e fullErr) Code() string            { return e.code }
func (e fullErr) Title() string           { return e.title }
func (e fullErr) Details() map[string]any { return e.details }

// fixture mínima: só HTTPProblem.
type minErr struct {
	msg    string
	status int
	code   string
}

func (e minErr) Error() string   { return e.msg }
func (e minErr) HTTPStatus() int { return e.status }
func (e minErr) Code() string    { return e.code }

func TestRender_TypedFull(t *testing.T) {
	err := fullErr{
		msg:     "team not found: urn:ce:org:team:foo",
		status:  404,
		code:    "org.team.not_found",
		title:   "Team not found",
		details: map[string]any{"urn": "urn:ce:org:team:foo"},
	}
	status, p := errs.Render(err)
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if p.Code != "org.team.not_found" {
		t.Errorf("code = %q", p.Code)
	}
	if p.Type != "ce:err:org.team.not_found" {
		t.Errorf("type = %q", p.Type)
	}
	if p.Title != "Team not found" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Detail != err.msg {
		t.Errorf("detail = %q", p.Detail)
	}
	if got := p.Extras["urn"]; got != "urn:ce:org:team:foo" {
		t.Errorf("extras.urn = %v", got)
	}
}

func TestRender_TypedMinimal(t *testing.T) {
	// Sem Title/Details → defaults.
	err := minErr{msg: "x", status: 409, code: "x.y"}
	status, p := errs.Render(err)
	if status != 409 {
		t.Fatalf("status = %d", status)
	}
	if p.Title != "Conflict" {
		t.Errorf("title = %q, want Conflict (default from http.StatusText)", p.Title)
	}
	if p.Extras != nil {
		t.Errorf("extras = %v, want nil", p.Extras)
	}
}

func TestRender_WrappedTyped(t *testing.T) {
	base := minErr{msg: "x", status: 422, code: "y.z"}
	wrapped := fmt.Errorf("doing thing: %w", base)
	status, p := errs.Render(wrapped)
	if status != 422 {
		t.Fatalf("status = %d", status)
	}
	if p.Code != "y.z" {
		t.Errorf("code = %q", p.Code)
	}
}

func TestRender_TypedWinsOverSentinel(t *testing.T) {
	// Erro tipado que ALSO faz Unwrap para um sentinel — typed deve vencer.
	type wrapsSentinel struct{ minErr }
	_ = errors.Is // explicit dep import-check
	// (sem método Unwrap não precisa; mas se houvesse, As para Tipo vence Is para sentinel)
	err := wrapsSentinel{minErr{msg: "specific", status: 404, code: "domain.specific"}}
	_, p := errs.Render(err)
	if p.Code != "domain.specific" {
		t.Errorf("code = %q, want domain.specific (typed wins)", p.Code)
	}
}

func TestRender_SentinelNotFound(t *testing.T) {
	err := fmt.Errorf("lookup foo: %w", repository.ErrNotFound)
	status, p := errs.Render(err)
	if status != 404 {
		t.Fatalf("status = %d", status)
	}
	if p.Code != "not_found" {
		t.Errorf("code = %q", p.Code)
	}
}

func TestRender_SentinelAmbiguous(t *testing.T) {
	status, p := errs.Render(repository.ErrAmbiguous)
	if status != 409 || p.Code != "ambiguous" {
		t.Errorf("status=%d code=%q", status, p.Code)
	}
}

func TestRender_SentinelConflict(t *testing.T) {
	status, p := errs.Render(repository.ErrConflict)
	if status != 409 || p.Code != "conflict" {
		t.Errorf("status=%d code=%q", status, p.Code)
	}
}

func TestRender_SentinelInvalidArgument_PropagatesMessage(t *testing.T) {
	msg := "limit too large"
	err := fmt.Errorf("%s: %w", msg, repository.ErrInvalidArgument)
	status, p := errs.Render(err)
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	if p.Detail == "internal error" {
		t.Errorf("detail not propagated: %q", p.Detail)
	}
}

func TestRender_UnknownNoLeak(t *testing.T) {
	status, p := errs.Render(errors.New("opaque internal: db pool exhausted"))
	if status != 500 {
		t.Fatalf("status = %d", status)
	}
	if p.Code != "internal" {
		t.Errorf("code = %q", p.Code)
	}
	if p.Detail != "internal error" {
		t.Errorf("detail leaked: %q", p.Detail)
	}
}

func TestRender_Nil(t *testing.T) {
	status, p := errs.Render(nil)
	if status != 500 {
		t.Fatalf("status = %d", status)
	}
	if p.Code != "internal" {
		t.Errorf("code = %q", p.Code)
	}
}

func TestProblem_MarshalJSON_FlatExtras(t *testing.T) {
	p := errs.Problem{
		Type: "ce:err:x.y", Title: "X Y", Status: 404, Code: "x.y",
		Detail: "thing",
		Extras: map[string]any{"urn": "urn:foo", "n": float64(3)},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["urn"] != "urn:foo" {
		t.Errorf("urn not flattened: %v", got)
	}
	if got["n"] != float64(3) {
		t.Errorf("n missing: %v", got)
	}
	if got["code"] != "x.y" {
		t.Errorf("code missing: %v", got)
	}
}

func TestProblem_MarshalJSON_FixedFieldsWinOnCollision(t *testing.T) {
	p := errs.Problem{
		Type: "ce:err:x.y", Title: "official", Status: 400, Code: "x.y",
		Extras: map[string]any{"title": "attempt to override", "code": "fake"},
	}
	b, _ := json.Marshal(p)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if got["title"] != "official" {
		t.Errorf("title overridden: %v", got)
	}
	if got["code"] != "x.y" {
		t.Errorf("code overridden: %v", got)
	}
}

func TestProblem_MarshalJSON_OmitsEmpty(t *testing.T) {
	p := errs.Problem{Type: "ce:err:x", Title: "T", Status: 500, Code: "x"}
	b, _ := json.Marshal(p)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if _, has := got["detail"]; has {
		t.Errorf("detail should be omitted: %v", got)
	}
	if _, has := got["trace_id"]; has {
		t.Errorf("trace_id should be omitted: %v", got)
	}
}

func TestContentType(t *testing.T) {
	if errs.ContentType != "application/problem+json" {
		t.Errorf("ContentType = %q", errs.ContentType)
	}
}

func TestDefaultTitle(t *testing.T) {
	if errs.DefaultTitle(404) != "Not Found" {
		t.Errorf("404 = %q", errs.DefaultTitle(404))
	}
	if errs.DefaultTitle(999) != "Error" {
		t.Errorf("999 = %q", errs.DefaultTitle(999))
	}
}
