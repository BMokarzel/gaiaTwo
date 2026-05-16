.PHONY: build test test-race test-integration cover lint fmt tidy vuln clean help

GO          ?= go
PKG         := ./...
COVER_FILE  := coverage.out

help: ## Lista targets disponíveis
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: ## Compila todos os pacotes
	$(GO) build $(PKG)

test: ## Roda testes unitários
	$(GO) test $(PKG)

test-race: ## Roda testes com detector de race
	$(GO) test -race $(PKG)

test-integration: ## Roda integration tests (precisa NEO4J_TEST_URI; sem env, skip)
	$(GO) test -tags=integration $(PKG)

cover: ## Gera relatório de cobertura
	$(GO) test -coverprofile=$(COVER_FILE) $(PKG)
	$(GO) tool cover -func=$(COVER_FILE) | tail -1

cover-html: cover ## Abre cobertura em HTML
	$(GO) tool cover -html=$(COVER_FILE)

lint: ## Roda golangci-lint
	golangci-lint run $(PKG)

fmt: ## Formata o código com gofumpt
	gofumpt -w .

tidy: ## Limpa dependências
	$(GO) mod tidy

vuln: ## Verifica vulnerabilidades conhecidas
	govulncheck $(PKG)

clean: ## Remove artefatos
	rm -f $(COVER_FILE)
	$(GO) clean $(PKG)
