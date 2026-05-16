// Package controller implementa o HTTP do "search plane" (F-016 S-009).
//
// Consome o port `search.NodeSearcher` (narrow, consumer-owned) —
// não importa `repository`. O wiring (cmd/api/main.go) é quem
// constrói o adapter que satisfaz o port a partir do NodeRepository
// real.
package controller

import (
	"net/http"

	"costEngine/internal/app/search"
)

// Controller agrega o port narrow e satisfaz `httpserver.Registrar`.
type Controller struct {
	searcher search.NodeSearcher
}

// New cria o Controller. Searcher nil dispara panic (fail-fast).
func New(s search.NodeSearcher) *Controller {
	if s == nil {
		panic("search/controller: New requires non-nil NodeSearcher")
	}
	return &Controller{searcher: s}
}

// Register registra `/v1/architecture/search` no mux.
func (c *Controller) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/architecture/search", c.handleSearch)
}
