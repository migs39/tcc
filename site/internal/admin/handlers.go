package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"tcc/site/web"
)

const nomeCookie = "sessao_admin"

// Avisos exibidos após redirecionamento. A URL leva só a chave, nunca dados.
var avisos = map[string]string{
	"cadastrado": "Administrador cadastrado.",
	"salvo":      "Dados salvos.",
	"senha":      "Senha alterada. As sessões desse administrador foram encerradas.",
	"desativado": "Administrador desativado.",
	"reativado":  "Administrador reativado.",
}

type Handlers struct {
	svc *Servico
	tpl *web.Templates
	log *slog.Logger
}

func NovosHandlers(svc *Servico, tpl *web.Templates, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, tpl: tpl, log: log}
}

// Registrar monta as rotas da área administrativa (RF01). Tudo exige sessão,
// exceto a tela de login.
func (h *Handlers) Registrar(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/entrar", h.telaEntrar)
	mux.HandleFunc("POST /admin/entrar", h.entrar)
	mux.HandleFunc("POST /admin/sair", h.sair)

	mux.HandleFunc("GET /admin/{$}", h.exigir(h.listar))
	mux.HandleFunc("GET /admin/administradores/novo", h.exigir(h.telaNovo))
	mux.HandleFunc("POST /admin/administradores", h.exigir(h.cadastrar))
	mux.HandleFunc("GET /admin/administradores/{id}", h.exigir(h.telaEditar))
	mux.HandleFunc("POST /admin/administradores/{id}", h.exigir(h.salvar))
	mux.HandleFunc("POST /admin/administradores/{id}/senha", h.exigir(h.trocarSenha))
	mux.HandleFunc("POST /admin/administradores/{id}/desativar", h.exigir(h.desativar))
	mux.HandleFunc("POST /admin/administradores/{id}/reativar", h.exigir(h.reativar))
}

type chaveAdmin struct{}

// AdminLogado devolve o admin da requisição (colocado por exigir).
func AdminLogado(ctx context.Context) *Admin {
	a, _ := ctx.Value(chaveAdmin{}).(*Admin)
	return a
}

// exigir só deixa passar requisições com sessão válida de admin ativo.
func (h *Handlers) exigir(prox http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		c, err := r.Cookie(nomeCookie)
		if err != nil {
			http.Redirect(w, r, "/admin/entrar", http.StatusSeeOther)
			return
		}
		a, err := h.svc.AdminDaSessao(r.Context(), c.Value)
		if errors.Is(err, ErrSessaoInvalida) {
			apagarCookie(w)
			http.Redirect(w, r, "/admin/entrar", http.StatusSeeOther)
			return
		}
		if err != nil {
			h.falhaInterna(w, err)
			return
		}
		prox(w, r.WithContext(context.WithValue(r.Context(), chaveAdmin{}, &a)))
	}
}

func definirCookie(w http.ResponseWriter, token string, expira time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookie,
		Value:    token,
		Path:     "/admin",
		Expires:  expira,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func apagarCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: nomeCookie, Value: "", Path: "/admin", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handlers) falhaInterna(w http.ResponseWriter, err error) {
	h.log.Error("erro interno na área administrativa", "erro", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

func (h *Handlers) render(w http.ResponseWriter, status int, pagina string, dados any) {
	if err := h.tpl.Renderizar(w, status, pagina, dados); err != nil {
		h.log.Error("renderizando página", "pagina", pagina, "erro", err)
	}
}

func base(r *http.Request) web.Base {
	b := web.Base{Aviso: avisos[r.URL.Query().Get("aviso")]}
	if a := AdminLogado(r.Context()); a != nil {
		b.Admin = a
	}
	return b
}

// --- login -----------------------------------------------------------------

type dadosEntrar struct {
	web.Base
	Email string
}

func (h *Handlers) telaEntrar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h.render(w, http.StatusOK, "entrar", dadosEntrar{Base: base(r)})
}

func (h *Handlers) entrar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	email := r.PostFormValue("email")
	a, err := h.svc.Autenticar(r.Context(), email, r.PostFormValue("senha"))
	if errors.Is(err, ErrCredenciaisInvalidas) {
		d := dadosEntrar{Base: base(r), Email: email}
		d.Erro = "E-mail ou senha inválidos."
		h.render(w, http.StatusUnauthorized, "entrar", d)
		return
	}
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	token, expira, err := h.svc.CriarSessao(r.Context(), a.ID)
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	definirCookie(w, token, expira)
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

func (h *Handlers) sair(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(nomeCookie); err == nil {
		if err := h.svc.EncerrarSessao(r.Context(), c.Value); err != nil {
			h.falhaInterna(w, err)
			return
		}
	}
	apagarCookie(w)
	http.Redirect(w, r, "/admin/entrar", http.StatusSeeOther)
}

// --- gestão de administradores ---------------------------------------------

type dadosLista struct {
	web.Base
	Lista []Admin
}

func (h *Handlers) listar(w http.ResponseWriter, r *http.Request) {
	lista, err := h.svc.Listar(r.Context())
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	h.render(w, http.StatusOK, "administradores", dadosLista{Base: base(r), Lista: lista})
}

type dadosNovo struct {
	web.Base
	Nome, Email        string
	TamanhoMinimoSenha int
}

func (h *Handlers) telaNovo(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "admin_novo", dadosNovo{Base: base(r), TamanhoMinimoSenha: TamanhoMinimoSenha})
}

