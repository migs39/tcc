package catalogo

import (
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"tcc/site/internal/admin"
	"tcc/site/web"
)

var avisos = map[string]string{
	"livro-cadastrado":    "Livro cadastrado.",
	"livro-salvo":         "Livro salvo.",
	"livro-desativado":    "Livro removido do catálogo.",
	"livro-reativado":     "Livro devolvido ao catálogo.",
	"capa-salva":          "Capa atualizada.",
	"capa-removida":       "Capa removida.",
	"faixa-criada":        "Faixa de preço criada.",
	"faixa-desativada":    "Faixa de preço desativada.",
	"faixa-reativada":     "Faixa de preço reativada.",
	"categoria-criada":    "Categoria criada.",
	"categoria-renomeada": "Categoria renomeada.",
	"categoria-removida":  "Categoria removida.",
}

type Handlers struct {
	svc *Servico
	adm *admin.Handlers
	tpl *web.Templates
	log *slog.Logger
}

func NovosHandlers(svc *Servico, adm *admin.Handlers, tpl *web.Templates, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, adm: adm, tpl: tpl, log: log}
}

// Registrar monta as rotas do catálogo. As administrativas exigem sessão (RF01).
func (h *Handlers) Registrar(mux *http.ServeMux) {
	e := h.adm.Exigir
	mux.HandleFunc("GET /admin/livros", e(h.listarLivros))
	mux.HandleFunc("GET /admin/livros/novo", e(h.telaNovoLivro))
	mux.HandleFunc("POST /admin/livros", e(h.cadastrarLivro))
	mux.HandleFunc("GET /admin/livros/{id}", e(h.telaEditarLivro))
	mux.HandleFunc("POST /admin/livros/{id}", e(h.salvarLivro))
	mux.HandleFunc("POST /admin/livros/{id}/desativar", e(h.desativarLivro))
	mux.HandleFunc("POST /admin/livros/{id}/reativar", e(h.reativarLivro))
	mux.HandleFunc("POST /admin/livros/{id}/capa", e(h.enviarCapa))
	mux.HandleFunc("POST /admin/livros/{id}/capa/remover", e(h.removerCapa))

	mux.HandleFunc("GET /admin/precos", e(h.listarFaixas))
	mux.HandleFunc("POST /admin/precos", e(h.criarFaixa))
	mux.HandleFunc("POST /admin/precos/{id}/desativar", e(h.desativarFaixa))
	mux.HandleFunc("POST /admin/precos/{id}/reativar", e(h.reativarFaixa))

	mux.HandleFunc("GET /admin/categorias", e(h.listarCategorias))
	mux.HandleFunc("POST /admin/categorias", e(h.criarCategoria))
	mux.HandleFunc("POST /admin/categorias/{id}", e(h.renomearCategoria))
	mux.HandleFunc("POST /admin/categorias/{id}/remover", e(h.removerCategoria))

	// Só para a área administrativa. Não há rota pública de capa avulsa: o
	// programa local recebe as miniaturas de todos os livros no pacote do
	// catálogo (D-006); pedir uma capa por vez revelaria o interesse (L1).
	mux.HandleFunc("GET /admin/livros/{id}/capa", e(h.capa))
	mux.HandleFunc("GET /admin/livros/{id}/miniatura", e(h.miniatura))
}

func (h *Handlers) render(w http.ResponseWriter, status int, pagina string, dados any) {
	if err := h.tpl.Renderizar(w, status, pagina, dados); err != nil {
		h.log.Error("renderizando página", "pagina", pagina, "erro", err)
	}
}

