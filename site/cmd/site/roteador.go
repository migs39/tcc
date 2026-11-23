package main

import (
	"log/slog"
	"mime"
	"net/http"
	"unicode/utf8"

	"tcc/site/web"

	"tcc/protocolo/tema"
)

// Modulo é um conjunto de rotas (admin, catálogo, ...).
type Modulo interface{ Registrar(*http.ServeMux) }

// NovoRoteador monta todas as rotas e as proteções comuns.
func NovoRoteador(tpl *web.Templates, log *slog.Logger, modulos ...Modulo) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(web.Estaticos())))
	mux.Handle("GET /tema/", http.StripPrefix("/tema/", http.FileServerFS(tema.FS)))
	for _, m := range modulos {
		m.Registrar(mux)
	}

	// CSRF: rejeita POST vindo de outra origem (Sec-Fetch-Site / Origin).
	return cabecalhosSeguranca(http.NewCrossOriginProtection().Handler(exigirUTF8(mux)))
}

// exigirUTF8 recusa com 422 um formulário cujos campos não são UTF-8 válido.
// Sem isso, o texto chega ao Postgres e vira erro interno (500). O navegador
// sempre envia UTF-8 (as páginas declaram o charset); isto cobre clientes
// como curl em terminal Latin-1. Formulários multipart (capa, arquivo do
// livro) só levam arquivos e ficam de fora.
func exigirUTF8(prox http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			tipo, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if tipo == "application/x-www-form-urlencoded" && r.ParseForm() == nil && !formularioUTF8(r) {
				http.Error(w, "o formulário tem texto com codificação inválida; use UTF-8", http.StatusUnprocessableEntity)
				return
			}
		}
		prox.ServeHTTP(w, r)
	})
}

func formularioUTF8(r *http.Request) bool {
	for k, vs := range r.PostForm {
		if !utf8.ValidString(k) {
			return false
		}
		for _, v := range vs {
			if !utf8.ValidString(v) {
				return false
			}
		}
	}
	return true
}

// cabecalhosSeguranca: nenhum recurso de fora da própria origem pode ser
// carregado (D-003: sem scripts, fontes ou analytics de terceiros) e nenhum
// Referer é enviado ao sair do site.
func cabecalhosSeguranca(prox http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; "+
				"connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "interest-cohort=(), browsing-topics=()")
		prox.ServeHTTP(w, r)
	})
}
