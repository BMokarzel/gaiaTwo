// Package local lê arquivos Parquet de um diretório local. Útil para
// desenvolvimento, fixtures de teste e ingestão de exports manuais do
// CUR (download manual do bucket).
//
// É um leitor low-level: emite linhas como `RawRow` (map de
// columnPath → valor Go). O mapping para `cost.Line` é responsabilidade
// do parser (ver `modules/cost/parser`). Esta camada não conhece
// semântica de CUR.
package local

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// RawRow é uma linha lida do Parquet: column path → valor Go.
//
// Paths usam "." como separador (ex.: `resource_tags_user_env`,
// ou em schemas aninhados `line_item.product_code`).
//
// Valores são primitivos Go (`string`, `int64`, `float64`, `bool`).
// `nil` significa valor NULL. ByteArrays são convertidas em string
// (CUR usa ByteArray para todas as colunas texto).
type RawRow map[string]any

// Reader streama linhas de um arquivo Parquet local. Não é safe para
// uso concorrente.
type Reader struct {
	f          *os.File
	pf         *parquet.File
	columns    []string // index → nome (joined path)
	rowGroups  []parquet.RowGroup
	rgIdx      int
	rows       parquet.Rows
	buf        []parquet.Row
	bufN       int
	bufI       int
	rowsClosed bool
}

// Open abre um arquivo Parquet local. Caller deve chamar Close().
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("local: open %q: %w", path, err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("local: stat %q: %w", path, err)
	}
	pf, err := parquet.OpenFile(f, stat.Size())
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("local: parse parquet %q: %w", path, err)
	}
	cols := pf.Schema().Columns()
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = strings.Join(c, ".")
	}
	return &Reader{
		f:         f,
		pf:        pf,
		columns:   colNames,
		rowGroups: pf.RowGroups(),
		buf:       make([]parquet.Row, 256),
	}, nil
}

// Columns retorna os nomes (column paths) na ordem dos índices.
func (r *Reader) Columns() []string {
	out := make([]string, len(r.columns))
	copy(out, r.columns)
	return out
}

// NumRows retorna o total de linhas no arquivo.
func (r *Reader) NumRows() int64 { return r.pf.NumRows() }

// Next lê a próxima linha. Retorna io.EOF quando o arquivo acaba.
//
// A `RawRow` retornada é reusada internamente entre chamadas no que
// se refere aos slices subjacentes do parquet-go — caller que precise
// reter valores deve copiar strings/[]byte se necessário (strings em
// Go são imutáveis, o cuidado é apenas se manipular bytes brutos).
func (r *Reader) Next() (RawRow, error) {
	for {
		// Drena buffer corrente.
		if r.bufI < r.bufN {
			row := r.buf[r.bufI]
			r.bufI++
			return r.rowToMap(row), nil
		}

		// Buffer exaurido — tenta ler mais do row group corrente.
		if r.rows != nil && !r.rowsClosed {
			n, err := r.rows.ReadRows(r.buf)
			r.bufN = n
			r.bufI = 0
			if n > 0 {
				continue
			}
			if errors.Is(err, io.EOF) {
				_ = r.rows.Close()
				r.rowsClosed = true
				r.rows = nil
				r.rgIdx++
			} else if err != nil {
				_ = r.rows.Close()
				r.rowsClosed = true
				r.rows = nil
				return nil, fmt.Errorf("local: read rows: %w", err)
			}
			continue
		}

		// Próximo row group.
		if r.rgIdx >= len(r.rowGroups) {
			return nil, io.EOF
		}
		r.rows = r.rowGroups[r.rgIdx].Rows()
		r.rowsClosed = false
	}
}

// rowToMap converte uma parquet.Row em RawRow. Para colunas repetidas
// (raras no CUR), só o primeiro valor é mantido — o CUR não usa
// arrays nas colunas operacionais.
func (r *Reader) rowToMap(row parquet.Row) RawRow {
	out := make(RawRow, len(r.columns))
	for _, v := range row {
		if v.IsNull() {
			continue
		}
		col := v.Column()
		if col < 0 || col >= len(r.columns) {
			continue
		}
		name := r.columns[col]
		if _, seen := out[name]; seen {
			continue // mantém primeiro valor de colunas repetidas
		}
		out[name] = valueToGo(v)
	}
	return out
}

// Close libera os recursos. Idempotente.
func (r *Reader) Close() error {
	var err error
	if r.rows != nil && !r.rowsClosed {
		err = r.rows.Close()
		r.rowsClosed = true
	}
	if r.f != nil {
		if cerr := r.f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		r.f = nil
	}
	return err
}

// valueToGo converte um parquet.Value em primitivo Go.
func valueToGo(v parquet.Value) any {
	switch v.Kind() {
	case parquet.Boolean:
		return v.Boolean()
	case parquet.Int32:
		return int64(v.Int32())
	case parquet.Int64:
		return v.Int64()
	case parquet.Float:
		return float64(v.Float())
	case parquet.Double:
		return v.Double()
	case parquet.ByteArray, parquet.FixedLenByteArray:
		// CUR usa ByteArray para todas as strings; cópia via string()
		// é segura (string em Go é imutável).
		return string(v.ByteArray())
	default:
		// Int96 (timestamp legacy) e qualquer kind desconhecido caem em string.
		return v.String()
	}
}
