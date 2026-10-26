package pacote

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"tcc/site/web"
)

type Handlers struct {
	svc *Servico
	tpl *web.Templates
	log *slog.Logger
}

func NovosHandlers(svc *Servico, tpl *web.Templates, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, tpl: tpl, log: log}
}

// Registrar monta as rotas públicas: a página de apresentação e download e o
// pacote. Todo visitante recebe exatamente a mesma resposta; nada aqui depende
// de quem pede (L1, D-006).
func (h *Handlers) Registrar(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.inicio)
	mux.HandleFunc("GET /catalogo.zip", h.zip)
	mux.HandleFunc("GET /catalogo/hash", h.hash)
}

type dadosInicio struct {
	web.Base
	Versao         int64
	Livros, Grupos int
	TamanhoKB      int
	SHA256         string
	ChavePublica   string
}

// inicio é a página pública: o que a loja promete, como funciona e os dados
// para conferir que todos recebem o mesmo catálogo.
func (h *Handlers) inicio(w http.ResponseWriter, r *http.Request) {
	d := dadosInicio{ChavePublica: hex.EncodeToString(h.svc.ChavePublica())}
	if p, err := h.svc.Atual(r.Context()); err != nil {
		h.log.Error("gerando pacote para a página inicial", "erro", err)
	} else {
		d.Versao, d.Livros, d.Grupos = p.Versao, p.Livros, p.Grupos
		d.TamanhoKB = (len(p.Dados) + 1023) / 1024
		d.SHA256 = hex.EncodeToString(p.SHA256[:])
	}
	if err := h.tpl.Renderizar(w, http.StatusOK, "inicio", d); err != nil {
		h.log.Error("renderizando início", "erro", err)
	}
}

func (h *Handlers) atual(w http.ResponseWriter, r *http.Request) (*Pacote, bool) {
	p, err := h.svc.Atual(r.Context())
	if err != nil {
		h.log.Error("gerando pacote do catálogo", "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return nil, false
	}
	return p, true
}

func cabecalhos(w http.ResponseWriter, p *Pacote) {
	h := w.Header()
	h.Set("Catalogo-Versao", strconv.FormatInt(p.Versao, 10))
	h.Set("Catalogo-SHA256", hex.EncodeToString(p.SHA256[:]))
	h.Set("Catalogo-Assinatura", base64.StdEncoding.EncodeToString(p.Assinatura))
	// O programa sempre revalida; com ETag igual, recebe 304 sem corpo.
	h.Set("Cache-Control", "no-cache")
}

func (h *Handlers) zip(w http.ResponseWriter, r *http.Request) {
	p, ok := h.atual(w, r)
	if !ok {
		return
	}
	cabecalhos(w, p)
	etag := `"` + hex.EncodeToString(p.SHA256[:]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="catalogo-v`+strconv.FormatInt(p.Versao, 10)+`.zip"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(p.Dados)))
	w.Write(p.Dados)
}

// RespostaHash é o JSON de /catalogo/hash: o "hash público" que qualquer um
// pode comparar para conferir que recebeu o mesmo catálogo que os outros.
type RespostaHash struct {
	Versao     int64  `json:"versao"`
	SHA256     string `json:"sha256"`
	Assinatura string `json:"assinatura"`
	Tamanho    int    `json:"tamanho"`
}

func (h *Handlers) hash(w http.ResponseWriter, r *http.Request) {
	p, ok := h.atual(w, r)
	if !ok {
		return
	}
	cabecalhos(w, p)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RespostaHash{
		Versao:     p.Versao,
		SHA256:     hex.EncodeToString(p.SHA256[:]),
		Assinatura: base64.StdEncoding.EncodeToString(p.Assinatura),
		Tamanho:    len(p.Dados),
	})
}
