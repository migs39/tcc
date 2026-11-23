// Package compra implementa o lado do site na compra privada: checkout sem
// conta (RF05, RF06), pagamento (RF13), sender do OT (RF07, RF08), assinatura
// cega dos tokens (RF16), e-mail e reenvio da resposta do OT (RF14, RF15),
// publicação dos livros cifrados na CDN e recebimento do reporte da CDN (RF18).
//
// O que o site aprende numa compra: e-mail, grupo, N, valor e pagamento. Nada
// aqui grava, loga ou devolve em erro índices escolhidos, chaves extraídas ou
// dados que liguem um token à compra (invariantes 1–4).
package compra

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/protocolo"
	"tcc/site/internal/chaves"
)

// PrazoReenvio é o prazo do RF15 (D-010).
const PrazoReenvio = 30 * 24 * time.Hour

// TamanhoMaximoRequisicao limita a requisição do OT (N·ℓ OTs MR19).
const TamanhoMaximoRequisicao = 1 << 20

type Servico struct {
	pool    *pgxpool.Pool
	ot      protocolo.OTHelper
	gateway Gateway
	fila    *Fila
	cdn     *ClienteCDN
	agora   func() time.Time
	// TarifaToken é a remuneração da CDN por token entregue, em centavos
	// (D-014). Não depende do livro.
	TarifaToken int
}

func NovoServico(pool *pgxpool.Pool, ot protocolo.OTHelper, gw Gateway, fila *Fila, cdn *ClienteCDN) *Servico {
	return &Servico{pool: pool, ot: ot, gateway: gw, fila: fila, cdn: cdn, agora: time.Now, TarifaToken: 50}
}

// ErroCompra é um erro de validação mostrado ao cliente. As mensagens são
// fixas e não dependem dos livros escolhidos.
type ErroCompra struct{ Msg string }

func (e ErroCompra) Error() string { return e.Msg }

var (
	ErrEmail         = ErroCompra{"e-mail inválido"}
	ErrQuantidade    = ErroCompra{fmt.Sprintf("quantidade deve ficar entre 1 e %d", protocolo.MaximoPorCompra)}
	ErrVersao        = ErroCompra{"versão do grupo desatualizada ou inexistente; atualize o catálogo"}
	ErrGrupoPequeno  = ErroCompra{fmt.Sprintf("grupo com menos de %d livros", protocolo.MinimoGrupo)}
	ErrQuantMaior    = ErroCompra{"quantidade maior que o grupo"}
	ErrRequisicao    = ErroCompra{"requisição de OT inválida"}
	ErrCegados       = ErroCompra{"valores cegados inválidos"}
	ErrNaoEncontrada = errors.New("compra não encontrada")
)

func validarEmail(e string) (string, bool) {
	e = strings.TrimSpace(e)
	a, err := mail.ParseAddress(e)
	if err != nil || a.Name != "" || a.Address != e || len(e) > 254 {
		return "", false
	}
	return e, true
}

