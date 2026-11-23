package compra

import (
	"bufio"
	"encoding/base64"
	"net"
	"strings"
	"testing"
)

// servidorSMTP é um servidor SMTP mínimo, só para teste: aceita uma mensagem
// sem TLS nem autenticação e devolve o envelope e o conteúdo recebidos.
func servidorSMTP(t *testing.T) (string, chan [3]string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	ch := make(chan [3]string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { c.Write([]byte(s + "\r\n")) }
		w("220 teste")
		var de, para string
		var dados strings.Builder
		for {
			linha, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(linha))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 ok")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				de = strings.TrimSpace(linha)[len("MAIL FROM:"):]
				w("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				para = strings.TrimSpace(linha)[len("RCPT TO:"):]
				w("250 ok")
			case cmd == "DATA":
				w("354 envie")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					dados.WriteString(l)
				}
				w("250 ok")
				ch <- [3]string{de, para, dados.String()}
			case cmd == "QUIT":
				w("221 tchau")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().String(), ch
}

// RF14 (envio real): o remetente "Nome <endereço>" vira só o endereço no
// envelope; a mensagem tem Date e Message-ID e leva a resposta do OT anexada.
func TestSMTP(t *testing.T) {
	end, ch := servidorSMTP(t)
	host, porta, _ := net.SplitHostPort(end)
	anexo := []byte(`{"formato":1}`)
	err := SMTP{Host: host, Porta: porta, De: "Livraria OT <loja@exemplo.com>"}.Enviar(Mensagem{
		Para: "cliente@exemplo.com", Assunto: "Sua compra", Texto: "olá", Anexo: anexo,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r[0] != "<loja@exemplo.com>" || r[1] != "<cliente@exemplo.com>" {
		t.Fatalf("envelope: %q %q", r[0], r[1])
	}
	for _, cab := range []string{"Date: ", "Message-ID: <", "@exemplo.com>", "From: Livraria OT <loja@exemplo.com>", "To: cliente@exemplo.com", "resposta-ot.json"} {
		if !strings.Contains(r[2], cab) {
			t.Errorf("mensagem sem %q", cab)
		}
	}
	if !strings.Contains(r[2], base64.StdEncoding.EncodeToString(anexo)) {
		t.Error("anexo ausente")
	}
}
