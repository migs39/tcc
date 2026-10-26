-- D-006: pacote do catálogo baixado inteiro pelo programa local.

-- +goose Up

-- Contador de revisões: qualquer mudança no que vai para o pacote o incrementa
-- (via trigger), e o site só regera o pacote quando ele muda.
CREATE TABLE catalogo_revisao (
    unica   BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (unica),
    revisao BIGINT  NOT NULL
);
INSERT INTO catalogo_revisao (unica, revisao) VALUES (TRUE, 1);

-- +goose StatementBegin
CREATE FUNCTION catalogo_mudou() RETURNS trigger AS $$
BEGIN
    UPDATE catalogo_revisao SET revisao = revisao + 1;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER livros_mudou AFTER INSERT OR UPDATE OR DELETE ON livros
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();
CREATE TRIGGER categorias_mudou AFTER INSERT OR UPDATE OR DELETE ON categorias
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();
CREATE TRIGGER faixas_mudou AFTER INSERT OR UPDATE OR DELETE ON faixas_preco
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();
CREATE TRIGGER capas_mudou AFTER INSERT OR UPDATE OR DELETE ON capas
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();
CREATE TRIGGER versoes_mudou AFTER INSERT OR UPDATE OR DELETE ON versoes_grupo
    FOR EACH STATEMENT EXECUTE FUNCTION catalogo_mudou();

-- Cada versão publicada do pacote. O mesmo conteúdo gera os mesmos bytes;
-- versão nova só quando o conteúdo muda em relação à última.
CREATE TABLE pacotes_catalogo (
    versao     BIGSERIAL PRIMARY KEY,
    sha256     BYTEA       NOT NULL CHECK (octet_length(sha256) = 32),
    assinatura BYTEA       NOT NULL CHECK (octet_length(assinatura) = 64),
    dados      BYTEA       NOT NULL,
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE pacotes_catalogo;
DROP TRIGGER versoes_mudou ON versoes_grupo;
DROP TRIGGER capas_mudou ON capas;
DROP TRIGGER faixas_mudou ON faixas_preco;
DROP TRIGGER categorias_mudou ON categorias;
DROP TRIGGER livros_mudou ON livros;
DROP FUNCTION catalogo_mudou();
DROP TABLE catalogo_revisao;
