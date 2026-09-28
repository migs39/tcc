package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tcc/site/internal/admin"
	"tcc/site/internal/db/dbteste"
	"tcc/site/web"
)

func novoServidor(t *testing.T) (*httptest.Server, *admin.Servico) {
	t.Helper()
	svc, err := admin.NovoServico(dbteste.Novo(t))
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := web.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewTLSServer(NovoRoteador(tpl, admin.NovosHandlers(svc, tpl, log), log))
	t.Cleanup(srv.Close)
	return srv, svc
}

func cliente(t *testing.T, srv *httptest.Server) *http.Client {
	jar, _ := cookiejar.New(nil)
	c := srv.Client()
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
	NovoRoteador(tpl, admin.NovosHandlers(nil, tpl, log), log).
		ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status %d", rec.Code)
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
			t.Fatalf("página inicial referencia recurso externo (%q)", externo)
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
