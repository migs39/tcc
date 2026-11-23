package cliente

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// RF17: livro já baixado cujo arquivo sumiu da biblioteca é apontado como
// ausente (não há segundo download); pendente ou presente, não.
func TestAusentes(t *testing.T) {
	dir := t.TempDir()
	presente := filepath.Join(dir, "1 - A.txt")
	if err := os.WriteFile(presente, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	cp := &Compra{
		Titulos:  []string{"A", "B", "C"},
		Baixados: []string{presente, filepath.Join(dir, "2 - B.txt"), ""},
	}
	if got := ausentes(cp); !reflect.DeepEqual(got, []string{"B"}) {
		t.Fatalf("ausentes = %v, esperado [B]", got)
	}
}

// D-004: o programa fala só com o site e a CDN configurados; a variável de
// ambiente vence o endereço embutido, que vence o localhost.
func TestConfigurarEnderecos(t *testing.T) {
	chave := strings.Repeat("ab", 32)
	t.Setenv("LIVRARIA_DADOS", t.TempDir())
	t.Setenv("SITE_URL", "")
	t.Setenv("CDN_URL", "")
	c, err := Configurar(Embutidos{ChavePacote: chave})
	if err != nil || c.SiteURL != "http://localhost:8080" || c.CDNURL != "http://localhost:8081" {
		t.Fatalf("padrão: %v %q %q", err, c.SiteURL, c.CDNURL)
	}
	c, _ = Configurar(Embutidos{ChavePacote: chave, SiteURL: "https://loja.exemplo/", CDNURL: "https://loja.exemplo:8443"})
	if c.SiteURL != "https://loja.exemplo" || c.CDNURL != "https://loja.exemplo:8443" {
		t.Fatalf("embutido: %q %q", c.SiteURL, c.CDNURL)
	}
	t.Setenv("SITE_URL", "http://outro:9000")
	c, _ = Configurar(Embutidos{ChavePacote: chave, SiteURL: "https://loja.exemplo"})
	if c.SiteURL != "http://outro:9000" {
		t.Fatalf("variável não venceu: %q", c.SiteURL)
	}
}
