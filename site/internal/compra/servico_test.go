package compra

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/protocolo"
	"tcc/site/internal/catalogo"
	"tcc/site/internal/chaves"
	"tcc/site/internal/db/dbteste"
)

func otHelper(t testing.TB) protocolo.OTHelper {
	t.Helper()
	c := os.Getenv("OT_HELPER")
	if c == "" {
		var err error
		if c, err = exec.LookPath("ot-helper"); err != nil {
			t.Skip("ot-helper indisponível; defina OT_HELPER")
		}
	}
	return protocolo.OTHelper{Caminho: c}
}

// enviadorMemoria guarda os e-mails para inspeção.
type enviadorMemoria struct{ ch chan Mensagem }

func (e enviadorMemoria) Enviar(m Mensagem) error { e.ch <- m; return nil }

type amb struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	cat    *catalogo.Servico
	svc    *Servico
	ot     protocolo.OTHelper
	emails chan Mensagem
	faixa  int64
	versao catalogo.VersaoGrupo
}

func novoAmb(t testing.TB, livros int) amb {
	t.Helper()
	ot := otHelper(t)
	pool := dbteste.Novo(t)
	a := amb{ctx: context.Background(), pool: pool, cat: catalogo.NovoServico(pool), ot: ot, emails: make(chan Mensagem, 16)}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a.svc = NovoServico(pool, ot, GatewayMock{}, NovaFila(enviadorMemoria{a.emails}, log), nil)
	c, err := a.cat.CriarCategoria(a.ctx, "Ficção")
	if err != nil {
		t.Fatal(err)
	}
	f, err := a.cat.CriarFaixa(a.ctx, 2990)
	if err != nil {
		t.Fatal(err)
	}
	a.faixa = f.ID
	for i := 0; i < livros; i++ {
		if _, err := a.cat.Cadastrar(a.ctx, catalogo.DadosLivro{
			Titulo: "Livro " + string(rune('A'+i)), Autor: "Autor", CategoriaID: c.ID, FaixaID: f.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := chaves.GarantirChavesToken(a.ctx, pool); err != nil {
		t.Fatal(err)
	}
	if a.versao, err = a.cat.VersaoAtual(a.ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	return a
}

// cliente faz o lado do programa local até o envio.
type cliente struct {
	sem      protocolo.Semente
	indices  []int
	escolhas string
	tokens   *protocolo.TokensPendentes
	pedido   protocolo.PedidoCompra
}

func (a amb) cliente(t testing.TB, indices []int, pagamento string) cliente {
	t.Helper()
	sem, _ := protocolo.NovaSemente()
	n := len(a.versao.Livros)
	esc, err := protocolo.EscolhasOT(indices, n)
	if err != nil {
		t.Fatal(err)
	}
	req, err := a.ot.Requisicao(a.ctx, sem.SementeOT(), esc)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := chaves.ChavesPublicasToken(a.ctx, a.pool)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := protocolo.CegarTokens(pub[a.faixa], sem.Fluxo("tokens"), len(indices))
	if err != nil {
		t.Fatal(err)
	}
	return cliente{sem, indices, esc, tp, protocolo.PedidoCompra{
		Email: "cliente@exemplo.com", VersaoGrupoID: a.versao.ID, Quantidade: len(indices),
		RequisicaoOT: req, Cegados: tp.Cegados, Pagamento: pagamento,
	}}
}

func (a amb) chave(t testing.TB, livro int64) [protocolo.TamChave]byte {
	ks, err := chaves.ChavesLivros(a.ctx, a.pool, []int64{livro})
	if err != nil {
		t.Fatal(err)
	}
	return ks[0]
}

// RF07, RF08, RF13, RF14, RF16: compra aprovada; o cliente extrai exatamente
// as chaves dos livros escolhidos e recebe tokens válidos.
func TestCompraPontaAPonta(t *testing.T) {
	a := novoAmb(t, 6)
	c := a.cliente(t, []int{4, 1}, "aprovar")
	res, err := a.svc.Comprar(a.ctx, c.pedido)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "aprovada" || res.Resposta == nil {
		t.Fatalf("compra não aprovada: %+v", res)
	}
	recebidos, err := a.ot.Extrair(a.ctx, c.sem.SementeOT(), c.escolhas, res.Resposta.RespostaSender)
	if err != nil {
		t.Fatal(err)
	}
	ks, err := protocolo.AbrirGrupo(res.Resposta.Cifras, recebidos, c.indices, len(a.versao.Livros))
	if err != nil {
		t.Fatal(err)
	}
	for k, i := range c.indices {
		if ks[k] != a.chave(t, a.versao.Livros[i]) {
			t.Errorf("chave do índice %d não confere", i)
		}
	}
	toks, err := c.tokens.Finalizar(res.Resposta.AssinaturasCegas)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := chaves.ChavesPublicasToken(a.ctx, a.pool)
	for _, tk := range toks {
		if protocolo.VerificarToken(pub[a.faixa], tk) != nil {
			t.Fatal("token inválido")
		}
	}

	// RF14 e invariante 2: o e-mail leva a resposta do OT, sem chave alguma.
	select {
	case m := <-a.emails:
		if m.Para != "cliente@exemplo.com" {
			t.Fatal("e-mail para o destinatário errado")
		}
		for _, id := range a.versao.Livros {
			k := a.chave(t, id)
			if bytes.Contains(m.Anexo, k[:]) || strings.Contains(string(m.Anexo), base64.StdEncoding.EncodeToString(k[:])) {
				t.Fatal("e-mail contém uma chave de livro")
			}
		}
		var r protocolo.RespostaOT
		if json.Unmarshal(m.Anexo, &r) != nil || r.CompraID != res.Resposta.CompraID {
			t.Fatal("anexo não é a resposta do OT")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("e-mail não enviado (RNF04)")
	}

	// Invariante 4: cobrança pelo grupo × N.
	var valor int
	a.pool.QueryRow(a.ctx, `SELECT valor_centavos FROM compras`).Scan(&valor)
	if valor != 2*2990 {
		t.Fatalf("valor cobrado %d", valor)
	}
}

// Invariante 1 e testes de privacidade: duas compras com escolhas diferentes
// deixam no site registros e respostas de mesma forma e tamanho; nada do que o
// site guarda depende dos índices.
func TestSiteNaoAprendeEscolha(t *testing.T) {
	a := novoAmb(t, 8)
	var tamanhos []int
	var linhas []string
	for _, ind := range [][]int{{0, 1, 2}, {7, 7, 5}} {
		c := a.cliente(t, ind, "aprovar")
		res, err := a.svc.Comprar(a.ctx, c.pedido)
		if err != nil {
			t.Fatal(err)
		}
		corpo, _ := json.Marshal(res.Resposta)
		tamanhos = append(tamanhos, len(corpo), len(c.pedido.RequisicaoOT))
		<-a.emails
	}
	if tamanhos[0] != tamanhos[2] || tamanhos[1] != tamanhos[3] {
		t.Fatalf("tamanhos dependem da escolha: %v", tamanhos)
	}
	rows, err := a.pool.Query(a.ctx,
		`SELECT email, versao_grupo_id, quantidade, valor_centavos FROM compras`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var e string
		var v int64
		var q, val int
		rows.Scan(&e, &v, &q, &val)
		linhas = append(linhas, e+"|"+string(rune(v))+"|"+string(rune(q))+"|"+string(rune(val)))
	}
	if len(linhas) != 2 || linhas[0] != linhas[1] {
		t.Fatal("registros das compras diferem além do identificador")
	}
	// As colunas de compras são só as previstas na migração.
	var colunas []string
	r2, _ := a.pool.Query(a.ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'compras' AND table_schema = current_schema() ORDER BY column_name`)
	for r2.Next() {
		var c string
		r2.Scan(&c)
		colunas = append(colunas, c)
	}
	esperadas := "criada_em,email,id,pagamento_id,quantidade,reenvio_ate,resposta_ot,valor_centavos,versao_grupo_id"
	if strings.Join(colunas, ",") != esperadas {
		t.Fatalf("colunas de compras mudaram: %v", colunas)
	}
}

// RF13: pagamento recusado não gera compra nem e-mail.
func TestPagamentoRecusado(t *testing.T) {
	a := novoAmb(t, 5)
	c := a.cliente(t, []int{0}, "recusar")
	res, err := a.svc.Comprar(a.ctx, c.pedido)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "recusada" || res.Resposta != nil {
		t.Fatal("pagamento recusado entregou resposta")
	}
	var n int
	a.pool.QueryRow(a.ctx, `SELECT count(*) FROM compras`).Scan(&n)
	if n != 0 {
		t.Fatal("compra recusada foi gravada")
	}
}

// D-010 e validações.
func TestValidacoes(t *testing.T) {
	a := novoAmb(t, 4) // abaixo do mínimo
	c := a.cliente(t, []int{0}, "aprovar")
	if _, err := a.svc.Comprar(a.ctx, c.pedido); err != ErrGrupoPequeno {
		t.Fatalf("grupo pequeno aceito: %v", err)
	}
	b := novoAmb(t, 5)
	c = b.cliente(t, []int{0}, "aprovar")
	p := c.pedido
	p.Email = "não é e-mail"
	if _, err := b.svc.Comprar(b.ctx, p); err != ErrEmail {
		t.Fatalf("e-mail inválido aceito: %v", err)
	}
	p = c.pedido
	p.VersaoGrupoID = 999999
	if _, err := b.svc.Comprar(b.ctx, p); err != ErrVersao {
		t.Fatalf("versão inexistente aceita: %v", err)
	}
	p = c.pedido
	p.RequisicaoOT = []byte{1, 2, 3}
	if _, err := b.svc.Comprar(b.ctx, p); err != ErrRequisicao {
		t.Fatalf("requisição inválida aceita: %v", err)
	}
	p = c.pedido
	p.Quantidade = 2
	if _, err := b.svc.Comprar(b.ctx, p); err != ErrCegados {
		t.Fatalf("quantidade sem cegados aceita: %v", err)
	}
}

// RF15: reenvio para o e-mail original dentro do prazo; fora do prazo, nada.
func TestReenvio(t *testing.T) {
	a := novoAmb(t, 5)
	c := a.cliente(t, []int{2}, "aprovar")
	res, err := a.svc.Comprar(a.ctx, c.pedido)
	if err != nil {
		t.Fatal(err)
	}
	<-a.emails
	if err := a.svc.Reenviar(a.ctx, res.Resposta.CompraID); err != nil {
		t.Fatal(err)
	}
	m := <-a.emails
	if m.Para != "cliente@exemplo.com" || !strings.HasPrefix(m.Assunto, "Reenvio") {
		t.Fatal("reenvio errado")
	}
	a.svc.agora = func() time.Time { return time.Now().Add(PrazoReenvio + time.Hour) }
	if err := a.svc.Reenviar(a.ctx, res.Resposta.CompraID); err != ErrNaoEncontrada {
		t.Fatal("reenvio fora do prazo aceito")
	}
}

// RF18, UC17: lote válido aceito; repetido, adulterado ou com contagem
// errada recusado.
func TestReporteCDN(t *testing.T) {
	a := novoAmb(t, 5)
	c := a.cliente(t, []int{0, 3}, "aprovar")
	res, err := a.svc.Comprar(a.ctx, c.pedido)
	if err != nil {
		t.Fatal(err)
	}
	<-a.emails
	toks, err := c.tokens.Finalizar(res.Resposta.AssinaturasCegas)
	if err != nil {
		t.Fatal(err)
	}
	lote := protocolo.ReporteCDN{Tokens: []string{toks[1].String(), toks[0].String()}, Quantidade: 2}
	if n, err := a.svc.ReceberReporte(a.ctx, lote); err != nil || n != 2 {
		t.Fatalf("lote válido recusado: %v", err)
	}
	if _, err := a.svc.ReceberReporte(a.ctx, lote); err == nil {
		t.Fatal("lote repetido aceito")
	}
	ruim := toks[0]
	ruim.Assinatura = append([]byte{}, ruim.Assinatura...)
	ruim.Assinatura[5] ^= 1
	if _, err := a.svc.ReceberReporte(a.ctx, protocolo.ReporteCDN{Tokens: []string{ruim.String()}, Quantidade: 1}); err == nil {
		t.Fatal("token adulterado aceito")
	}
	if _, err := a.svc.ReceberReporte(a.ctx, protocolo.ReporteCDN{Tokens: []string{toks[0].String()}, Quantidade: 3}); err == nil {
		t.Fatal("quantidade errada aceita")
	}
	// D-014: remuneração = quantidade × tarifa.
	res2, err := a.svc.Lotes(a.ctx)
	if err != nil || res2.Lotes != 1 || res2.Tokens != 2 || res2.Total != 2*a.svc.TarifaToken {
		t.Fatalf("lotes: %+v %v", res2, err)
	}
}

// RF14 (stub): o e-mail gravado em arquivo tem o anexo e permissão restrita.
func TestCaixaDeSaida(t *testing.T) {
	dir := t.TempDir()
	if err := (CaixaDeSaida{Pasta: dir}).Enviar(Mensagem{Para: "a@b.com", Assunto: "x", Texto: "y", Anexo: []byte(`{"a":1}`)}); err != nil {
		t.Fatal(err)
	}
	arqs, _ := filepath.Glob(filepath.Join(dir, "*.eml"))
	if len(arqs) != 1 {
		t.Fatal("e-mail não gravado")
	}
	b, _ := os.ReadFile(arqs[0])
	if !strings.Contains(string(b), base64.StdEncoding.EncodeToString([]byte(`{"a":1}`))) {
		t.Fatal("anexo ausente")
	}
}

// RNF03/RNF05: custo de uma compra no site (sender do OT, cifras de
// Naor–Pinkas, cobrança mock, N assinaturas cegas RSA-3072 e gravação).
// go test -run x -bench Compra ./internal/compra
func BenchmarkCompra(b *testing.B) {
	a := novoAmb(b, 100)
	clientes := make([]cliente, b.N)
	for i := range clientes {
		clientes[i] = a.cliente(b, []int{3, 50, 99, 0, 42}, "aprovar")
	}
	go func() {
		for range a.emails {
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.svc.Comprar(a.ctx, clientes[i].pedido); err != nil {
			b.Fatal(err)
		}
	}
}

// RNF05 (≥ 600 transações/min): compras simultâneas disputando o banco e o
// ot-helper. A requisição de cada cliente é gerada antes do cronômetro, porque
// roda no programa local, não no site. Reporta compras/min.
// go test -run x -bench CompraConcorrente -cpu 1,4,8 ./internal/compra
func BenchmarkCompraConcorrente(b *testing.B) {
	a := novoAmb(b, 100)
	clientes := make([]cliente, b.N)
	for i := range clientes {
		clientes[i] = a.cliente(b, []int{3, 50, 99, 0, 42}, "aprovar")
	}
	go func() {
		for range a.emails {
		}
	}()
	var prox atomic.Int64
	b.SetParallelism(4) // 4 × GOMAXPROCS clientes simultâneos
	b.ResetTimer()
	inicio := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := prox.Add(1) - 1
			res, err := a.svc.Comprar(a.ctx, clientes[i].pedido)
			if err != nil {
				b.Error(err)
				return
			}
			if res.Status != "aprovada" {
				b.Errorf("compra %d: %s", i, res.Status)
				return
			}
		}
	})
	b.ReportMetric(float64(b.N)/time.Since(inicio).Minutes(), "compras/min")
}
