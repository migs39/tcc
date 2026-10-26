-- D-006: o programa local baixa o catálogo inteiro, com uma miniatura de
-- tamanho fixo por livro. A original fica só para a área administrativa.

-- +goose Up
ALTER TABLE capas ADD COLUMN miniatura BYTEA;
ALTER TABLE capas ADD COLUMN miniatura_sha256 BYTEA;

-- Capas gravadas antes desta migração não têm miniatura; são descartadas e
-- precisam ser reenviadas (só existiam em ambientes de desenvolvimento).
DELETE FROM capas;

ALTER TABLE capas ALTER COLUMN miniatura SET NOT NULL;
ALTER TABLE capas ALTER COLUMN miniatura_sha256 SET NOT NULL;

-- +goose Down
ALTER TABLE capas DROP COLUMN miniatura_sha256;
ALTER TABLE capas DROP COLUMN miniatura;
