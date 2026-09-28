-- RF01 / UC01: contas de administradores e sessões da área administrativa.
-- Nenhuma tabela aqui guarda dado de cliente ou de compra.

-- +goose Up
CREATE TABLE administradores (
    id            BIGSERIAL PRIMARY KEY,
    nome          TEXT        NOT NULL CHECK (length(trim(nome)) > 0),
    email         TEXT        NOT NULL CHECK (position('@' IN email) > 1),
    senha_hash    TEXT        NOT NULL,           -- argon2id em formato PHC
    ativo         BOOLEAN     NOT NULL DEFAULT TRUE,
    criado_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- E-mail único sem diferenciar maiúsculas.
CREATE UNIQUE INDEX administradores_email_unico ON administradores (lower(email));

-- Só o hash SHA-256 do token fica no banco; o token em si vive apenas no cookie.
CREATE TABLE sessoes_admin (
    token_hash BYTEA       PRIMARY KEY,
    admin_id   BIGINT      NOT NULL REFERENCES administradores (id) ON DELETE CASCADE,
    expira_em  TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessoes_admin_admin_id ON sessoes_admin (admin_id);

-- +goose Down
DROP TABLE sessoes_admin;
DROP TABLE administradores;
