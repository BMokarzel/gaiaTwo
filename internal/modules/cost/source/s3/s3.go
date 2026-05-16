// Package s3 implementa `cost.CURImporter` lendo CUR Parquet do S3.
//
// Layout esperado:
//
//	s3://<bucket>/<prefix>/<report-name>/year=YYYY/month=MM/<files...>.parquet
//
// CUR v1 também entrega `<report-name>-Manifest.json` por mês com a
// lista canônica de arquivos a consumir. Se um manifest for encontrado,
// usamos `reportKeys` dele — caso contrário fazemos listagem direta da
// prefix `month=MM/`.
//
// Estratégia de download: cada arquivo Parquet é baixado para um
// diretório temporário (`os.TempDir()/ce-cur-<runid>`) e lido via
// `source/local.Open`. ReaderAt sobre S3 (byte-range) é uma otimização
// futura; o MVP prioriza correção sobre throughput.
package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/parser"
	"costEngine/internal/modules/cost/source/local"
)

// S3Client é o subset do SDK AWS S3 usado pelo importer. Permite mock
// nos testes sem dependência de rede.
type S3Client interface {
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Importer implementa `cost.CURImporter` lendo de um bucket S3.
type Importer struct {
	Client     S3Client
	Bucket     string
	Prefix     string // ex.: "cur/my-report" (sem trailing slash)
	ReportID   string
	NowFn      func() time.Time
	BufferSize int
	TempDir    string // se vazio, usa os.TempDir()
}

// manifest é a representação simplificada do `Manifest.json` do CUR v1.
type manifest struct {
	AssemblyID string   `json:"assemblyId"`
	ReportKeys []string `json:"reportKeys"`
}

// Discover lista partitions disponíveis. Mes (year=YYYY/month=MM/) é
// detectado por listagem de "commom prefixes". Quando um manifest existe
// no diretório do mês, suas `reportKeys` são usadas como `ObjectKeys`.
func (im *Importer) Discover(ctx context.Context, since time.Time) ([]cost.Partition, error) {
	if im.Client == nil || im.Bucket == "" {
		return nil, fmt.Errorf("s3 importer: Client/Bucket required")
	}
	root := strings.TrimSuffix(im.Prefix, "/") + "/"

	// 1. Lista subdirectórios year=YYYY/
	years, err := listSubdirs(ctx, im.Client, im.Bucket, root)
	if err != nil {
		return nil, fmt.Errorf("s3 importer: list years: %w", err)
	}

	var parts []cost.Partition
	for _, y := range years {
		yearStr := strings.TrimSuffix(strings.TrimPrefix(y, root+"year="), "/")
		var year int
		if _, err := fmt.Sscanf(yearStr, "%d", &year); err != nil {
			continue
		}
		months, err := listSubdirs(ctx, im.Client, im.Bucket, y)
		if err != nil {
			return nil, err
		}
		for _, m := range months {
			monthStr := strings.TrimSuffix(strings.TrimPrefix(m, y+"month="), "/")
			var mon int
			if _, err := fmt.Sscanf(monthStr, "%d", &mon); err != nil {
				continue
			}
			part, err := im.partitionForMonth(ctx, m, year, mon)
			if err != nil {
				return nil, err
			}
			if part.IsZero() || len(part.ObjectKeys) == 0 {
				continue
			}
			parts = append(parts, part)
		}
	}

	if !since.IsZero() {
		filtered := parts[:0]
		for _, p := range parts {
			if !p.BillingMonth.Before(since) {
				filtered = append(filtered, p)
			}
		}
		parts = filtered
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].BillingMonth.Before(parts[j].BillingMonth) })
	return parts, nil
}

// partitionForMonth resolve as object keys de uma partition (year, mon).
// Tenta o manifest CUR primeiro; em fallback lista os Parquet no prefixo.
func (im *Importer) partitionForMonth(ctx context.Context, monthPrefix string, year, mon int) (cost.Partition, error) {
	keys, runID, err := im.findKeysViaManifest(ctx, monthPrefix)
	if err != nil {
		return cost.Partition{}, err
	}
	if len(keys) == 0 {
		keys, err = listParquetUnder(ctx, im.Client, im.Bucket, monthPrefix)
		if err != nil {
			return cost.Partition{}, err
		}
		runID = fmt.Sprintf("s3-%04d-%02d", year, mon)
	}
	sort.Strings(keys)
	return cost.Partition{
		Provider:     node.ProviderAWS,
		ReportID:     im.ReportID,
		BillingMonth: time.Date(year, time.Month(mon), 1, 0, 0, 0, 0, time.UTC),
		URI:          fmt.Sprintf("s3://%s/%s", im.Bucket, strings.TrimSuffix(im.Prefix, "/")),
		Manifest:     monthPrefix + "Manifest.json",
		RunID:        runID,
		ObjectKeys:   keys,
	}, nil
}

