package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"tcc/site/internal/admin"
	"tcc/site/internal/catalogo"
	"tcc/site/internal/db/dbteste"
	"tcc/site/internal/pacote"
	"tcc/site/web"
)

func novoServidor(t *testing.T) (*httptest.Server, *admin.Servico) {
	srv, svc, _ := novoServidorCompleto(t)
	return srv, svc
}

func novoServidorCompleto(t *testing.T) (*httptest.Server, *admin.Servico, *catalogo.Servico) {
	t.Helper()
	pool := dbteste.Novo(t)
	svc, err := admin.NovoServico(pool)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := web.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hAdmin := admin.NovosHandlers(svc, tpl, log)
	cat := catalogo.NovoServico(pool)
	_, chavePacote, _ := ed25519.GenerateKey(nil)
	srv := httptest.NewTLSServer(NovoRoteador(tpl, log, hAdmin, catalogo.NovosHandlers(cat, hAdmin, tpl, log),
		pacote.NovosHandlers(pacote.NovoServico(pool, chavePacote), tpl, log)))
	chavePublicaTeste = chavePacote.Public().(ed25519.PublicKey)
	t.Cleanup(srv.Close)
	return srv, svc, cat
}

func cliente(t *testing.T, srv *httptest.Server) *http.Client {
	jar, _ := cookiejar.New(nil)
	// Cópia: srv.Client() devolve sempre o mesmo objeto, e cada cliente precisa
	// do próprio pote de cookies.
	copia := *srv.Client()
	c := &copia
	c.Jar = jar
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// D-003: nenhuma página carrega recursos de terceiros.
func TestCabecalhosDeSeguranca(t *testing.T) {
	tpl, err := web.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := httptest.NewRecorder()
	NovoRoteador(tpl, log, admin.NovosHandlers(nil, tpl, log)).
		ServeHTTP(rec, httptest.NewRequest("GET", "/admin/entrar", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/entrar: status %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "http") {
		t.Fatalf("CSP permite origem externa: %q", csp)
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("Referrer-Policy ausente")
	}
	corpo := rec.Body.String()
	for _, externo := range []string{"http://", "https://", "//cdn", "googleapis"} {
		if strings.Contains(corpo, externo) {
			t.Fatalf("tela de login referencia recurso externo (%q)", externo)
		}
	}
}

// RF01: só administradores autenticados acessam a área administrativa.
func TestAreaAdminExigeLogin(t *testing.T) {
	srv, _ := novoServidor(t)
	c := cliente(t, srv)
	for _, caminho := range []string{"/admin/", "/admin/administradores/novo", "/admin/administradores/1"} {
		resp, err := c.Get(srv.URL + caminho)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/admin/entrar" {
			t.Errorf("GET %s sem sessão: %d %q", caminho, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	resp, _ := c.PostForm(srv.URL+"/admin/administradores",
		url.Values{"nome": {"X"}, "email": {"x@exemplo.com"}, "senha": {"senha-de-teste-123"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST sem sessão: status %d", resp.StatusCode)
	}
}

func TestFluxoLoginCadastroSair(t *testing.T) {
	srv, svc := novoServidor(t)
	if _, err := svc.CadastrarPrimeiro(context.Background(), "Ana", "ana@exemplo.com", "senha-de-teste-123"); err != nil {
		t.Fatal(err)
	}
	c := cliente(t, srv)

	resp, _ := c.PostForm(srv.URL+"/admin/entrar", url.Values{"email": {"ana@exemplo.com"}, "senha": {"errada-errada-1"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login com senha errada: status %d", resp.StatusCode)
	}

	resp, _ = c.PostForm(srv.URL+"/admin/entrar", url.Values{"email": {"ana@exemplo.com"}, "senha": {"senha-de-teste-123"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login válido: status %d", resp.StatusCode)
	}
	var sessao *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "sessao_admin" {
			sessao = ck
		}
	}
	if sessao == nil || !sessao.HttpOnly || !sessao.Secure || sessao.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie de sessão sem HttpOnly/Secure/SameSite=Strict: %+v", sessao)
	}

	resp, _ = c.PostForm(srv.URL+"/admin/administradores",
		url.Values{"nome": {"Bia"}, "email": {"bia@exemplo.com"}, "senha": {"senha-da-bia-123"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("cadastro por admin logado: status %d", resp.StatusCode)
	}

	resp, _ = c.Get(srv.URL + "/admin/")
	corpo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(corpo), "bia@exemplo.com") {
		t.Fatalf("lista não mostra o novo admin (status %d)", resp.StatusCode)
	}

	resp, _ = c.PostForm(srv.URL+"/admin/sair", nil)
	resp.Body.Close()
	resp, _ = c.Get(srv.URL + "/admin/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("após sair, área admin ainda acessível: status %d", resp.StatusCode)
	}
}

// CSRF: POST vindo de outra origem é recusado mesmo com sessão válida.
func TestRecusaPostDeOutraOrigem(t *testing.T) {
	srv, svc := novoServidor(t)
	svc.CadastrarPrimeiro(context.Background(), "Ana", "ana@exemplo.com", "senha-de-teste-123")
	c := cliente(t, srv)
	resp, _ := c.PostForm(srv.URL+"/admin/entrar", url.Values{"email": {"ana@exemplo.com"}, "senha": {"senha-de-teste-123"}})
	resp.Body.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/admin/administradores",
		strings.NewReader(url.Values{"nome": {"Eva"}, "email": {"eva@exemplo.com"}, "senha": {"senha-da-eva-123"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST cross-site: esperado 403, veio %d", resp.StatusCode)
	}
}

// entrarComoAdmin cria o primeiro admin e devolve um cliente logado.
func entrarComoAdmin(t *testing.T, srv *httptest.Server, svc *admin.Servico) *http.Client {
	t.Helper()
	if _, err := svc.CadastrarPrimeiro(context.Background(), "Ana", "ana@exemplo.com", "senha-de-teste-123"); err != nil {
		t.Fatal(err)
	}
	c := cliente(t, srv)
	resp, err := c.PostForm(srv.URL+"/admin/entrar", url.Values{"email": {"ana@exemplo.com"}, "senha": {"senha-de-teste-123"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status %d", resp.StatusCode)
	}
	return c
}

// RF01: as telas do catálogo também exigem admin autenticado.
func TestCatalogoAdminExigeLogin(t *testing.T) {
	srv, _ := novoServidor(t)
	c := cliente(t, srv)
	for _, caminho := range []string{"/admin/livros", "/admin/livros/novo", "/admin/precos", "/admin/categorias"} {
		resp, err := c.Get(srv.URL + caminho)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s sem sessão: status %d", caminho, resp.StatusCode)
		}
	}
	resp, _ := c.PostForm(srv.URL+"/admin/precos", url.Values{"valor": {"9,90"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/admin/entrar" {
		t.Fatalf("POST /admin/precos sem sessão: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// RF02 + RF03 pela interface: faixa, categoria, livro e capa.
func TestFluxoCatalogo(t *testing.T) {
	srv, svcAdmin, cat := novoServidorCompleto(t)
	c := entrarComoAdmin(t, srv, svcAdmin)
	ctx := context.Background()

	post := func(caminho string, v url.Values) *http.Response {
		t.Helper()
		resp, err := c.PostForm(srv.URL+caminho, v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if r := post("/admin/precos", url.Values{"valor": {"29,90"}}); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("criar faixa: %d", r.StatusCode)
	}
	if r := post("/admin/precos", url.Values{"valor": {"abc"}}); r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("faixa inválida: %d", r.StatusCode)
	}
	if r := post("/admin/categorias", url.Values{"nome": {"Ficção"}}); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("criar categoria: %d", r.StatusCode)
	}
	faixas, _ := cat.ListarFaixas(ctx)
	cats, _ := cat.ListarCategorias(ctx)
	if len(faixas) != 1 || faixas[0].ValorCentavos != 2990 || len(cats) != 1 {
		t.Fatalf("faixas %+v, categorias %+v", faixas, cats)
	}

	livro := url.Values{
		"titulo": {"Dom Casmurro"}, "autor": {"Machado de Assis"}, "ano": {"1899"},
		"categoria_id": {strconv.FormatInt(cats[0].ID, 10)}, "faixa_id": {strconv.FormatInt(faixas[0].ID, 10)},
	}
	r := post("/admin/livros", livro)
	if r.StatusCode != http.StatusSeeOther || !strings.HasPrefix(r.Header.Get("Location"), "/admin/livros/") {
		t.Fatalf("cadastrar livro: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	livros, _ := cat.Listar(ctx)
	if len(livros) != 1 || livros[0].Titulo != "Dom Casmurro" {
		t.Fatalf("livros: %+v", livros)
	}
	id := strconv.FormatInt(livros[0].ID, 10)

	// RF03 pela interface: faixa forjada no formulário é recusada.
	forjado := url.Values{}
	for k, v := range livro {
		forjado[k] = v
	}
	forjado.Set("faixa_id", "999")
	if r := post("/admin/livros", forjado); r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("faixa forjada: %d", r.StatusCode)
	}

	// Capa: envio multipart e leitura pela rota pública.
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	fw, _ := mw.CreateFormFile("capa", "capa.png")
	fw.Write(img.Bytes())
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/admin/livros/"+id+"/capa", &corpo)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("enviar capa: %d", resp.StatusCode)
	}

	get := func(cl *http.Client, caminho string) (*http.Response, []byte) {
		t.Helper()
		resp, err := cl.Get(srv.URL + caminho)
		if err != nil {
			t.Fatal(err)
		}
		dados, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, dados
	}

	// O admin vê a original e a miniatura de tamanho fixo (D-006).
	resp, dados := get(c, "/admin/livros/"+id+"/capa")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || !bytes.Equal(dados, img.Bytes()) {
		t.Fatalf("capa original: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	req, _ = http.NewRequest("GET", srv.URL+"/admin/livros/"+id+"/capa", nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	resp304, _ := c.Do(req)
	resp304.Body.Close()
	if resp304.StatusCode != http.StatusNotModified {
		t.Fatalf("capa com ETag igual: esperado 304, veio %d", resp304.StatusCode)
	}
	resp, dados = get(c, "/admin/livros/"+id+"/miniatura")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("miniatura: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(dados)); err != nil || cfg.Width != 200 || cfg.Height != 300 {
		t.Fatalf("miniatura com tamanho errado: %+v %v", cfg, err)
	}

	// L1 / D-006: não há capa avulsa para o público.
	anonimo := cliente(t, srv)
	for _, caminho := range []string{"/capas/" + id, "/admin/livros/" + id + "/capa", "/admin/livros/" + id + "/miniatura"} {
		if resp, _ := get(anonimo, caminho); resp.StatusCode == http.StatusOK {
			t.Errorf("GET %s sem sessão devolveu a imagem", caminho)
		}
	}

	// Remover do catálogo: a miniatura (que vai no pacote) deixa de existir.
	if r := post("/admin/livros/"+id+"/desativar", nil); r.StatusCode != http.StatusSeeOther || !strings.Contains(r.Header.Get("Location"), "livro-desativado") {
		t.Fatalf("desativar livro: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	if resp, _ := get(c, "/admin/livros/"+id+"/miniatura"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("miniatura de livro removido: %d", resp.StatusCode)
	}

	// As telas renderizam sem erro.
	for _, caminho := range []string{"/admin/livros", "/admin/livros/novo", "/admin/livros/" + id, "/admin/precos", "/admin/categorias"} {
		resp, err := c.Get(srv.URL + caminho)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: %d", caminho, resp.StatusCode)
		}
	}
}

// chavePublicaTeste é a chave do pacote do último servidor de teste criado.
var chavePublicaTeste ed25519.PublicKey

// D-006 / L1: o pacote é público, igual para todos e assinado; com o ETag
// certo, nada é baixado de novo.
func TestPacoteDoCatalogo(t *testing.T) {
	srv, _, cat := novoServidorCompleto(t)
	ctx := context.Background()
	c, _ := cat.CriarCategoria(ctx, "Ficção")
	f, _ := cat.CriarFaixa(ctx, 2990)
	if _, err := cat.Cadastrar(ctx, catalogo.DadosLivro{Titulo: "Dom Casmurro", Autor: "Machado de Assis", CategoriaID: c.ID, FaixaID: f.ID}); err != nil {
		t.Fatal(err)
	}

	anonimo := cliente(t, srv)
	resp, err := anonimo.Get(srv.URL + "/catalogo.zip")
	if err != nil {
		t.Fatal(err)
	}
	dados, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("GET /catalogo.zip: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	// O programa local confere: hash do corpo, versão e assinatura.
	versao, _ := strconv.ParseInt(resp.Header.Get("Catalogo-Versao"), 10, 64)
	hash := sha256.Sum256(dados)
	assinatura, _ := base64.StdEncoding.DecodeString(resp.Header.Get("Catalogo-Assinatura"))
	if resp.Header.Get("Catalogo-SHA256") != hex.EncodeToString(hash[:]) ||
		!pacote.Verificar(chavePublicaTeste, versao, hash, assinatura) {
		t.Fatal("cabeçalhos do pacote não conferem com o corpo")
	}
	if !bytes.Contains(dados, []byte("catalogo.json")) {
		t.Fatal("ZIP sem catalogo.json")
	}

	req, _ := http.NewRequest("GET", srv.URL+"/catalogo.zip", nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	resp, _ = anonimo.Do(req)
	corpo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified || len(corpo) != 0 {
		t.Fatalf("revalidação com ETag igual: %d, %d bytes", resp.StatusCode, len(corpo))
	}

	// Hash público: igual ao do pacote baixado.
	resp, _ = anonimo.Get(srv.URL + "/catalogo/hash")
	var h pacote.RespostaHash
	json.NewDecoder(resp.Body).Decode(&h)
	resp.Body.Close()
	if h.Versao != versao || h.SHA256 != hex.EncodeToString(hash[:]) || h.Tamanho != len(dados) {
		t.Fatalf("/catalogo/hash diverge do pacote: %+v", h)
	}

	// Outro visitante recebe exatamente os mesmos bytes.
	resp, _ = cliente(t, srv).Get(srv.URL + "/catalogo.zip")
	outro, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(outro, dados) {
		t.Fatal("dois visitantes receberam pacotes diferentes")
	}
}

// Página pública de download: renderiza com os dados reais do pacote e não
// referencia nenhum recurso externo (D-003, D-006).
func TestPaginaInicial(t *testing.T) {
	srv, _, cat := novoServidorCompleto(t)
	ctx := context.Background()
	c, _ := cat.CriarCategoria(ctx, "Ficção")
	f, _ := cat.CriarFaixa(ctx, 2990)
	cat.Cadastrar(ctx, catalogo.DadosLivro{Titulo: "Dom Casmurro", Autor: "Machado de Assis", CategoriaID: c.ID, FaixaID: f.ID})

	anonimo := cliente(t, srv)
	resp, err := anonimo.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	corpo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: %d", resp.StatusCode)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatal("página pública definiu cookie")
	}
	resp, _ = anonimo.Get(srv.URL + "/catalogo/hash")
	var h pacote.RespostaHash
	json.NewDecoder(resp.Body).Decode(&h)
	resp.Body.Close()
	pagina := string(corpo)
	if !strings.Contains(pagina, h.SHA256) || !strings.Contains(pagina, hex.EncodeToString(chavePublicaTeste)) {
		t.Fatal("página inicial não mostra o hash do pacote atual e a chave pública")
	}
	for _, externo := range []string{"http://", "https://", "//cdn", "googleapis", "<script"} {
		if strings.Contains(pagina, externo) {
			t.Fatalf("página inicial referencia recurso externo ou script (%q)", externo)
		}
	}
}
