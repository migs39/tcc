// Package tema guarda o tema visual comum ao site, à CDN e ao programa local
// (estilo "Cofre criptográfico"): tema.css e as fontes, servidos pelo próprio
// componente, sem recursos de terceiros (D-003).
package tema

import (
	"embed"
	"io/fs"
)

//go:embed tema.css fontes/*.woff2 fontes/*.txt
var arquivos embed.FS

// FS é servido em /tema/ (tema.css e fontes/).
var FS fs.FS = arquivos

// CSS devolve o conteúdo de tema.css.
func CSS() []byte {
	b, _ := arquivos.ReadFile("tema.css")
	return b
}

// Fonte devolve uma fonte embutida (para quem precisa inlinar, como o
// programa local).
func Fonte(nome string) []byte {
	b, _ := arquivos.ReadFile("fontes/" + nome)
	return b
}

// Icone é o símbolo da marca (cadeado), em SVG, na cor dada.
func Icone(cor string) string {
	return `<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="` + cor +
		`" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 11h12v10H6zM8 11V7a4 4 0 0 1 8 0v4"/></svg>`
}
