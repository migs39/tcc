-- D-014: remuneração da CDN por tarifa fixa por token. O valor de cada lote
-- é quantidade × tarifa vigente; não depende de livro nem de horário de token.

-- +goose Up
ALTER TABLE lotes_cdn ADD COLUMN valor_centavos INTEGER NOT NULL DEFAULT 0 CHECK (valor_centavos >= 0);

-- +goose Down
ALTER TABLE lotes_cdn DROP COLUMN valor_centavos;
