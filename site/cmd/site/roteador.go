package main

import (
	"log/slog"
	"net/http"

	"tcc/site/internal/admin"
	"tcc/site/web"
)

// NovoRoteador monta todas as rotas e as proteções comuns.
func NovoRoteador(tpl *web.Templates, adm *admin.Handlers, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(web.Estaticos())))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := tpl.Renderizar(w, http.StatusOK, "inicio", web.Base{}); err != nil {
			log.Error("renderizando início", "erro", err)
		}
	})
	adm.Registrar(mux)

	// CSRF: rejeita POST vindo de outra origem (Sec-Fetch-Site / Origin).
	return cabecalhosSeguranca(http.NewCrossOriginProtection().Handler(mux))
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
