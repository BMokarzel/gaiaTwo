package hris

import (
	"errors"
	"strings"
	"testing"
)

const goodCSV = `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Manager,Platform,Checkout,,2020-03-01,
carol@x.com,Carol,Eng,Product,Growth,bob@x.com,2023-06-01,2026-04-30
`

func TestParse_Happy(t *testing.T) {
	rows, errs, err := Parse(strings.NewReader(goodCSV))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errs: %v", errs)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0].Email != "alice@x.com" || rows[0].Name != "Alice" || rows[0].Team != "Platform" {
		t.Errorf("rows[0]=%+v", rows[0])
	}
	if rows[0].EndDate != nil {
		t.Errorf("rows[0].EndDate should be nil")
	}
	if rows[2].EndDate == nil || rows[2].EndDate.Year() != 2026 {
		t.Errorf("rows[2].EndDate=%v", rows[2].EndDate)
	}
}

func TestParse_BadHeader(t *testing.T) {
	body := `wrong,header
a,b
`
	_, _, err := Parse(strings.NewReader(body))
	if !errors.Is(err, ErrHeader) {
		t.Errorf("want ErrHeader, got %v", err)
	}
}

func TestParse_RowErrorsNonFatal(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
,NoEmail,Eng,Platform,Checkout,,2024-01-15,
bob@x.com,,Eng,Platform,Checkout,,2024-01-15,
carol@x.com,Carol,Eng,,,bob@x.com,2024-01-15,
dave@x.com,Dave,Eng,Platform,Checkout,,bad-date,
eve@x.com,Eve,Eng,Platform,Checkout,,2024-01-15,bad-end-date
frank@x.com,Frank,Eng,Platform,Checkout,,2024-01-15,
`
	rows, errs, err := Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("rows=%d want 2 (alice, frank)", len(rows))
	}
	if len(errs) != 5 {
		t.Errorf("errs=%d want 5", len(errs))
	}
	// Errors carry line numbers
	for _, e := range errs {
		if e.LineNum < 3 {
			t.Errorf("unexpected line %d in %v", e.LineNum, e)
		}
	}
}

func TestParse_EmptyFile(t *testing.T) {
	_, _, err := Parse(strings.NewReader(""))
	if !errors.Is(err, ErrHeader) {
		t.Errorf("want ErrHeader on empty file, got %v", err)
	}
}
