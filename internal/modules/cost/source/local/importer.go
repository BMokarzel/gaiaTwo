package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/parser"
)

// Importer implementa `cost.CURImporter` lendo Parquet de um diretório
// local. Cada subdiretório `year=YYYY/month=MM/` (ou arquivo `.parquet`
// direto na raiz) é descoberto como uma partition.
//
// Provider é fixo (AWS) — outras nuvens terão Importers próprios.
type Importer struct {
	// Root é o diretório raiz. Pode conter:
	//   - arquivos `.parquet` diretos (1 partition por arquivo); ou
	//   - layout `year=YYYY/month=MM/*.parquet` (estilo S3 CUR).
	Root string
	// ReportID identifica o CUR/relatório para fins de lineage.
	ReportID string
	// NowFn retorna o tempo de ingestão (injetável p/ testes determinísticos).
	NowFn func() time.Time
	// BufferSize controla o tamanho dos canais; 0 ⇒ default.
	BufferSize int
}

// Discover lista partitions disponíveis no diretório `Root`, filtrando
// por `BillingMonth >= since` (use time.Time{} para incluir tudo).
func (im *Importer) Discover(ctx context.Context, since time.Time) ([]cost.Partition, error) {
	if im.Root == "" {
		return nil, fmt.Errorf("local importer: Root empty")
	}
	st, err := os.Stat(im.Root)
	if err != nil {
		return nil, fmt.Errorf("local importer: stat root: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("local importer: Root %q is not a directory", im.Root)
	}

	// Layout 1: year=YYYY/month=MM/*.parquet
	parts, err := discoverPartitioned(im.Root, im.ReportID)
	if err != nil {
		return nil, err
	}

	// Layout 2 (fallback): qualquer .parquet diretamente na raiz ⇒ 1 partition única.
	if len(parts) == 0 {
		var keys []string
		entries, _ := os.ReadDir(im.Root)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".parquet") {
				keys = append(keys, e.Name())
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			parts = append(parts, cost.Partition{
				Provider:     node.ProviderAWS,
				ReportID:     im.ReportID,
				BillingMonth: time.Time{}, // desconhecido
				URI:          "file://" + filepath.ToSlash(im.Root),
				RunID:        "local-flat",
				ObjectKeys:   keys,
			})
		}
	}

	if !since.IsZero() {
		filtered := parts[:0]
		for _, p := range parts {
			if p.BillingMonth.IsZero() || !p.BillingMonth.Before(since) {
				filtered = append(filtered, p)
			}
		}
		parts = filtered
	}
	return parts, nil
}

func discoverPartitioned(root, reportID string) ([]cost.Partition, error) {
	yearEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("local importer: read root: %w", err)
	}
	var out []cost.Partition
	for _, ye := range yearEntries {
		if !ye.IsDir() || !strings.HasPrefix(ye.Name(), "year=") {
			continue
		}
		yearStr := strings.TrimPrefix(ye.Name(), "year=")
		var year int
		if _, err := fmt.Sscanf(yearStr, "%d", &year); err != nil {
			continue
		}
		yearDir := filepath.Join(root, ye.Name())
		monthEntries, err := os.ReadDir(yearDir)
		if err != nil {
			continue
		}
		for _, me := range monthEntries {
			if !me.IsDir() || !strings.HasPrefix(me.Name(), "month=") {
				continue
			}
			monthStr := strings.TrimPrefix(me.Name(), "month=")
			var month int
			if _, err := fmt.Sscanf(monthStr, "%d", &month); err != nil {
				continue
			}
			monthDir := filepath.Join(yearDir, me.Name())
			files, err := os.ReadDir(monthDir)
			if err != nil {
				continue
			}
			var keys []string
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".parquet") {
					keys = append(keys, filepath.ToSlash(filepath.Join(ye.Name(), me.Name(), f.Name())))
				}
			}
			if len(keys) == 0 {
				continue
			}
			sort.Strings(keys)
			out = append(out, cost.Partition{
				Provider:     node.ProviderAWS,
				ReportID:     reportID,
				BillingMonth: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
				URI:          "file://" + filepath.ToSlash(root),
				RunID:        fmt.Sprintf("local-%04d-%02d", year, month),
				ObjectKeys:   keys,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BillingMonth.Before(out[j].BillingMonth) })
	return out, nil
}

// Read streama linhas de todos os arquivos Parquet da partition. As
// linhas inválidas vão pelo canal de erros, sem interromper o stream.
// Cancelamento de ctx encerra ambos os canais.
func (im *Importer) Read(ctx context.Context, p cost.Partition) (cost.ReadResult, error) {
	if p.IsZero() {
		return cost.ReadResult{}, cost.ErrPartitionNotFound
	}
	bufSize := im.BufferSize
	if bufSize <= 0 {
		bufSize = 256
	}
	now := im.NowFn
	if now == nil {
		now = time.Now
	}
	root := strings.TrimPrefix(p.URI, "file://")

	lines := make(chan cost.Line, bufSize)
	errs := make(chan cost.ParseError, bufSize)

	go func() {
		defer close(lines)
		defer close(errs)

		pctx := parser.Context{
			Provider:   p.Provider,
			ReportID:   p.ReportID,
			RunID:      p.RunID,
			IngestedAt: now().UTC(),
		}

		for _, key := range p.ObjectKeys {
			full := filepath.Join(root, filepath.FromSlash(key))
			if err := streamFile(ctx, full, pctx, lines, errs); err != nil {
				// Erro fatal de arquivo: reporta e passa adiante.
				select {
				case errs <- cost.ParseError{
					SourceFile: key,
					Reason:     err.Error(),
					At:         now().UTC(),
				}:
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	return cost.ReadResult{Lines: lines, Errors: errs}, nil
}

func streamFile(ctx context.Context, path string, pctx parser.Context, lines chan<- cost.Line, errs chan<- cost.ParseError) error {
	r, err := Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	pctx.SourceFile = filepath.Base(path)
	var idx int64
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		raw, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		line, perr := parser.Parse(pctx, raw)
		idx++
		if perr != nil {
			select {
			case errs <- cost.ParseError{
				SourceFile: pctx.SourceFile,
				RowIndex:   idx - 1,
				Reason:     perr.Error(),
				At:         pctx.IngestedAt,
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		select {
		case lines <- line:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
