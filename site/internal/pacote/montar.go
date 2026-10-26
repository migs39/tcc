package pacote

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// FormatoAtual identifica o formato do catalogo.json; muda se o esquema mudar
// de forma incompatível.
const FormatoAtual = 1

// Catalogo é o conteúdo de catalogo.json.
type Catalogo struct {
	Formato    int         `json:"formato"`
	Categorias []Categoria `json:"categorias"`
	Grupos     []Grupo     `json:"grupos"`
	Livros     []Livro     `json:"livros"`
}

type Categoria struct {
	ID   int64  `json:"id"`
	Nome string `json:"nome"`
}

// Grupo é a versão atual de um grupo de mesmo preço. O índice de cada livro
// no OT é sua posição em Livros (RF07).
type Grupo struct {
	FaixaID       int64   `json:"faixa_id"`
	PrecoCentavos int     `json:"preco_centavos"`
	VersaoGrupoID int64   `json:"versao_grupo_id"`
	VersaoNumero  int     `json:"versao_numero"`
	Livros        []int64 `json:"livros"`
}

type Livro struct {
	ID            int64  `json:"id"`
	Titulo        string `json:"titulo"`
	Autor         string `json:"autor"`
	CategoriaID   int64  `json:"categoria_id"`
	Descricao     string `json:"descricao"`
	Ano           int    `json:"ano,omitempty"`
	ISBN          string `json:"isbn,omitempty"`
	Editora       string `json:"editora,omitempty"`
	FaixaID       int64  `json:"faixa_id"`
	PrecoCentavos int    `json:"preco_centavos"`
	Miniatura     string `json:"miniatura,omitempty"` // caminho dentro do ZIP
}

// dataFixa vai em todos os arquivos do ZIP para os bytes não dependerem do
// momento da geração.
var dataFixa = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// lerConteudo lê do banco tudo o que vai no pacote. Deve rodar numa transação
// REPEATABLE READ, para todas as consultas verem o mesmo estado.
func lerConteudo(ctx context.Context, tx pgx.Tx) (Catalogo, map[int64][]byte, error) {
	cat := Catalogo{Formato: FormatoAtual}

	linhas, err := tx.Query(ctx, `SELECT id, nome FROM categorias ORDER BY id`)
	if err != nil {
		return cat, nil, err
	}
	cat.Categorias, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[Categoria])
	if err != nil {
		return cat, nil, err
	}

	// Grupos: versão mais recente de cada faixa ativa, só se tiver livros.
	linhas, err = tx.Query(ctx,
		`SELECT DISTINCT ON (f.id) f.id, f.valor_centavos, v.id, v.numero
		   FROM faixas_preco f JOIN versoes_grupo v ON v.faixa_id = f.id
		  WHERE f.ativa
		  ORDER BY f.id, v.numero DESC`)
	if err != nil {
		return cat, nil, err
	}
	grupos, err := pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Grupo, error) {
		var g Grupo
		err := l.Scan(&g.FaixaID, &g.PrecoCentavos, &g.VersaoGrupoID, &g.VersaoNumero)
		return g, err
	})
	if err != nil {
		return cat, nil, err
	}
	for _, g := range grupos {
		linhas, err := tx.Query(ctx,
			`SELECT livro_id FROM versao_grupo_livros WHERE versao_id = $1 ORDER BY indice`, g.VersaoGrupoID)
		if err != nil {
			return cat, nil, err
		}
		g.Livros, err = pgx.CollectRows(linhas, pgx.RowTo[int64])
		if err != nil {
			return cat, nil, err
		}
		if len(g.Livros) > 0 {
			cat.Grupos = append(cat.Grupos, g)
		}
	}

	linhas, err = tx.Query(ctx,
		`SELECT l.id, l.titulo, l.autor, l.categoria_id, l.descricao, coalesce(l.ano, 0),
		        coalesce(l.isbn, ''), l.editora, l.faixa_id, f.valor_centavos, c.miniatura
		   FROM livros l
		   JOIN faixas_preco f ON f.id = l.faixa_id
		   LEFT JOIN capas c ON c.livro_id = l.id
		  WHERE l.ativo
		  ORDER BY l.id`)
	if err != nil {
		return cat, nil, err
	}
	miniaturas := map[int64][]byte{}
	cat.Livros, err = pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Livro, error) {
		var v Livro
		var mini []byte
		err := l.Scan(&v.ID, &v.Titulo, &v.Autor, &v.CategoriaID, &v.Descricao, &v.Ano,
			&v.ISBN, &v.Editora, &v.FaixaID, &v.PrecoCentavos, &mini)
		if mini != nil {
			v.Miniatura = "miniaturas/" + strconv.FormatInt(v.ID, 10) + ".jpg"
			miniaturas[v.ID] = mini
		}
		return v, err
	})
	if cat.Categorias == nil {
		cat.Categorias = []Categoria{}
	}
	if cat.Grupos == nil {
		cat.Grupos = []Grupo{}
	}
	if cat.Livros == nil {
		cat.Livros = []Livro{}
	}
	return cat, miniaturas, err
}

// montarZIP gera o ZIP de forma determinística: mesma ordem de arquivos,
// mesma data, mesmos métodos de compressão. Mesmo conteúdo, mesmos bytes.
func montarZIP(cat Catalogo, miniaturas map[int64][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	jsonBytes, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := escrever(zw, "catalogo.json", zip.Deflate, jsonBytes); err != nil {
		return nil, err
	}
	// JPEG já é comprimido: guardado sem compressão.
	for _, l := range cat.Livros {
		if mini, ok := miniaturas[l.ID]; ok {
			if err := escrever(zw, l.Miniatura, zip.Store, mini); err != nil {
				return nil, err
			}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func escrever(zw *zip.Writer, nome string, metodo uint16, dados []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: nome, Method: metodo, Modified: dataFixa})
	if err != nil {
		return err
	}
	_, err = w.Write(dados)
	return err
}
