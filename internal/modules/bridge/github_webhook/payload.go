package github_webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"costEngine/internal/entity/node"
)

// ErrIgnored é um sentinela "soft" — payload válido, mas não há nada
// pra fazer (ex.: PR não-merged, action diferente de "closed", nenhuma
// label `feature:`). Webhook deve responder 200 e logar info.
var ErrIgnored = errors.New("github_webhook: payload ignored")

// ErrMalformed indica JSON inválido ou campos obrigatórios ausentes.
// Caller deve responder 400.
var ErrMalformed = errors.New("github_webhook: malformed payload")

// labelPrefix é o esquema convencionado para amarrar um PR a uma
// Feature: `feature:<feature_urn>` (ADR sem número; F-013 §Notas).
const labelPrefix = "feature:"

// Payload é a forma mínima do evento `pull_request` que este módulo
// consome. Campos extra do JSON são ignorados (forward-compat).
type Payload struct {
	Action      string
	Merged      bool
	MergedAt    time.Time
	HTMLURL     string
	FeatureURNs []node.URN // extraídos de labels `feature:<urn>`
	Files       []string   // paths relativos ao root do repo
	Repo        string     // full_name (`owner/repo`)
}

// rawPR cobre só os campos que olhamos. Decoder ignora o resto.
type rawPR struct {
	Action      string `json:"action"`
	PullRequest struct {
		HTMLURL  string     `json:"html_url"`
		Merged   bool       `json:"merged"`
		MergedAt *time.Time `json:"merged_at"`
		Labels   []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Head struct {
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	// Files é um campo opcional injetado pelo proxy/CI que enriquece o
	// payload do GitHub com a lista de arquivos do PR (o evento nativo
	// não inclui `files`). Se ausente, callers devem hidratar via API
	// posteriormente — fora do escopo do MVP.
	Files []struct {
		Filename string `json:"filename"`
	} `json:"files"`
}

// Parse decodifica o body JSON e aplica os filtros do F-013.
//
// Retorna:
//   - (*Payload, nil)             — evento útil, prosseguir;
//   - (nil, ErrIgnored)           — válido mas no-op (log info);
//   - (nil, ErrMalformed wrapped) — body inválido (400).
func Parse(body []byte) (*Payload, error) {
	var raw rawPR
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if raw.Action != "closed" {
		return nil, ErrIgnored
	}
	if !raw.PullRequest.Merged {
		return nil, ErrIgnored
	}
	if raw.PullRequest.MergedAt == nil {
		return nil, fmt.Errorf("%w: merged_at missing", ErrMalformed)
	}

	repo := raw.Repository.FullName
	if repo == "" {
		repo = raw.PullRequest.Head.Repo.FullName
	}
	if repo == "" {
		return nil, fmt.Errorf("%w: repo missing", ErrMalformed)
	}

	var features []node.URN
	for _, l := range raw.PullRequest.Labels {
		if !strings.HasPrefix(l.Name, labelPrefix) {
			continue
		}
		urn := node.URN(strings.TrimPrefix(l.Name, labelPrefix))
		if urn == "" {
			continue
		}
		features = append(features, urn)
	}
	if len(features) == 0 {
		return nil, ErrIgnored
	}

	files := make([]string, 0, len(raw.Files))
	for _, f := range raw.Files {
		if f.Filename != "" {
			files = append(files, f.Filename)
		}
	}

	return &Payload{
		Action:      raw.Action,
		Merged:      true,
		MergedAt:    raw.PullRequest.MergedAt.UTC(),
		HTMLURL:     raw.PullRequest.HTMLURL,
		FeatureURNs: features,
		Files:       files,
		Repo:        repo,
	}, nil
}