// errosDeValidacao são mostrados ao usuário; os demais viram "erro interno".
func ehErroDeValidacao(err error) bool {
	for _, e := range []error{ErrEmailEmUso, ErrEmailInvalido, ErrNomeVazio, ErrSenhaCurta, ErrUltimoAdminAtivo} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

func (h *Handlers) cadastrar(w http.ResponseWriter, r *http.Request) {
	nome, email := r.PostFormValue("nome"), r.PostFormValue("email")
	_, err := h.svc.Cadastrar(r.Context(), nome, email, r.PostFormValue("senha"))
	if ehErroDeValidacao(err) {
		d := dadosNovo{Base: base(r), Nome: nome, Email: email, TamanhoMinimoSenha: TamanhoMinimoSenha}
		d.Erro = mensagem(err)
		h.render(w, http.StatusUnprocessableEntity, "admin_novo", d)
		return
	}
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	http.Redirect(w, r, "/admin/?aviso=cadastrado", http.StatusSeeOther)
}

type dadosEditar struct {
	web.Base
	Alvo               Admin
	TamanhoMinimoSenha int
}

func idDaRota(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *Handlers) telaEditar(w http.ResponseWriter, r *http.Request) {
	h.editar(w, r, http.StatusOK, "")
}

// editar mostra a tela de edição, opcionalmente com uma mensagem de erro.
func (h *Handlers) editar(w http.ResponseWriter, r *http.Request, status int, erro string) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	alvo, err := h.svc.Buscar(r.Context(), id)
	if errors.Is(err, ErrNaoEncontrado) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.falhaInterna(w, err)
		return
	}
	d := dadosEditar{Base: base(r), Alvo: alvo, TamanhoMinimoSenha: TamanhoMinimoSenha}
	d.Erro = erro
	h.render(w, status, "admin_editar", d)
}

// acao executa uma alteração sobre o admin da rota e redireciona com aviso.
func (h *Handlers) acao(w http.ResponseWriter, r *http.Request, aviso string, f func(id int64) error) {
	id, ok := idDaRota(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := f(id)
	switch {
	case errors.Is(err, ErrNaoEncontrado):
		http.NotFound(w, r)
	case ehErroDeValidacao(err):
		h.editar(w, r, http.StatusUnprocessableEntity, mensagem(err))
	case err != nil:
		h.falhaInterna(w, err)
	default:
		http.Redirect(w, r, "/admin/administradores/"+strconv.FormatInt(id, 10)+"?aviso="+aviso, http.StatusSeeOther)
	}
}

func (h *Handlers) salvar(w http.ResponseWriter, r *http.Request) {
	h.acao(w, r, "salvo", func(id int64) error {
		return h.svc.Atualizar(r.Context(), id, r.PostFormValue("nome"), r.PostFormValue("email"))
	})
}

func (h *Handlers) trocarSenha(w http.ResponseWriter, r *http.Request) {
	h.acao(w, r, "senha", func(id int64) error {
		if err := h.svc.TrocarSenha(r.Context(), id, r.PostFormValue("senha")); err != nil {
			return err
		}
		if AdminLogado(r.Context()).ID == id {
			// A troca encerrou a própria sessão; o próximo acesso pede login.
			apagarCookie(w)
		}
		return nil
	})
}

func (h *Handlers) desativar(w http.ResponseWriter, r *http.Request) {
	h.acao(w, r, "desativado", func(id int64) error { return h.svc.Desativar(r.Context(), id) })
}

func (h *Handlers) reativar(w http.ResponseWriter, r *http.Request) {
	h.acao(w, r, "reativado", func(id int64) error { return h.svc.Reativar(r.Context(), id) })
}

// mensagem transforma o erro de validação em frase para a tela.
func mensagem(err error) string {
	s := err.Error()
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r) + "."
}
