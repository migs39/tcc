-- Catálogo (RF02), preços discretos (RF03) e grupos versionados (RF07, L2).
-- Tudo aqui é público ou administrativo: nenhuma tabela guarda dado de compra.

-- +goose Up

-- RF03: conjunto de preços pré-definidos. Valor em centavos e imutável:
-- mudar o preço de uma faixa repreçaria um grupo inteiro no meio de compras.
CREATE TABLE faixas_preco (
    id             BIGSERIAL PRIMARY KEY,
    valor_centavos INTEGER     NOT NULL UNIQUE CHECK (valor_centavos > 0),
    ativa          BOOLEAN     NOT NULL DEFAULT TRUE,
    criada_em      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categorias (
    id        BIGSERIAL PRIMARY KEY,
    nome      TEXT        NOT NULL CHECK (length(trim(nome)) > 0),
    criada_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX categorias_nome_unico ON categorias (lower(nome));

-- RF02. Livro "removido" fica inativo: versões antigas de grupo o referenciam.
-- RF03 imposto pelo banco: faixa_id é obrigatória e precisa existir.
CREATE TABLE livros (
    id            BIGSERIAL PRIMARY KEY,
    titulo        TEXT        NOT NULL CHECK (length(trim(titulo)) > 0),
    autor         TEXT        NOT NULL CHECK (length(trim(autor)) > 0),
    descricao     TEXT        NOT NULL DEFAULT '',
    ano           SMALLINT    CHECK (ano IS NULL OR ano > 0),
    isbn          TEXT,
    editora       TEXT        NOT NULL DEFAULT '',
    categoria_id  BIGINT      NOT NULL REFERENCES categorias (id) ON DELETE RESTRICT,
    faixa_id      BIGINT      NOT NULL REFERENCES faixas_preco (id) ON DELETE RESTRICT,
    ativo         BOOLEAN     NOT NULL DEFAULT TRUE,
    criado_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX livros_faixa_ativos ON livros (faixa_id) WHERE ativo;

-- Capa: até 2 MB, tipo conferido pelo conteúdo no upload.
CREATE TABLE capas (
    livro_id BIGINT PRIMARY KEY REFERENCES livros (id) ON DELETE CASCADE,
    tipo     TEXT   NOT NULL CHECK (tipo IN ('image/jpeg', 'image/png', 'image/webp')),
    dados    BYTEA  NOT NULL CHECK (octet_length(dados) BETWEEN 1 AND 2097152),
    sha256   BYTEA  NOT NULL
);

-- RF07 / L2: grupo = livros ativos de uma faixa. Cada mudança de composição
-- gera uma versão imutável; o índice do OT é a posição do livro na versão.
CREATE TABLE versoes_grupo (
    id        BIGSERIAL PRIMARY KEY,
    faixa_id  BIGINT      NOT NULL REFERENCES faixas_preco (id) ON DELETE RESTRICT,
    numero    INTEGER     NOT NULL CHECK (numero > 0),
    criada_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (faixa_id, numero)
);

CREATE TABLE versao_grupo_livros (
    versao_id BIGINT  NOT NULL REFERENCES versoes_grupo (id) ON DELETE RESTRICT,
    indice    INTEGER NOT NULL CHECK (indice >= 0),
    livro_id  BIGINT  NOT NULL REFERENCES livros (id) ON DELETE RESTRICT,
    PRIMARY KEY (versao_id, indice),
    UNIQUE (versao_id, livro_id)
);

-- +goose Down
DROP TABLE versao_grupo_livros;
DROP TABLE versoes_grupo;
DROP TABLE capas;
DROP TABLE livros;
DROP TABLE categorias;
DROP TABLE faixas_preco;
