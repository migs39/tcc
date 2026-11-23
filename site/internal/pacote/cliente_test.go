package pacote

import (
	"net/http/httptest"
	"strings"
	"testing"

	"tcc/site/web"
)

// D-004: o programa local não é servido pelo site; a página só aponta para um
// endereço https público e mostra o SHA-256. Endereço que não é https é
// descartado.
func TestNovoCliente(t *testing.T) {
	h := strings.Repeat("Ab", 32)
	if c := NovoCliente("https://github.com/x/y/releases/latest/download/a.zip", h); c.SHA256 != strings.ToLower(h) || c.URL == "" {
		t.Fatalf("válido recusado: %+v", c)
	}
	for _, u := range []string{"http://exemplo.com/a.zip", "javascript:alert(1)", "/cliente.zip", ""} {
		if c := NovoCliente(u, h); c.URL != "" {
			t.Errorf("%q aceito", u)
		}
	}
	if c := NovoCliente("https://exemplo.com/a.zip", "não é hash"); c.SHA256 != "" {
		t.Error("hash inválido aceito")
	}
}

func TestPaginaComDownload(t *testing.T) {
	tpl, err := web.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	render := func(c Cliente) string {
		rec := httptest.NewRecorder()
		if err := tpl.Renderizar(rec, 200, "inicio", dadosInicio{Cliente: c}); err != nil {
			t.Fatal(err)
		}
		return rec.Body.String()
	}
	sha := strings.Repeat("0f", 32)
	p := render(NovoCliente("https://github.com/migs39/tcc/releases/latest/download/LivrariaOT-cliente.zip", sha))
	if !strings.Contains(p, `href="https://github.com/migs39/tcc/releases/latest/download/LivrariaOT-cliente.zip"`) ||
		!strings.Contains(p, sha) || strings.Contains(p, "lp-desativado") {
		t.Fatal("página não mostra o link do Release e o hash")
	}
	if p := render(Cliente{}); !strings.Contains(p, "lp-desativado") || strings.Contains(p, "Baixar para Windows") {
		t.Fatal("sem configuração, o download deveria aparecer indisponível")
	}
}
