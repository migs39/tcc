// Package web embute templates e arquivos estáticos do site e renderiza as
// páginas. Nenhum recurso de terceiros (scripts, fontes, analytics): D-003.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed templates static
var arquivos embed.FS

// Estaticos devolve os arquivos servidos em /static/.
func Estaticos() fs.FS {
	sub, err := fs.Sub(arquivos, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// Base traz os campos que o layout usa em toda página. Os dados de cada
// página embutem Base.
type Base struct {
	Admin any // administrador logado, ou nil na área pública
	Aviso string
	Erro  string
}

// Templates guarda uma árvore por página, cada uma com o layout comum.
type Templates struct {
	paginas map[string]*template.Template
}

// FormatarCentavos escreve 2990 como "R$ 29,90".
func FormatarCentavos(c int) string {
	return fmt.Sprintf("R$ %d,%02d", c/100, c%100)
}

var funcoes = template.FuncMap{
	"preco": FormatarCentavos,
	// centavos mostra o valor sem "R$" para preencher campos de formulário.
	"centavos": func(c int) string { return fmt.Sprintf("%d,%02d", c/100, c%100) },
}

// Carregar monta cada página com o layout e os parciais compartilhados.
func Carregar() (*Templates, error) {
	nomes, err := fs.Glob(arquivos, "templates/paginas/*.html")
	if err != nil {
		return nil, err
	}
	t := &Templates{paginas: map[string]*template.Template{}}
	for _, arq := range nomes {
		nome := strings.TrimSuffix(path.Base(arq), ".html")
		tpl, err := template.New(path.Base(arq)).Funcs(funcoes).
			ParseFS(arquivos, "templates/layout.html", "templates/parciais/*.html", arq)
		if err != nil {
			return nil, err
		}
		t.paginas[nome] = tpl
	}
	return t, nil
}

// Renderizar monta a página inteira antes de escrever, para não enviar HTML
// pela metade se o template falhar.
func (t *Templates) Renderizar(w http.ResponseWriter, status int, pagina string, dados any) error {
	tpl, ok := t.paginas[pagina]
	if !ok {
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return fs.ErrNotExist
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", dados); err != nil {
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}
