# Estado do projeto — Livraria Digital com OT

Atualizado em: 2026-10-26

Documento para o grupo. Resume o que foi decidido, onde está cada parte e o que falta.
- **Detalhe e justificativa das decisões:** `CLAUDE.md`, seção "Decisões tomadas" (IDs D-001 a D-007).
- **Divergências e lacunas entre PRD e especificação:** `docs/divergencias_docs.md`.

---

## 1. Arquitetura atual

Mudou em relação à especificação v2 (ver D-004 e a divergência D3).

| Componente | Responsabilidade | Requisitos |
|---|---|---|
| **Site** | Administra e publica o catálogo (pacote completo, igual para todos; sem vitrine pública), preços discretos, gateway, lado *sender* do OT, assinatura cega de tokens. Compra sem conta, só e-mail. | RF01–RF03, RF05–RF08, RF13–RF16 |
| **CDN** | Guarda livros cifrados; entrega 1 livro por token válido e não usado; reporta só tokens usados + quantidade. | RF17, RF18 |
| **Programa local (cliente)** | Baixa o catálogo inteiro e faz navegação e busca localmente (D-006). Gera a requisição de OT e a semente, fala **só** com o site e a CDN, extrai as chaves e decifra localmente. Não é código servido pelo site. | RF04, RF10–RF12 (RF10 alterado) |
| Gateway (externo) | Cobra pelo grupo e por N. | RF13 |

**Fluxo de compra:**
1. O programa local, com o catálogo já baixado (D-006), escolhe N livros do grupo, sorteia uma semente nova (CSPRNG) e salva semente + escolhas (RF11, D-001).
2. O programa envia ao site: e-mail, requisição OT e N valores cegados (RF06, RF07).
3. O site cobra o grupo × N no gateway (RF13).
4. Pago: o site devolve a resposta do OT e as N assinaturas cegas; a resposta do OT também vai por e-mail (RF08, RF14, RF16).
5. O programa remove o cegamento e troca cada token por um livro cifrado na CDN (RF17).
6. A CDN reporta ao site só tokens usados + quantidade (RF18).
7. O programa extrai as chaves com a semente e decifra. A chave nunca sai da máquina (RF12).

Se algo falhar entre os passos 3 e 7, o programa reexecuta o receptor com a semente salva e usa a resposta do OT recebida por e-mail (D-001).

---

## 2. Decisões tomadas

| ID | Tema | Decisão | Situação |
|---|---|---|---|
| D-001 | Execução do OT | Síncrona na compra; a semente do PRNG do receptor fica salva e permite reextrair as chaves a partir da resposta do e-mail | Decidida; determinismo do receptor conferido no código, com teste automatizado no passo 6 |
| D-002 | Biblioteca de OT | libOTe (C++, MIT), OT base Masny–Rindal [MR19], via binário C++ auxiliar | Decidida, sem spike; build no Windows e desempenho verificados por testes no passo 6 (risco aceito) |
| D-003 | Stack do site | Go + PostgreSQL + HTML renderizado no servidor; catálogo inteiro com filtro no navegador; libOTe via binário C++ auxiliar | Decidida |
| D-003 (compl.) | Ferramentas | Um módulo Go por componente; `net/http` padrão; `pgx` v5; migrações `goose`; argon2id; sessão de admin própria; Postgres via Docker Compose; identificadores em português | Decidida |
| D-004 | Programa local | Deixa de ser offline; usa rede só com o site e a CDN | Decidida; especificação v3 atualizada, PRD pendente |
| D-005 | Modelo do catálogo | Livro completo com capa (Postgres, 2 MB); faixas de preço editáveis e imutáveis; categorias controladas; nova versão de grupo só quando a composição muda | Decidida e implementada (passo 2) |
| D-006 | Navegação e busca | No programa local, sobre o pacote completo do catálogo; site sem vitrine pública; miniaturas 200×300 JPEG | Decidida; miniaturas implementadas, pacote no passo 3 |
| D-007 | Formato do pacote | ZIP único determinístico (JSON + miniaturas), pacote completo com ETag, assinatura Ed25519 + hash público, versão só cresce | Decidida e implementada (passo 3) |

**Descartados, com motivo registrado no CLAUDE.md:**
- emp-ot (não suporta Windows);
- circl `simot`;
- mpz e swanky ("não usar em produção");
- OT via OPRF (foge do foco do TCC);
- receptor do OT no navegador (o invariante 1 viraria premissa).

---

## 3. Decisões em aberto