func (im *Importer) findKeysViaManifest(ctx context.Context, monthPrefix string) ([]string, string, error) {
	// Procura o primeiro arquivo terminado em "-Manifest.json" no prefixo.
	out, err := im.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &im.Bucket,
		Prefix: &monthPrefix,
	})
	if err != nil {
		return nil, "", err
	}
	for _, obj := range out.Contents {
		key := *obj.Key
		if !strings.HasSuffix(strings.ToLower(key), "manifest.json") {
			continue
		}
		got, err := im.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &im.Bucket, Key: &key})
		if err != nil {
			return nil, "", err
		}
		var mf manifest
		data, err := io.ReadAll(got.Body)
		_ = got.Body.Close()
		if err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(data, &mf); err != nil {
			return nil, "", err
		}
		return mf.ReportKeys, mf.AssemblyID, nil
	}
	return nil, "", nil
}

// listSubdirs retorna apenas os "common prefixes" (subdirs) com delimiter "/".
func listSubdirs(ctx context.Context, c S3Client, bucket, prefix string) ([]string, error) {
	delim := "/"
	var out []string
	var token *string
	for {
		page, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            &bucket,
			Prefix:            &prefix,
			Delimiter:         &delim,
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, cp := range page.CommonPrefixes {
			out = append(out, *cp.Prefix)
		}
		if page.NextContinuationToken == nil {
			break
		}
		token = page.NextContinuationToken
	}
	return out, nil
}

func listParquetUnder(ctx context.Context, c S3Client, bucket, prefix string) ([]string, error) {
	var out []string
	var token *string
	for {
		page, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            &bucket,
			Prefix:            &prefix,
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if strings.HasSuffix(strings.ToLower(*obj.Key), ".parquet") {
				out = append(out, *obj.Key)
			}
		}
		if page.NextContinuationToken == nil {
			break
		}
		token = page.NextContinuationToken
	}
	return out, nil
}

// Read baixa as object keys da partition para um diretório temporário
// e streama linhas via `source/local`. O diretório é removido após
// fechamento dos canais.
func (im *Importer) Read(ctx context.Context, p cost.Partition) (cost.ReadResult, error) {
	if p.IsZero() {
		return cost.ReadResult{}, cost.ErrPartitionNotFound
	}
	if im.Client == nil {
		return cost.ReadResult{}, fmt.Errorf("s3 importer: Client nil")
	}
	bufSize := im.BufferSize
	if bufSize <= 0 {
		bufSize = 256
	}
	now := im.NowFn
	if now == nil {
		now = time.Now
	}

	tempBase := im.TempDir
	if tempBase == "" {
		tempBase = os.TempDir()
	}
	tmp, err := os.MkdirTemp(tempBase, "ce-cur-")
	if err != nil {
		return cost.ReadResult{}, fmt.Errorf("s3 importer: mkdtemp: %w", err)
	}

	lines := make(chan cost.Line, bufSize)
	errs := make(chan cost.ParseError, bufSize)

	go func() {
		defer close(lines)
		defer close(errs)
		defer os.RemoveAll(tmp)

		pctx := parser.Context{
			Provider:   p.Provider,
			ReportID:   p.ReportID,
			RunID:      p.RunID,
			IngestedAt: now().UTC(),
		}

		for _, key := range p.ObjectKeys {
			local, err := im.download(ctx, key, tmp)
			if err != nil {
				select {
				case errs <- cost.ParseError{SourceFile: key, Reason: "download: " + err.Error(), At: now().UTC()}:
				case <-ctx.Done():
					return
				}
				continue
			}
			if err := streamFile(ctx, local, pctx, lines, errs); err != nil {
				select {
				case errs <- cost.ParseError{SourceFile: key, Reason: err.Error(), At: now().UTC()}:
				case <-ctx.Done():
					return
				}
			}
			_ = os.Remove(local)
			if ctx.Err() != nil {
				return
			}
		}
	}()
	return cost.ReadResult{Lines: lines, Errors: errs}, nil
}

func (im *Importer) download(ctx context.Context, key, dir string) (string, error) {
	out, err := im.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &im.Bucket, Key: &key})
	if err != nil {
		return "", err
	}
	defer out.Body.Close()
	dst := filepath.Join(dir, filepath.Base(key))
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return "", err
	}
	return dst, nil
}

func streamFile(ctx context.Context, path string, pctx parser.Context, lines chan<- cost.Line, errs chan<- cost.ParseError) error {
	r, err := local.Open(path)
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
			case errs <- cost.ParseError{SourceFile: pctx.SourceFile, RowIndex: idx - 1, Reason: perr.Error(), At: pctx.IngestedAt}:
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
