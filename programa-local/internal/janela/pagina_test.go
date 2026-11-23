package janela

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// D-004/D-014: a página da janela não carrega nada de fora e traz o tema com
// as fontes embutidas.
func TestPaginaSemRecursosExternos(t *testing.T) {
	p := Pagina()
	if strings.Contains(p, "/*TEMA*/") || !strings.Contains(p, "data:font/woff2;base64,") {
		t.Fatal("tema não embutido")
	}
	if regexp.MustCompile(`(src|href)=["']?https?:`).MatchString(p) || strings.Contains(p, `url("http`) || strings.Contains(p, `url("fontes/`) {
		t.Fatal("a página referencia recurso externo")
	}
	if !strings.Contains(p, "connect-src 'none'") {
		t.Fatal("CSP da página não bloqueia a rede")
	}
}

func TestDespacharValida(t *testing.T) {
	a := &API{}
	if _, err := a.Despachar(context.Background(), "naoexiste", nil); err == nil {
		t.Fatal("método desconhecido aceito")
	}
	if _, err := a.Despachar(context.Background(), "baixar", []byte(`[]`)); err == nil {
		t.Fatal("argumento faltando aceito")
	}
	if _, err := a.Despachar(context.Background(), "comprar", []byte(`{`)); err == nil {
		t.Fatal("JSON inválido aceito")
	}
}