| Tema | Necessária antes de |
|---|---|
| Gateway de pagamento: mock (recomendado) ou sandbox real | Passo 4 |
| Como o pagamento se encaixa com o programa local (o programa abre o navegador ou intermedeia) | Passo 4 |
| Variante de OT: 1-de-n repetido (Naor–Pinkas 1999 sobre 1-de-2), k-de-n ou GOT | Passo 6 |
| ~~Formato do pacote do catálogo~~ decidido em D-007 | — |
| Formato dos artefatos: requisição OT, resposta OT, tokens, livro cifrado | Passos 6–7 |
| Interface do programa local (agora também é a vitrine, D-006) | Programa local |
| Algoritmo simétrico (≥ 128 bits, RNF01) | Passo 6 |
| Tamanho mínimo de grupo de mesmo preço | Passo 6 |
| Política de chaves da assinatura cega (conjunto de anonimato, lacuna L4) | Passo 7 |
| Token de uso único ou novo download | Passo 7 / CDN |
| Frequência e lotes do reporte da CDN; remuneração | Passo 8 |
| Prazo de reenvio (RF15) | Passo 9 |
| Proteção da semente em disco no programa local | Programa local |

---

## 4. Plano do site e situação

| # | Entrega | Requisitos | Situação |
|---|---|---|---|
| 0 | Esqueleto: repositório, módulo Go, Postgres (Docker), migrações, testes | — | **Concluído**: `docker-compose.yml` (Postgres 17), migrações goose embutidas e aplicadas na subida, bancos de teste isolados por schema |
| 1 | Admins: cadastro, login, gestão; primeiro admin por comando de seed | RF01, UC01 | **Concluído**: argon2id, sessão com hash do token, desativar/reativar (nunca o último ativo), troca de senha encerra sessões, CSRF, CSP. 13 testes passando (9 de integração contra Postgres) |
| 2 | Catálogo: CRUD de livros, preços discretos, categorias, capas, grupos versionados | RF02, RF03, UC02, UC03 | **Concluído** (D-005): telas de livros, preços e categorias; capa no Postgres; RF03 imposto também pelo banco; versões de grupo imutáveis. miniatura 200×300 para o pacote (D-006); original só na área admin. 24 testes passando no site |
| 3 | Pacote do catálogo: ZIP único e determinístico (catalogo.json + miniaturas), igual para todos, assinado (Ed25519) com hash público; a busca roda no programa local (D-006, D-007) | RF04, RNF02, UC04 | **Concluído**: `GET /catalogo.zip` (ETag, 304) e `GET /catalogo/hash`; versão nova só quando o conteúdo muda. Página pública de apresentação e download em `/` (mostra versão, hash e chave pública do catálogo; sem cookies nem terceiros). 29 testes passando no site |
| 4 | Checkout sem conta: e-mail, gateway, aprovado/recusado | RF05, RF06, RF13 | Não iniciado |
| 5 | E-mail com a resposta do OT (stub), assíncrono | RF14, RNF04 | Não iniciado |
| 6 | OT real no lado sender (binário C++ auxiliar com libOTe/MR19), com testes de build no Windows, determinismo por semente e benchmark (RNF03, RNF05) | RF07, RF08 | Não iniciado; faltam CMake e compilador C++ |
| 7 | Assinatura cega (RSA RFC 9474, circl) | RF16 | Não iniciado |
| 8 | Recebimento e validação do reporte da CDN | RF18, UC17 | Não iniciado |
| 9 | Reenvio da resposta do OT | RF15 | Não iniciado |
| 10 | Testes de privacidade e carga | RNF02, RNF03, RNF05 | Não iniciado |

CDN e programa local: não iniciados.

---

## 5. Ambiente de desenvolvimento

| Ferramenta | Situação na máquina de desenvolvimento |
|---|---|
| Go | Instalado (1.27) |
| Git | Instalado |
| Docker Desktop | Funcionando (WSL instalado); Postgres sobe com `docker compose up -d` |
| CMake + compilador C++ | Não instalados (necessários no passo 6, para o libOTe) |
| LaTeX (MiKTeX) | Instalado; a especificação v3 compila sem erros |

---

## 6. Pendências de documentação

- ~~Atualizar a especificação~~ Feito: especificação **v3** (`docs/Especificacao_Detalhada_v3.pdf`, fonte em `docs/especificacao/`). Falta a lacuna L4 (política de chaves dos tokens), que depende de decisão.
- Atualizar o PRD (documento de outra pessoa do grupo, não refeito): lista do que mudar em `docs/prd_alteracoes.md`.
- Corrigir no PRD as divergências D1 (gestão de admins) e D2 (UC17 sem RF).
- Levar ao orientador: a escolha do libOTe/MR19 e a construção 1-de-n sobre 1-de-2.
