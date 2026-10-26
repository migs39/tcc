package pacote

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

type Handlers struct {
	svc *Servico
	log *slog.Logger
}

func NovosHandlers(svc *Servico, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, log: log}
}

// Registrar monta as rotas públicas do pacote. Todo visitante recebe
// exatamente a mesma resposta; nada aqui depende de quem pede (L1, D-006).
func (h *Handlers) Registrar(mux *http.ServeMux) {
	mux.HandleFunc("GET /catalogo.zip", h.zip)
	mux.HandleFunc("GET /catalogo/hash", h.hash)
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
