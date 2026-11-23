-- Compra privada (RF05–RF08, RF13–RF16), chaves (D-009, D-011) e reporte da
-- CDN (RF18, UC17).
--
-- Invariantes (CLAUDE.md): nenhuma tabela guarda índices escolhidos, chaves
-- extraídas pelo cliente, valores cegados, tokens ligados a compras ou
-- horários por token.

-- +goose Up

-- D-009: chave AES-256 de cada livro. A CDN guarda só o livro cifrado.
CREATE TABLE chaves_livros (
    livro_id      BIGINT  PRIMARY KEY REFERENCES livros (id) ON DELETE RESTRICT,
    chave         BYTEA   NOT NULL CHECK (octet_length(chave) = 32),
    publicado_cdn BOOLEAN NOT NULL DEFAULT FALSE
);

-- D-011: chave RSA-3072 de assinatura cega, uma por faixa de preço. A pública
-- vai no pacote assinado do catálogo (igual para todos).
CREATE TABLE chaves_tokens (
    faixa_id BIGINT PRIMARY KEY REFERENCES faixas_preco (id) ON DELETE RESTRICT,
    privada  BYTEA  NOT NULL, -- PKCS#8 DER
    publica  BYTEA  NOT NULL  -- PKIX DER
);
CREATE TRIGGER chaves_tokens_mudou AFTER INSERT OR UPDATE OR DELETE ON chaves_tokens
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();

-- Compra aprovada: e-mail, grupo, quantidade, valor e a resposta do OT para o
-- reenvio (RF15). O site sabe quem comprou e de qual grupo, nunca o quê.
CREATE TABLE compras (
    id              TEXT        PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
    email           TEXT        NOT NULL,
    versao_grupo_id BIGINT      NOT NULL REFERENCES versoes_grupo (id) ON DELETE RESTRICT,
    quantidade      INTEGER     NOT NULL CHECK (quantidade > 0),
    valor_centavos  INTEGER     NOT NULL CHECK (valor_centavos > 0),
    pagamento_id    TEXT        NOT NULL,
    resposta_ot     BYTEA       NOT NULL,
    criada_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    reenvio_ate     TIMESTAMPTZ NOT NULL
);

-- UC17: tokens já reportados (só o resumo da mensagem, para recusar repetição)
-- e os lotes recebidos, com a quantidade para a remuneração. Sem horário por
-- token e sem ligação com compras (RF16, RF18).
CREATE TABLE tokens_reportados (
    resumo   BYTEA  PRIMARY KEY CHECK (octet_length(resumo) = 32),
    faixa_id BIGINT NOT NULL REFERENCES faixas_preco (id) ON DELETE RESTRICT
);

CREATE TABLE lotes_cdn (
    id          BIGSERIAL   PRIMARY KEY,
    quantidade  INTEGER     NOT NULL CHECK (quantidade > 0),
    recebido_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE lotes_cdn;
DROP TABLE tokens_reportados;
DROP TABLE compras;
DROP TABLE chaves_tokens;
DROP TABLE chaves_livros;
