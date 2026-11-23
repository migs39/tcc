package janela

import (
	_ "embed"
	"encoding/base64"
	"strings"

	"tcc/protocolo/tema"
)

//go:embed pagina.html
var pagina string

// Pagina monta a página da janela: o tema comum com as fontes embutidas como
// data URL (a página não carrega nada de fora) e a aplicação.
func Pagina() string {
	css := string(tema.CSS())
	for _, f := range []string{"JetBrainsMono.woff2", "SpaceGrotesk.woff2"} {
		css = strings.ReplaceAll(css, `url("fontes/`+f+`")`,
			`url("data:font/woff2;base64,`+base64.StdEncoding.EncodeToString(tema.Fonte(f))+`")`)
	}
	return strings.Replace(pagina, "/*TEMA*/", css, 1)
}