// grupoAtual carrega a versão pedida, que precisa ser a atual de uma faixa
// ativa: o cliente escolheu os índices sobre ela (RF07, L2).
func (s *Servico) grupoAtual(ctx context.Context, versaoID int64) (faixa int64, preco int, livros []int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT f.id, f.valor_centavos
		   FROM versoes_grupo v JOIN faixas_preco f ON f.id = v.faixa_id
		  WHERE v.id = $1 AND f.ativa
		    AND v.numero = (SELECT max(numero) FROM versoes_grupo WHERE faixa_id = f.id)`,
		versaoID).Scan(&faixa, &preco)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil, ErrVersao
	}
	if err != nil {
		return 0, 0, nil, err
	}
	linhas, err := s.pool.Query(ctx,
		`SELECT livro_id FROM versao_grupo_livros WHERE versao_id = $1 ORDER BY indice`, versaoID)
	if err != nil {
		return 0, 0, nil, err
	}
	livros, err = pgx.CollectRows(linhas, pgx.RowTo[int64])
	return faixa, preco, livros, err
}

// Comprar executa os passos 2–4 do fluxo: valida, roda o sender do OT, cobra,
// assina às cegas, guarda a resposta para reenvio e agenda o e-mail.
func (s *Servico) Comprar(ctx context.Context, p protocolo.PedidoCompra) (protocolo.ResultadoCompra, error) {
	email, ok := validarEmail(p.Email)
	if !ok {
		return protocolo.ResultadoCompra{}, ErrEmail
	}
	if p.Quantidade < 1 || p.Quantidade > protocolo.MaximoPorCompra {
		return protocolo.ResultadoCompra{}, ErrQuantidade
	}
	if len(p.Cegados) != p.Quantidade {
		return protocolo.ResultadoCompra{}, ErrCegados
	}
	for _, c := range p.Cegados {
		if len(c) != protocolo.BitsChaveToken/8 {
			return protocolo.ResultadoCompra{}, ErrCegados
		}
	}
	if len(p.RequisicaoOT) == 0 || len(p.RequisicaoOT) > TamanhoMaximoRequisicao {
		return protocolo.ResultadoCompra{}, ErrRequisicao
	}

	faixa, preco, livros, err := s.grupoAtual(ctx, p.VersaoGrupoID)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	n := len(livros)
	if n < protocolo.MinimoGrupo {
		return protocolo.ResultadoCompra{}, ErrGrupoPequeno
	}
	if p.Quantidade > n {
		return protocolo.ResultadoCompra{}, ErrQuantMaior
	}

	// Sender do OT antes de cobrar: uma requisição inválida não gera cobrança.
	// O resultado só é entregue depois do pagamento aprovado.
	m := p.Quantidade * protocolo.BitsIndice(n)
	respSender, pares, err := s.ot.Sender(ctx, p.RequisicaoOT, m)
	if err != nil {
		return protocolo.ResultadoCompra{}, ErrRequisicao
	}
	chLivros, err := chaves.ChavesLivros(ctx, s.pool, livros)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	cifras, err := protocolo.CifrarGrupo(chLivros, pares, p.Quantidade)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	sk, err := chaves.ChaveToken(ctx, s.pool, faixa)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}

	// Invariante 4: cobra o grupo × N, nunca por livro.
	valor := preco * p.Quantidade
	pagID, aprovado, err := s.gateway.Cobrar(ctx, valor, p.Pagamento)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	if !aprovado {
		return protocolo.ResultadoCompra{Status: "recusada", Mensagem: "pagamento recusado"}, nil
	}

	assinaturas := make([][]byte, len(p.Cegados))
	for i, c := range p.Cegados {
		if assinaturas[i], err = protocolo.AssinarCego(sk, c); err != nil {
			return protocolo.ResultadoCompra{}, ErrCegados
		}
	}

	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	resp := &protocolo.RespostaOT{
		Formato:          protocolo.FormatoResposta,
		CompraID:         hex.EncodeToString(id),
		VersaoGrupoID:    p.VersaoGrupoID,
		Quantidade:       p.Quantidade,
		RespostaSender:   respSender,
		Cifras:           cifras,
		AssinaturasCegas: assinaturas,
	}
	corpo, err := json.Marshal(resp)
	if err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	agora := s.agora()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO compras (id, email, versao_grupo_id, quantidade, valor_centavos, pagamento_id, resposta_ot, reenvio_ate)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		resp.CompraID, email, p.VersaoGrupoID, p.Quantidade, valor, pagID, corpo, agora.Add(PrazoReenvio)); err != nil {
		return protocolo.ResultadoCompra{}, err
	}
	s.fila.Enfileirar(mensagemResposta(email, resp.CompraID, corpo, false))
	return protocolo.ResultadoCompra{Status: "aprovada", Resposta: resp}, nil
}

func mensagemResposta(para, id string, corpo []byte, reenvio bool) Mensagem {
	assunto := "Sua compra na Livraria OT"
	if reenvio {
		assunto = "Reenvio: sua compra na Livraria OT"
	}
	return Mensagem{
		Para:    para,
		Assunto: assunto,
		Texto: "Compra " + id + ".\r\n\r\n" +
			"Em anexo está a resposta do protocolo de transferência inconsciente. Ela não\r\n" +
			"contém a chave dos livros: só o programa local, com o segredo guardado no seu\r\n" +
			"computador, consegue extraí-la.\r\n\r\n" +
			"Se a compra foi interrompida, salve o anexo e, no mesmo computador da compra:\r\n" +
			"  - na janela Livraria OT: aba Compras > Recuperar uma compra pelo e-mail;\r\n" +
			"  - na linha de comando: livraria recuperar <arquivo resposta-ot.json>\r\n\r\n" +
			"Sem o segredo guardado no computador da compra, este anexo não abre os livros.\r\n",
		Anexo: corpo,
	}
}

// Reenviar manda de novo a resposta do OT ao e-mail original, dentro do prazo
// (RF15). Devolve ErrNaoEncontrada também fora do prazo.
func (s *Servico) Reenviar(ctx context.Context, compraID string) error {
	var email string
	var corpo []byte
	err := s.pool.QueryRow(ctx,
		`SELECT email, resposta_ot FROM compras WHERE id = $1 AND reenvio_ate > $2`,
		compraID, s.agora()).Scan(&email, &corpo)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNaoEncontrada
	}
	if err != nil {
		return err
	}
	s.fila.Enfileirar(mensagemResposta(email, compraID, corpo, true))
	return nil
}

// ErroReporte é um lote recusado.
type ErroReporte struct{ Msg string }

func (e ErroReporte) Error() string { return e.Msg }

// ReceberReporte valida um lote da CDN (UC17): cada token precisa ter
// assinatura válida de alguma faixa e não ter sido reportado antes. O lote é
// aceito inteiro ou recusado inteiro. Devolve a quantidade aceita.
func (s *Servico) ReceberReporte(ctx context.Context, r protocolo.ReporteCDN) (int, error) {
	if len(r.Tokens) == 0 || r.Quantidade != len(r.Tokens) || len(r.Tokens) > 10000 {
		return 0, ErroReporte{"lote vazio ou quantidade não confere"}
	}
	pubs, err := chaves.ChavesPublicasToken(ctx, s.pool)
	if err != nil {
		return 0, err
	}
	type lido struct {
		resumo [32]byte
		faixa  int64
	}
	lidos := make([]lido, 0, len(r.Tokens))
	vistos := map[[32]byte]bool{}
	for _, ts := range r.Tokens {
		t, err := protocolo.LerToken(ts)
		if err != nil {
			return 0, ErroReporte{"token malformado"}
		}
		faixa := int64(-1)
		for id, pk := range pubs {
			if protocolo.VerificarToken(pk, t) == nil {
				faixa = id
				break
			}
		}
		if faixa < 0 {
			return 0, ErroReporte{"token com assinatura inválida"}
		}
		h := sha256.Sum256(t.Mensagem)
		if vistos[h] {
			return 0, ErroReporte{"token repetido no lote"}
		}
		vistos[h] = true
		lidos = append(lidos, lido{h, faixa})
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, l := range lidos {
			tag, err := tx.Exec(ctx,
				`INSERT INTO tokens_reportados (resumo, faixa_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				l.resumo[:], l.faixa)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErroReporte{"token já reportado"}
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO lotes_cdn (quantidade, valor_centavos) VALUES ($1, $2)`,
			len(lidos), len(lidos)*s.TarifaToken)
		return err
	})
	if err != nil {
		return 0, err
	}
	return len(lidos), nil
}

// PublicarLivro cifra o arquivo do livro com a chave dele (D-009) e o envia à
// CDN. O arquivo em claro não fica guardado no site.
func (s *Servico) PublicarLivro(ctx context.Context, livroID int64, dados []byte) error {
	if s.cdn == nil {
		return errors.New("CDN não configurada")
	}
	ks, err := chaves.ChavesLivros(ctx, s.pool, []int64{livroID})
	if err != nil {
		return err
	}
	cifrado, err := protocolo.CifrarLivro(ks[0], livroID, dados)
	if err != nil {
		return err
	}
	if err := s.cdn.Publicar(ctx, livroID, cifrado); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE chaves_livros SET publicado_cdn = TRUE WHERE livro_id = $1`, livroID)
	return err
}

// Lote é um lote do reporte da CDN, para o painel administrativo.
type Lote struct {
	ID         int64
	Quantidade int
	Valor      int
	Recebido   string
}

// ResumoCDN são os números do painel /admin/cdn.
type ResumoCDN struct {
	Lotes, Tokens, Total, Tarifa int
	Lista                        []Lote
}

// Lotes devolve os lotes recebidos, do mais recente ao mais antigo.
func (s *Servico) Lotes(ctx context.Context) (ResumoCDN, error) {
	r := ResumoCDN{Tarifa: s.TarifaToken}
	linhas, err := s.pool.Query(ctx,
		`SELECT id, quantidade, valor_centavos, to_char(recebido_em, 'DD/MM/YYYY HH24:MI')
		   FROM lotes_cdn ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return r, err
	}
	r.Lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[Lote])
	if err != nil {
		return r, err
	}
	err = s.pool.QueryRow(ctx,
		`SELECT count(*), coalesce(sum(quantidade), 0), coalesce(sum(valor_centavos), 0) FROM lotes_cdn`).
		Scan(&r.Lotes, &r.Tokens, &r.Total)
	return r, err
}
