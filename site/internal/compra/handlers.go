package compra

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"tcc/protocolo"
	"tcc/site/internal/admin"
	"tcc/site/web"
)

// TamanhoMaximoLivro limita o arquivo enviado pelo administrador.
const TamanhoMaximoLivro = 200 << 20

// Exigidor protege rotas administrativas (sessão do RF01).
type Exigidor interface {
	Exigir(http.HandlerFunc) http.HandlerFunc
}

type Handlers struct {
	tpl        *web.Templates
	svc        *Servico
	adm        Exigidor
	segredoCDN string
	log        *slog.Logger
}

func NovosHandlers(svc *Servico, adm Exigidor, segredoCDN string, tpl *web.Templates, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, adm: adm, segredoCDN: segredoCDN, tpl: tpl, log: log}
}

// Registrar monta as rotas. As da API são usadas pelo programa local e pela
// CDN; nenhuma resposta ou log depende dos livros escolhidos.
func (h *Handlers) Registrar(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/compras", h.comprar)
	mux.HandleFunc("POST /api/compras/{id}/reenvio", h.reenviar)
	mux.HandleFunc("POST /api/cdn/reporte", h.reporte)
	mux.HandleFunc("POST /admin/livros/{id}/arquivo", h.adm.Exigir(h.publicar))
	mux.HandleFunc("GET /admin/cdn", h.adm.Exigir(h.painelCDN))
}

type dadosCDN struct {
	web.Base
	ResumoCDN
}

func (h *Handlers) painelCDN(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Lotes(r.Context())
	if err != nil {
		h.log.Error("lendo lotes da CDN", "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	if err := h.tpl.Renderizar(w, http.StatusOK, "cdn", dadosCDN{admin.Base(r, ""), res}); err != nil {
		h.log.Error("renderizando página", "pagina", "cdn", "erro", err)
	}
}

func jsonResp(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type erroAPI struct {
	Erro string `json:"erro"`
}

func (h *Handlers) interno(w http.ResponseWriter, onde string, err error) {
	// Só o tipo da operação vai para o log: nada do pedido.
	h.log.Error("erro interno", "onde", onde, "erro", err)
	jsonResp(w, http.StatusInternalServerError, erroAPI{"erro interno"})
}

func (h *Handlers) comprar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var p protocolo.PedidoCompra
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonResp(w, http.StatusBadRequest, erroAPI{"pedido malformado"})
		return
	}
	res, err := h.svc.Comprar(r.Context(), p)
	var ec ErroCompra
	switch {
	case errors.As(err, &ec):
		jsonResp(w, http.StatusUnprocessableEntity, erroAPI{ec.Msg})
	case err != nil:
		h.interno(w, "compra", err)
	case res.Status != "aprovada":
		jsonResp(w, http.StatusPaymentRequired, res)
	default:
		jsonResp(w, http.StatusOK, res)
	}
}

var idCompra = regexp.MustCompile(`^[0-9a-f]{32}$`)

// reenviar responde igual exista ou não a compra, para não servir de oráculo.
func (h *Handlers) reenviar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if idCompra.MatchString(id) {
		if err := h.svc.Reenviar(r.Context(), id); err != nil && !errors.Is(err, ErrNaoEncontrada) {
			h.interno(w, "reenvio", err)
			return
		}
	}
	jsonResp(w, http.StatusAccepted, map[string]string{
		"mensagem": "se a compra existir e estiver no prazo, a resposta do OT foi reenviada ao e-mail original",
	})
}

func (h *Handlers) reporte(w http.ResponseWriter, r *http.Request) {
	if !AutorizadoPorSegredo(r, h.segredoCDN) {
		jsonResp(w, http.StatusUnauthorized, erroAPI{"não autorizado"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	var rep protocolo.ReporteCDN
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		jsonResp(w, http.StatusBadRequest, erroAPI{"lote malformado"})
		return
	}
	n, err := h.svc.ReceberReporte(r.Context(), rep)
	var er ErroReporte
	switch {
	case errors.As(err, &er):
		jsonResp(w, http.StatusConflict, erroAPI{er.Msg})
	case err != nil:
		h.interno(w, "reporte", err)
	default:
		jsonResp(w, http.StatusOK, map[string]int{"aceitos": n})
	}
}

func (h *Handlers) publicar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	destino := "/admin/livros/" + strconv.FormatInt(id, 10)
	r.Body = http.MaxBytesReader(w, r.Body, TamanhoMaximoLivro+64<<10)
	arq, _, err := r.FormFile("arquivo")
	if err != nil {
		http.Redirect(w, r, destino+"?aviso=arquivo-invalido", http.StatusSeeOther)
		return
	}
	defer arq.Close()
	dados, err := io.ReadAll(arq)
	if err != nil || len(dados) == 0 {
		http.Redirect(w, r, destino+"?aviso=arquivo-invalido", http.StatusSeeOther)
		return
	}
	if err := h.svc.PublicarLivro(r.Context(), id, dados); err != nil {
		h.log.Error("publicando livro na CDN", "erro", err)
		http.Redirect(w, r, destino+"?aviso=arquivo-falhou", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, destino+"?aviso=arquivo-publicado", http.StatusSeeOther)
}
