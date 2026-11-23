//go:build windows

// Package janela é a interface gráfica do programa local (D-014): uma janela
// nativa com WebView2 que mostra uma página local (embutida no executável).
// A página não acessa a rede: tudo passa pelas funções Go abaixo, que usam o
// mesmo cliente da linha de comando e falam só com o site e com a CDN
// (D-004). Não há porta localhost nem código vindo do site (D-001).
package janela

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"

	"tcc/programa-local/internal/cliente"
)

// Abrir mostra a janela e bloqueia até ela ser fechada.
func Abrir(c *cliente.Cliente) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(c.Pasta, "webview"),
		WindowOptions: webview2.WindowOptions{
			Title: "Livraria OT", Width: 1320, Height: 860, Center: true,
		},
	})
	if w == nil {
		return errWebView
	}
	defer w.Destroy()
	w.SetSize(1100, 720, webview2.HintMin)

	api := &API{c: c}
	var proximo atomic.Int64
	// O WebView2 chama as funções ligadas na thread da interface; trabalho de
	// rede ali congelaria a janela. Cada chamada roda numa goroutine e a
	// resposta volta por Eval na thread da interface.
	w.Bind("_chamar", func(metodo string, args json.RawMessage) int64 {
		id := proximo.Add(1)
		go func() {
			res, err := api.Despachar(context.Background(), metodo, args)
			var js string
			if err != nil {
				msg, _ := json.Marshal(err.Error())
				js = "window._resposta(" + itoa(id) + ", null, " + string(msg) + ")"
			} else {
				b, _ := json.Marshal(res)
				js = "window._resposta(" + itoa(id) + ", " + string(b) + ", null)"
			}
			w.Dispatch(func() { w.Eval(js) })
		}()
		return id
	})
	w.SetHtml(Pagina())
	w.Run()
	return nil
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// abrirNoSistema abre um arquivo ou pasta com o programa padrão do Windows.
func abrirNoSistema(caminho string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", caminho).Start()
}