func (h *Handlers) falhaInterna(w http.ResponseWriter, err error) {
	h.log.Error("erro interno no catálogo", "erro", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

func base(r *http.Request) web.Base {
	return admin.Base(r, avisos[r.URL.Query().Get("aviso")])
}

func idDaRota(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func mensagem(err error) string {
	r := []rune(err.Error())
	if len(r) == 0 {
		return ""
	}
	return strings.ToUpper(string(r[0])) + string(r[1:]) + "."
}

// resultado trata o erro de uma ação: validação volta para a tela (via
// mostrarErro), registro inexistente vira 404, o resto vira erro interno.
func (h *Handlers) resultado(w http.ResponseWriter, r *http.Request, err error, destino string, mostrarErro func(string)) {
	switch {
	case err == nil:
		http.Redirect(w, r, destino, http.StatusSeeOther)
	case errors.Is(err, ErrNaoEncontrado):
		http.NotFound(w, r)
	case ErroDeValidacao(err):
		mostrarErro(mensagem(err))
	default:
		h.falhaInterna(w, err)
	}
}

// --- livros ----------------------------------------------------------------

type dadosLivros struct {
	web.Base
	Livros []Livro
}

func (h *Handlers) listarLivros(w http.ResponseWriter, r *http.Request) {
	livros, err := h.svc.Listar(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	h.render(w, http.StatusOK, "livros", dadosLivros{Base: base(r), Livros: livros})
}

type dadosFormLivro struct {
	web.Base
	Livro      Livro
	Faixas     []Faixa
	Categorias []Categoria
	TamanhoMB  int
}

func (h *Handlers) formLivro(w http.ResponseWriter, r *http.Request, status int, l Livro, erro string) {
	faixas, err := h.svc.ListarFaixas(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	cats, err := h.svc.ListarCategorias(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	d := dadosFormLivro{Base: base(r), Livro: l, Faixas: faixas, Categorias: cats, TamanhoMB: TamanhoMaximoCapa >> 20}
	d.Erro = erro
	h.render(w, status, "livro_form", d)
}

func lerDadosLivro(r *http.Request) (DadosLivro, error) {
	num := func(campo string) int64 {
		v, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue(campo)), 10, 64)
		return v
	}
	d := DadosLivro{
		Titulo:      r.PostFormValue("titulo"),
		Autor:       r.PostFormValue("autor"),
		Descricao:   r.PostFormValue("descricao"),
		ISBN:        r.PostFormValue("isbn"),
		Editora:     r.PostFormValue("editora"),
		CategoriaID: num("categoria_id"),
		FaixaID:     num("faixa_id"),
	}
	if ano := strings.TrimSpace(r.PostFormValue("ano")); ano != "" {
		v, err := strconv.Atoi(ano)
		if err != nil {
			return d, ErrAnoInvalido
		}
		d.Ano = v
	}
	return d, nil
}

func (h *Handlers) telaNovoLivro(w http.ResponseWriter, r *http.Request) {
	h.formLivro(w, r, http.StatusOK, Livro{}, "")
}

func (h *Handlers) cadastrarLivro(w http.ResponseWriter, r *http.Request) {
	d, err := lerDadosLivro(r)
	var id int64
	if err == nil {
		id, err = h.svc.Cadastrar(r.Context(), d)
	}
	h.resultado(w, r, err, "/admin/livros/"+strconv.FormatInt(id, 10)+"?aviso=livro-cadastrado", func(msg string) {
		h.formLivro(w, r, http.StatusUnprocessableEntity, Livro{DadosLivro: d}, msg)
	})
}

func (h *Handlers) telaEditarLivro(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	l, err := h.svc.Buscar(r.Context(), id)
	if errors.Is(err, ErrNaoEncontrado) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	h.formLivro(w, r, http.StatusOK, l, "")
}

// voltarParaLivro mostra a tela do livro com um erro de validação.
func (h *Handlers) voltarParaLivro(w http.ResponseWriter, r *http.Request, id int64, d *DadosLivro, msg string) {
	l, err := h.svc.Buscar(r.Context(), id)
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	if d != nil {
		l.DadosLivro = *d
	}
	h.formLivro(w, r, http.StatusUnprocessableEntity, l, msg)
}

func (h *Handlers) salvarLivro(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	d, err := lerDadosLivro(r)
	if err == nil {
		err = h.svc.Atualizar(r.Context(), id, d)
	}
	h.resultado(w, r, err, "/admin/livros/"+strconv.FormatInt(id, 10)+"?aviso=livro-salvo", func(msg string) {
		h.voltarParaLivro(w, r, id, &d, msg)
	})
}

func (h *Handlers) acaoLivro(w http.ResponseWriter, r *http.Request, aviso string, f func(int64) error) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.resultado(w, r, f(id), "/admin/livros/"+strconv.FormatInt(id, 10)+"?aviso="+aviso, func(msg string) {
		h.voltarParaLivro(w, r, id, nil, msg)
	})
}

func (h *Handlers) desativarLivro(w http.ResponseWriter, r *http.Request) {
	h.acaoLivro(w, r, "livro-desativado", func(id int64) error { return h.svc.Desativar(r.Context(), id) })
}

func (h *Handlers) reativarLivro(w http.ResponseWriter, r *http.Request) {
	h.acaoLivro(w, r, "livro-reativado", func(id int64) error { return h.svc.Reativar(r.Context(), id) })
}

func (h *Handlers) enviarCapa(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, TamanhoMaximoCapa+64<<10)
	h.acaoLivro(w, r, "capa-salva", func(id int64) error {
		arq, _, err := r.FormFile("capa")
		if err != nil {
			var grande *http.MaxBytesError
			if errors.As(err, &grande) {
				return ErrCapaTamanho
			}
			return ErrCapaTipo
		}
		defer arq.Close()
		dados, err := io.ReadAll(io.LimitReader(arq, TamanhoMaximoCapa+1))
		if err != nil {
			return err
		}
		return h.svc.DefinirCapa(r.Context(), id, dados)
	})
}

func (h *Handlers) removerCapa(w http.ResponseWriter, r *http.Request) {
	h.acaoLivro(w, r, "capa-removida", func(id int64) error { return h.svc.RemoverCapa(r.Context(), id) })
}

// capa serve a imagem original enviada pelo admin.
func (h *Handlers) capa(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	tipo, dados, hash, err := h.svc.Capa(r.Context(), id)
	h.imagem(w, r, tipo, dados, hash, err)
}

// miniatura mostra ao admin como a capa vai no pacote do catálogo.
func (h *Handlers) miniatura(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	dados, hash, err := h.svc.Miniatura(r.Context(), id)
	h.imagem(w, r, "image/jpeg", dados, hash, err)
}

func (h *Handlers) imagem(w http.ResponseWriter, r *http.Request, tipo string, dados, hash []byte, err error) {
	if errors.Is(err, ErrNaoEncontrado) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	etag := `"` + hex.EncodeToString(hash) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
	w.Write(dados)
}

// --- faixas de preço -------------------------------------------------------

type dadosFaixas struct {
	web.Base
	Faixas []Faixa
	Valor  string
}

func (h *Handlers) telaFaixas(w http.ResponseWriter, r *http.Request, status int, valor, erro string) {
	faixas, err := h.svc.ListarFaixas(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	d := dadosFaixas{Base: base(r), Faixas: faixas, Valor: valor}
	d.Erro = erro
	h.render(w, status, "precos", d)
}

func (h *Handlers) listarFaixas(w http.ResponseWriter, r *http.Request) {
	h.telaFaixas(w, r, http.StatusOK, "", "")
}

func (h *Handlers) criarFaixa(w http.ResponseWriter, r *http.Request) {
	valor := r.PostFormValue("valor")
	c, err := LerPreco(valor)
	if err == nil {
		_, err = h.svc.CriarFaixa(r.Context(), c)
	}
	h.resultado(w, r, err, "/admin/precos?aviso=faixa-criada", func(msg string) {
		h.telaFaixas(w, r, http.StatusUnprocessableEntity, valor, msg)
	})
}

func (h *Handlers) acaoFaixa(w http.ResponseWriter, r *http.Request, aviso string, f func(int64) error) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.resultado(w, r, f(id), "/admin/precos?aviso="+aviso, func(msg string) {
		h.telaFaixas(w, r, http.StatusUnprocessableEntity, "", msg)
	})
}

func (h *Handlers) desativarFaixa(w http.ResponseWriter, r *http.Request) {
	h.acaoFaixa(w, r, "faixa-desativada", func(id int64) error { return h.svc.DesativarFaixa(r.Context(), id) })
}

func (h *Handlers) reativarFaixa(w http.ResponseWriter, r *http.Request) {
	h.acaoFaixa(w, r, "faixa-reativada", func(id int64) error { return h.svc.ReativarFaixa(r.Context(), id) })
}

// --- categorias ------------------------------------------------------------

type dadosCategorias struct {
	web.Base
	Categorias []Categoria
	Nome       string
}

func (h *Handlers) telaCategorias(w http.ResponseWriter, r *http.Request, status int, nome, erro string) {
	cats, err := h.svc.ListarCategorias(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	d := dadosCategorias{Base: base(r), Categorias: cats, Nome: nome}
	d.Erro = erro
	h.render(w, status, "categorias", d)
}

func (h *Handlers) listarCategorias(w http.ResponseWriter, r *http.Request) {
	h.telaCategorias(w, r, http.StatusOK, "", "")
}

func (h *Handlers) criarCategoria(w http.ResponseWriter, r *http.Request) {
	nome := r.PostFormValue("nome")
	_, err := h.svc.CriarCategoria(r.Context(), nome)
	h.resultado(w, r, err, "/admin/categorias?aviso=categoria-criada", func(msg string) {
		h.telaCategorias(w, r, http.StatusUnprocessableEntity, nome, msg)
	})
}

func (h *Handlers) renomearCategoria(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.svc.RenomearCategoria(r.Context(), id, r.PostFormValue("nome"))
	h.resultado(w, r, err, "/admin/categorias?aviso=categoria-renomeada", func(msg string) {
		h.telaCategorias(w, r, http.StatusUnprocessableEntity, "", msg)
	})
}

func (h *Handlers) removerCategoria(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.svc.RemoverCategoria(r.Context(), id)
	h.resultado(w, r, err, "/admin/categorias?aviso=categoria-removida", func(msg string) {
		h.telaCategorias(w, r, http.StatusUnprocessableEntity, "", msg)
	})
}
