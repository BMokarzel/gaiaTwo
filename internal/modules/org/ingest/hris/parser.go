package hris

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Row é uma linha CSV validada e normalizada.
type Row struct {
	LineNum     int    // número da linha original (1-indexado; cabeçalho é 1)
	Email       string // raw (não persistir — usar EmailHash)
	Name        string
	Role        string
	Team        string
	Squad       string
	ManagerMail string    // raw email do manager
	StartDate   time.Time // start_date
	EndDate     *time.Time // nil se em branco
}

// RowError sinaliza falha em uma linha. Não-fatal — o ingest continua.
type RowError struct {
	LineNum int
	Field   string
	Message string
}

func (e RowError) Error() string {
	return fmt.Sprintf("line %d: %s: %s", e.LineNum, e.Field, e.Message)
}

// expectedHeaders define a ordem canônica do CSV (F-010 spec).
var expectedHeaders = []string{
	"email", "name", "role", "team", "squad",
	"manager_email", "start_date", "end_date_or_blank",
}

// ErrHeader é retornado quando o header não bate. Distinguir do RowError
// (fatal vs. não-fatal) é importante: header errado significa CSV
// totalmente errado.
var ErrHeader = errors.New("hris: invalid header")

// Parse lê CSV de `r` e devolve (linhas válidas, erros por linha).
// Erro fatal só em (a) header inválido ou (b) I/O. Linhas mal-formadas
// individuais viram `RowError` no slice retornado.
//
// Datas aceitam "YYYY-MM-DD"; end_date_or_blank vazio → nil.
func Parse(r io.Reader) ([]Row, []RowError, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1 // valida manualmente para emitir RowError útil
	rd.TrimLeadingSpace = true

	rec, err := rd.Read()
	if err == io.EOF {
		return nil, nil, fmt.Errorf("%w: empty file", ErrHeader)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read header: %v", ErrHeader, err)
	}
	if err := validateHeader(rec); err != nil {
		return nil, nil, err
	}

	var rows []Row
	var errs []RowError

	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		line := lineOf(rd) // posição do registro atual
		if err != nil {
			errs = append(errs, RowError{LineNum: line, Field: "row", Message: err.Error()})
			continue
		}
		row, rerr := parseRow(rec, line)
		if rerr != nil {
			errs = append(errs, *rerr)
			continue
		}
		rows = append(rows, row)
	}
	return rows, errs, nil
}

func validateHeader(rec []string) error {
	if len(rec) != len(expectedHeaders) {
		return fmt.Errorf("%w: want %d cols %v, got %d", ErrHeader,
			len(expectedHeaders), expectedHeaders, len(rec))
	}
	for i, h := range expectedHeaders {
		if strings.ToLower(strings.TrimSpace(rec[i])) != h {
			return fmt.Errorf("%w: col %d want %q, got %q", ErrHeader, i, h, rec[i])
		}
	}
	return nil
}

// parseRow valida e converte uma linha. Retorna *RowError em vez de
// (Row, error) para o caller acumular no slice sem ifs extras.
func parseRow(rec []string, line int) (Row, *RowError) {
	if len(rec) != len(expectedHeaders) {
		return Row{}, &RowError{LineNum: line, Field: "row",
			Message: fmt.Sprintf("expected %d cols, got %d", len(expectedHeaders), len(rec))}
	}
	email := strings.TrimSpace(rec[0])
	name := strings.TrimSpace(rec[1])
	role := strings.TrimSpace(rec[2])
	team := strings.TrimSpace(rec[3])
	squad := strings.TrimSpace(rec[4])
	mgr := strings.TrimSpace(rec[5])
	startStr := strings.TrimSpace(rec[6])
	endStr := strings.TrimSpace(rec[7])

	if email == "" {
		return Row{}, &RowError{LineNum: line, Field: "email", Message: "required"}
	}
	if name == "" {
		return Row{}, &RowError{LineNum: line, Field: "name", Message: "required"}
	}
	if team == "" || squad == "" {
		return Row{}, &RowError{LineNum: line, Field: "team/squad", Message: "team and squad are required"}
	}
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return Row{}, &RowError{LineNum: line, Field: "start_date",
			Message: fmt.Sprintf("invalid date %q (want YYYY-MM-DD)", startStr)}
	}
	var endPtr *time.Time
	if endStr != "" {
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return Row{}, &RowError{LineNum: line, Field: "end_date_or_blank",
				Message: fmt.Sprintf("invalid date %q (want YYYY-MM-DD or empty)", endStr)}
		}
		endPtr = &end
	}

	return Row{
		LineNum:     line,
		Email:       email,
		Name:        name,
		Role:        role,
		Team:        team,
		Squad:       squad,
		ManagerMail: mgr,
		StartDate:   start,
		EndDate:     endPtr,
	}, nil
}

// lineOf retorna o número da linha do último record lido. csv.Reader
// expõe via FieldPos — usamos o início do registro (1-based).
func lineOf(rd *csv.Reader) int {
	l, _ := rd.FieldPos(0)
	return l
}
