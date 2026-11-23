package compra

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mensagem é um e-mail a enviar: só a resposta do OT, nunca uma chave
// (invariante 2).
type Mensagem struct {
	Para    string
	Assunto string
	Texto   string
	Anexo   []byte // resposta do OT em JSON
}

// Enviador entrega e-mails.
type Enviador interface {
	Enviar(Mensagem) error
}

// CaixaDeSaida é um Enviador de desenvolvimento: grava cada e-mail como um
// arquivo .eml numa pasta (stub do RF14). Trocar por SMTP não muda o resto.
type CaixaDeSaida struct{ Pasta string }

func (c CaixaDeSaida) Enviar(m Mensagem) error {
	if err := os.MkdirAll(c.Pasta, 0o700); err != nil {
		return err
	}
	b := make([]byte, 8)
	rand.Read(b)
	limite := "limite-" + hex.EncodeToString(b)
	var s strings.Builder
	fmt.Fprintf(&s, "To: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", m.Para, m.Assunto)
	fmt.Fprintf(&s, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", limite)
	fmt.Fprintf(&s, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", limite, m.Texto)
	fmt.Fprintf(&s, "--%s\r\nContent-Type: application/json\r\nContent-Disposition: attachment; filename=\"resposta-ot.json\"\r\nContent-Transfer-Encoding: base64\r\n\r\n", limite)
	enc := base64.StdEncoding.EncodeToString(m.Anexo)
	for len(enc) > 76 {
		s.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	s.WriteString(enc + "\r\n--" + limite + "--\r\n")
	nome := filepath.Join(c.Pasta, hex.EncodeToString(b)+".eml")
	return os.WriteFile(nome, []byte(s.String()), 0o600)
}

// SMTP envia pelo servidor configurado (D-014). Usa STARTTLS quando o
// servidor oferece; Usuario vazio = sem autenticação.
type SMTP struct {
	Host, Porta, Usuario, Senha, De string
}

func (c SMTP) Enviar(m Mensagem) error {
	var corpo strings.Builder
	b := make([]byte, 8)
	rand.Read(b)
	limite := "limite-" + hex.EncodeToString(b)
	// SMTP_DE pode ser "Nome <endereco>"; o envelope (MAIL FROM) leva só o endereço.
	envelope := c.De
	if a, err := mail.ParseAddress(c.De); err == nil {
		envelope = a.Address
	}
	dominio := "localhost"
	if i := strings.LastIndex(envelope, "@"); i >= 0 {
		dominio = envelope[i+1:]
	}
	// Date e Message-ID: sem eles, provedores como o Gmail tendem a marcar a
	// mensagem como spam. O Message-ID é aleatório e não liga nada à compra.
	id := make([]byte, 16)
	rand.Read(id)
	fmt.Fprintf(&corpo, "Date: %s\r\nMessage-ID: <%s@%s>\r\n", time.Now().Format(time.RFC1123Z), hex.EncodeToString(id), dominio)
	fmt.Fprintf(&corpo, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", c.De, m.Para, mime.QEncoding.Encode("utf-8", m.Assunto))
	fmt.Fprintf(&corpo, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", limite)
	fmt.Fprintf(&corpo, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", limite, m.Texto)
	fmt.Fprintf(&corpo, "--%s\r\nContent-Type: application/json\r\nContent-Disposition: attachment; filename=\"resposta-ot.json\"\r\nContent-Transfer-Encoding: base64\r\n\r\n", limite)
	enc := base64.StdEncoding.EncodeToString(m.Anexo)
	for len(enc) > 76 {
		corpo.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	corpo.WriteString(enc + "\r\n--" + limite + "--\r\n")
	var auth smtp.Auth
	if c.Usuario != "" {
		auth = smtp.PlainAuth("", c.Usuario, c.Senha, c.Host)
	}
	return smtp.SendMail(net.JoinHostPort(c.Host, c.Porta), auth, envelope, []string{m.Para}, []byte(corpo.String()))
}

// Fila envia e-mails fora da requisição (RNF04: até 1 min após o pagamento),
// com novas tentativas. Erros vão para o log sem o destinatário.
type Fila struct {
	ch  chan Mensagem
	env Enviador
	log *slog.Logger
}

func NovaFila(env Enviador, log *slog.Logger) *Fila {
	f := &Fila{ch: make(chan Mensagem, 1024), env: env, log: log}
	go f.rodar()
	return f
}

func (f *Fila) Enfileirar(m Mensagem) { f.ch <- m }

func (f *Fila) rodar() {
	for m := range f.ch {
		for tentativa := 0; tentativa < 5; tentativa++ {
			if err := f.env.Enviar(m); err == nil {
				break
			} else if tentativa == 4 {
				f.log.Error("e-mail não enviado após 5 tentativas", "erro", err)
			}
			time.Sleep(time.Duration(tentativa+1) * 2 * time.Second)
		}
	}
}
