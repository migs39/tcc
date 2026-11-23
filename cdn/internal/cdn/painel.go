package cdn

import (
	"context"
	"crypto/subtle"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"tcc/protocolo/tema"
)

// Painel são os números da página /painel. Nada aqui liga um token a um livro
// ou a um horário: só contagens.
type Painel struct {
	Livros, Tokens, Pendentes, LoteMinimo int
	Intervalo                             string
	UltimoLote                            string
	VersaoCatalogo                        int64
	Faixas                                int
}

func (c *CDN) painel(ctx context.Context) (Painel, error) {
	p := Painel{LoteMinimo: c.cfg.LoteMinimo, Intervalo: duracao(c.cfg.IntervaloLote)}
	c.mu.RLock()
	semCatalogo := c.versaoCat == 0
	c.mu.RUnlock()
	if semCatalogo {
		c.AtualizarCatalogo(ctx) // a CDN pode ter subido antes do site
	}
	arqs, _ := filepath.Glob(filepath.Join(c.cfg.Pasta, "*.bin"))
	p.Livros = len(arqs)
	if err := c.pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE NOT reportado) FROM tokens_usados`).Scan(&p.Tokens, &p.Pendentes); err != nil {
		return p, err
	}
	c.mu.RLock()
	p.VersaoCatalogo, p.Faixas = c.versaoCat, len(c.chaves)
	ultimo, enviou := c.ultimoLote, c.enviouLote
	c.mu.RUnlock()
	p.UltimoLote = "nenhum desde que a CDN foi iniciada"
	if enviou {
		p.UltimoLote = "há " + duracao(time.Since(ultimo))
	}
	return p, nil
}

func duracao(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return strconv.Itoa(int(d.Hours())) + " h"
	case d >= time.Minute:
		return strconv.Itoa(int(d.Minutes())) + " min"
	}
	return "menos de 1 min"
}

var tplPainel = template.Must(template.New("painel").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>CDN · painel</title>
<link rel="stylesheet" href="/tema/tema.css">
<style>
.pagina { max-width: 980px; margin: 0 auto; padding: 32px clamp(16px, 4vw, 40px) 56px; display: grid; gap: 20px; }
.topo { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.topo h1 { margin: 0; font-size: 28px; }
dl { margin: 0; display: grid; grid-template-columns: max-content 1fr; gap: 8px 20px; font-size: 14px; }
dt { color: var(--suave); }
dd { margin: 0; }
</style>
</head>
<body>
<main class="pagina">
  <div class="topo">
    <a class="marca-ot" href="/painel"><span class="icone">{{.Icone}}</span>CDN</a>
    {{if .P.VersaoCatalogo}}<span class="chip ativo">catálogo v{{.P.VersaoCatalogo}} · assinatura conferida</span>{{else}}<span class="chip" style="color: var(--alerta); border-color: var(--alerta)">catálogo indisponível: site fora do ar?</span>{{end}}
  </div>
  <h1>Entregas por token</h1>
  <div class="stats">
    <div class="stat"><span>Livros cifrados</span><strong>{{.P.Livros}}</strong><span>guardados nesta CDN</span></div>
    <div class="stat"><span>Tokens usados</span><strong>{{.P.Tokens}}</strong><span>uso único (RF17)</span></div>
    <div class="stat"><span>Lote pendente</span><strong>{{.P.Pendentes}} / {{.P.LoteMinimo}}</strong><span>ou a cada {{.P.Intervalo}}</span></div>
    <div class="stat"><span>Faixas de preço</span><strong>{{.P.Faixas}}</strong><span>uma chave de token cada</span></div>
  </div>
  <section class="cartao">
    <h2>Reporte ao site (RF18)</h2>
    <dl>
      <dt>Último lote</dt><dd>{{.P.UltimoLote}}</dd>
      <dt>Envia quando</dt><dd>juntar {{.P.LoteMinimo}} tokens ou passar {{.P.Intervalo}}</dd>
      <dt>O lote leva</dt><dd>os tokens, embaralhados, e a quantidade</dd>
      <dt>O lote não leva</dt><dd>o livro de cada token, horários, endereços</dd>
    </dl>
  </section>
  <p class="nota">Esta CDN não registra endereço, horário nem livro por requisição. Ela sabe quais livros foram baixados, mas não quem os comprou.</p>
</main>
</body>
</html>`))

// rotasPainel monta /painel (Basic auth: qualquer usuário, senha = segredo) e
// /tema/.
func (c *CDN) rotasPainel(mux *http.ServeMux) {
	mux.Handle("GET /tema/", http.StripPrefix("/tema/", http.FileServerFS(tema.FS)))
	mux.HandleFunc("GET /painel", func(w http.ResponseWriter, r *http.Request) {
		_, senha, ok := r.BasicAuth()
		if !ok || c.cfg.Segredo == "" || subtle.ConstantTimeCompare([]byte(senha), []byte(c.cfg.Segredo)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="CDN", charset="UTF-8"`)
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		p, err := c.painel(r.Context())
		if err != nil {
			http.Error(w, "erro interno", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		tplPainel.Execute(w, map[string]any{"P": p, "Icone": template.HTML(tema.Icone("#04110a"))})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/painel", http.StatusSeeOther)
	})
}
