# CLAUDE.md — TCC: Livraria Digital com Oblivious Transfer

TCC de Engenharia (PCS/Poli-USP, 2026): "Aplicação de Protocolos de Oblivious Transfer para Preservação de Privacidade em Transações Digitais".
Autores: João Paulo Garcia, João Pedro Magalhães, Miguel Luis Schwengber. Orientador: Prof. Dr. Thales Areco Bandiera Paiva.

Produto: protótipo de livraria digital em que a loja recebe o pagamento, mas **não sabe quais livros o cliente comprou**.

## Como trabalhar comigo

- Seja direto. Sem lição de moral.
- **Consulte antes de decidir.** Qualquer escolha de arquitetura, stack, biblioteca, formato ou parâmetro que não esteja definida aqui: apresente as opções com trade-offs e espere minha resposta. Não crie estrutura de pastas, dependências ou convenções novas sem eu aprovar.
- Fontes de verdade: `docs/Especificacao_Detalhada_v3.pdf` (especificação; fonte LaTeX em `docs/especificacao/`, alterações da v2 em `docs/especificacao_v3_alteracoes.md`) e `docs/PRD_Livraria_OT.pdf`. Consulte antes de responder sobre requisitos. Se divergirem, **a especificação vence** e você aponta a divergência.
- Cite requisitos pelo ID (RF01–RF18, RNF01–RNF08, UC01–UC20) em código, commits, testes e explicações.
- Distinga sempre: **garantia formal** (prova/propriedade do protocolo) × **premissa** (ex.: não conivência) × **escolha de engenharia**.
- **Git:** nunca faça commit ou push sem pedido explícito, com ou sem auto mode. Pedido de commit não autoriza push, e vice-versa. Repositório: https://github.com/migs39/tcc.

## Arquitetura (não mudar sem eu pedir) — RF09

| Componente | Responsabilidade | Requisitos |
|---|---|---|
| **Site (sistema web)** | Catálogo, admin, preços discretos, gateway de pagamento, lado *sender* do OT, assinatura cega de tokens. Compra sem conta, só com e-mail. | RF01–RF08, RF13–RF16 |
| **CDN** | Guarda livros cifrados; entrega 1 livro por token cego válido e não usado; reporta ao site só tokens usados e quantidade. | RF17, RF18 |
| **Programa local (cliente)** | Gera a requisição de OT, guarda o segredo, extrai as chaves e descriptografa. Pode usar rede, **só** para falar com o site e a CDN (D-004). Não é código servido pelo site. | RF10–RF12 (RF10 alterado por D-004) |
| Gateway (externo) | Cobra pelo grupo e pela quantidade N. | RF13 |

### Fluxo de compra (7 passos, PRD)
1. Programa local gera a requisição de OT e guarda o segredo (RF11).
2. Cliente envia ao site: e-mail + requisição OT + N valores cegados (RF06, RF07).
3. Site cobra via gateway (RF13).
4. Pago → site devolve resposta do OT + N assinaturas cegas; resposta do OT também vai por e-mail (RF08, RF14, RF16).
5. Cliente remove o cegamento e troca cada token por um livro cifrado na CDN (RF17).
6. CDN reporta ao site só tokens usados + quantidade (RF18).
7. Programa local extrai as chaves com o segredo e decifra, localmente (RF10 alterado por D-004, RF12).

A chave final só existe no passo 7, dentro do programa local. Ela nunca trafega pela rede.

## Invariantes de privacidade

Qualquer código ou proposta que viole um destes está **errado**. Revise contra eles em toda mudança.

1. O site nunca aprende os índices escolhidos nem a chave final (RF07, RF08).
2. E-mail (RF14) e reenvio (RF15) carregam a **resposta do OT**, nunca a chave.
3. Tokens não são ligáveis à sessão de assinatura (RF16). O reporte da CDN não traz o livro por token nem horários (RF18).
4. A cobrança é pelo grupo e pela quantidade N, nunca por livro.
5. Não conivência site–CDN é **premissa** (auditoria), não falha a corrigir. Esconder *quem* comprou está fora do escopo.
6. Perder o segredo local = perder o acesso. O reenvio não recupera nada.

### O que cada parte sabe (use para revisar código e schema)

| Parte | Sabe | Não pode saber |
|---|---|---|
| Site | e-mail, pagamento, grupo, N | quais livros do grupo; a chave final |
| Site, ao receber reporte | tokens usados, quantidade | em qual compra cada token foi emitido |
| CDN | quais livros foram baixados, por token | quem comprou |
| Gateway | valor e dados de pagamento | quais livros |
| E-mail do cliente | resposta do OT | a chave (sem o segredo local) |

### Consequências práticas no código
- Nada no site (banco, logs, métricas, analytics, mensagens de erro) pode conter índices escolhidos, chaves de livro, ou qualquer dado que ligue token ↔ compra.
- Cuidado com canais laterais: tamanho de payload, ordem, timing ou logs que variem conforme o livro escolhido.
- Reporte da CDN: sem timestamps por token, sem id de livro por token. Frequência/lote é decisão em aberto.
- Programa local só conecta ao site e à CDN (D-004). Nem telemetria, nem checagem de atualização, nem terceiros. Chave de livro e semente nunca saem da máquina.
- Aleatoriedade sempre de CSPRNG do sistema/biblioteca criptográfica.
- Testes de privacidade são requisito, não extra: devem mostrar que o que o site recebe (incluindo o reporte da CDN) não permite inferir a escolha.

## Rigor criptográfico

- Use **bibliotecas consolidadas** para primitivas simétricas e assinatura cega. Não invente primitivas.
- Implementar OT/Rabin do zero **só com fins didáticos**, e marcado como tal no código (comentário no topo + nome do módulo) — nunca no caminho de produção sem eu decidir.
- **Rabin:** impor `p ≡ q ≡ 3 (mod 4)` na geração de chaves (senão `c^((p+1)/4)` não é raiz). Tratar a ambiguidade das 4 raízes explicitamente (via CRT) e documentar como a raiz correta é identificada; cuidado com redundância ingênua (abre ataque de texto cifrado escolhido / quebra a redução à fatoração).
- **RNF01 (128 bits) vale para todas as primitivas.** Para fatoração (Rabin/RSA) isso significa módulo de ~3072 bits. Ao propor parâmetros: verifique contra RNF01 e mostre o custo em RNF03 (≤ 1 min por compra) e RNF05 (≥ 600 transações/min).
- Desempenho de OT: considere **OT Extension** e **k-out-of-n**; sugira medir cedo (benchmark antes de construir em cima).

## Requisitos não funcionais (metas)

| ID | Meta |
|---|---|
| RNF01 | ≥ 128 bits de segurança |
| RNF02 | Busca ≤ 5 s |
| RNF03 | Finalização da compra ≤ 1 min (do envio da requisição OT à resposta) |
| RNF04 | E-mail ≤ 1 min após pagamento |
| RNF05 | ≥ 600 transações/min |
| RNF06 | ≥ 99% uptime/mês |
| RNF07 | ≥ 99% das compras concluídas sem suporte |
| RNF08 | Mudança simples ≤ 8 h úteis |

## Decisões em aberto — NÃO decida sozinho

Quando um tema tocar em qualquer uma destas, apresente opções com trade-offs e pare:

- Variante de OT: 1-out-of-n repetido, k-out-of-n ou GOT
- Tamanho mínimo de grupo de mesmo preço
- Prazo de reenvio (RF15)
- Token de uso único ou permite novo download do mesmo livro
- Frequência e lotes do reporte da CDN; remuneração por token
- Algoritmo simétrico (dentro de RNF01)
- Stack / linguagens de cada componente
- Formato dos artefatos: requisição OT, resposta OT, tokens, livro cifrado

Quando uma for decidida, registre aqui (seção abaixo) com data e justificativa.

### Decisões tomadas

**D-001 — Execução do OT: síncrona na compra + semente salva (2026-09-27)**
- **Decisão:** o OT roda durante a compra, com o programa local aberto (≤ 1 min, RNF03). ~~A troca de mensagens site ↔ executável é por arquivos transportados pelo navegador, sem rede no executável (RF10).~~ Desde D-004, o programa local fala direto com o site. O programa gera o PRNG do receptor a partir de uma **semente** (≥ 128 bits, CSPRNG do sistema, uma por compra, nunca reutilizada) e salva **semente + escolhas** antes de enviar a requisição. Se a compra falhar depois do pagamento, o executável reexecuta o receptor com a mesma semente e usa a resposta do OT recebida por e-mail (RF14/RF15) para extrair as chaves.
- **Justificativa:** permite usar a biblioteca de OT sem alterar a matemática nem serializar o estado interno. Mantém RF11 (segredo guardado), RF14/RF15 e o invariante 6 (perder a semente = perder o acesso), e cobre falhas entre pagamento e entrega (RNF07).
- **Alternativas descartadas:** (A) síncrono puro, sem estado salvo: esvazia RF14/RF15 e arrisca "pagou e perdeu"; (C) adaptador que reorganiza o protocolo em duas funções com estado serializado: mais código criptográfico próprio.
- **Premissa:** toda a aleatoriedade do receptor vem do PRNG passado como parâmetro, então a reexecução com a mesma semente reproduz a mesma requisição. Conferida por leitura do código de `MasnyRindal::receive` (só usa o `prng` recebido); vira **teste automatizado obrigatório** no passo 6 (mesma semente → requisição byte a byte idêntica). Se falhar, voltar a esta decisão.
- **Consequências:** a semente é o segredo do cliente e precisa de proteção em disco (como proteger está em aberto). Porta localhost no programa local foi descartada.

**D-002 — Biblioteca de OT: libOTe, OT base Masny–Rindal [MR19] (2026-09-27, definitiva)**
- **Decisão:** usar o libOTe (C++, MIT) com o MR19 como OT base 1-de-2. O libOTe roda num binário C++ auxiliar (D-003), que recebe e devolve as mensagens do protocolo como bytes por um socket coproto próprio.
- **Justificativa:** biblioteca de OT mais completa e citada, testada em Windows (o programa local roda na máquina do cliente), e com vários OTs base (NP01, CO15, MR19, MRR20) para comparação no TCC. Mensagens do MR19: requisição do receptor, depois ponto do sender + cifras, numa única resposta.
- **Descartadas:** emp-ot (PVW encaixa melhor, mas não suporta Windows); circl `ot/simot` (só 1-de-2, sender fala primeiro); mpz e swanky (avisam "não usar em produção"). OT via OPRF (RFC 9497) foi descartado por fugir do foco do TCC, que é OT.
- **Continua em aberto:** a variante de OT (1-de-n repetido via Naor–Pinkas 1999 sobre 1-de-2, k-de-n ou GOT).
- **Sem spike (decisão do grupo, 2026-09-27):** as verificações viram testes automatizados na implementação (passo 6): build no Windows, reexecução por semente (D-001) e benchmark de N·log₂n OTs contra RNF03/RNF05. **Risco aceito:** se o libOTe não compilar no Windows ou a reexecução não for determinística, isso só aparece no passo 6.

**D-003 — Stack do site (2026-09-27)**
- **Linguagem:** Go. Motivo: RSA cego RFC 9474 pronto no `cloudflare/circl` (RF16), biblioteca padrão com HTTP, templates e criptografia, desempenho para RNF05, e binário único (a mesma linguagem serve para a CDN e para a parte não-C++ do programa local).
- **Banco:** PostgreSQL. Motivo: realista para o teste de carga (RNF05) e com constraints (FK) para impor RF03 no próprio banco.
- **Front:** HTML renderizado no servidor (templates Go), sem SPA e sem JS/fontes/analytics de terceiros. Motivo: menos superfície e nenhum terceiro vendo a navegação.
- **Busca (RF04):** o site entrega o **catálogo inteiro** num único pacote, igual para todos; o filtro roda no navegador. Motivo: o site não vê o que o cliente procura (lacuna L1). Garantia de engenharia, não formal.
- **Integração com o libOTe (D-002):** **binário C++ auxiliar** chamado pelo site (e pelo programa local), e não ligação direta via cgo. Motivo: isola o código C++ e deixa o site independente da forma como o libOTe é compilado.
- **Em aberto:** gateway de pagamento (RF13), a decidir antes do passo 4.
- **Complementos aprovados (2026-09-27):** um módulo Go por componente (`site/`, depois `cdn/` e o programa local); rotas com `net/http` da biblioteca padrão; driver `jackc/pgx` v5; migrações com `goose`; senhas com argon2id (`golang.org/x/crypto/argon2`); sessão de admin própria (token aleatório em cookie `HttpOnly`/`Secure`/`SameSite=Strict`, só o hash no banco); testes com `testing` contra Postgres real; Postgres de desenvolvimento via Docker Compose; identificadores e mensagens em português, citando requisitos em comentários e testes.

**D-004 — Programa local deixa de ser offline (2026-09-27)**
- **Decisão:** o "executável local offline" vira **programa local (cliente)** que pode usar rede, **só** para falar com o site e a CDN. Continua sendo um programa instalado, que **não é código servido pelo site**. Ele gera a requisição e o segredo, envia ao site, recebe a resposta, baixa os livros na CDN com os tokens e decifra localmente.
- **Justificativa:** usabilidade (RNF07): o cliente não precisa carregar arquivos entre navegador e programa. O invariante 1 continua **garantia formal**, porque o código do receptor do OT não vem do site.
- **Alternativas descartadas:** (B) receptor no navegador com código servido pelo site: o invariante 1 viraria premissa ("site serve código honesto"), exigiria libOTe em WebAssembly e a semente ficaria em armazenamento do navegador; (C) extensão/página estática de outra origem: mesmos problemas técnicos da B, premissa mais fraca, mas ainda premissa.
- **Diverge da especificação:** RF10 ("não se conecta à internet") e a descrição do RF09 ("executável local offline"). Registrado em `docs/divergencias_docs.md` (D3); a especificação e o PRD precisam ser atualizados.
- **Consequências:** o programa não faz telemetria, checagem de atualização nem fala com terceiros; chaves de livro e semente nunca saem da máquina. Canais laterais novos a tratar: o momento em que o programa baixa da CDN logo após a compra (timing, relevante para RF18/lotes) e o IP visto pela CDN (coberto pela premissa de não conivência, invariante 5). D-001 continua valendo (a semente protege contra falha entre pagamento e entrega). Como o pagamento no gateway se encaixa (programa abre o navegador ou paga por ele) fica para o passo 4.
## Escopo do MVP e ordem de implementação

MVP = compra privada ponta a ponta de um grupo de livros (todos os RFs essenciais). RF04 completo e RF15 podem vir depois.

Ordem (Fase 4 do método):
1. Catálogo e cadastro de livros (RF01–RF03)
2. Fluxo de compra com e-mail e gateway (RF05, RF06, RF13, RF14)
3. OT da escolha privada (RF07, RF08, RF11)
4. Tokens com assinatura cega e entrega pela CDN (RF16–RF18)
5. Programa local: segredo, extração da chave e descriptografia (RF10–RF12)

## Estrutura do repositório e comandos

Estado atual do projeto: `docs/estado_do_projeto.md` (atualizar a cada entrega).

Layout aprovado (D-003); o que ainda não existe está marcado:

```
TCC/
  docs/                    especificação (PDF v2 + fonte LaTeX v3 em docs/especificacao/), PRD, divergências, estado
  site/                    módulo Go tcc/site
    cmd/site/              servidor web + roteador
    cmd/admin-seed/        cria o primeiro admin (RF01)
    internal/admin/        contas, login, sessão (RF01)
    internal/catalogo/     livros, preços, grupos (RF02, RF03) (a criar)
    internal/vitrine/      páginas públicas (RF04)             (a criar)
    internal/db/           conexão e migrações (dbteste/: bancos de teste)
    web/                   templates/, static/ e renderização
    migrations/            SQL no formato goose
  cdn/  programa-local/  ot-helper/                           (depois)
  docker-compose.yml       Postgres de desenvolvimento
```

Comandos (na raiz sobe o Postgres; o resto dentro de `site/`):

```bash
docker compose up -d                                   # Postgres de desenvolvimento (localhost:5432)
go run ./cmd/admin-seed -nome "Nome" -email a@b.com    # primeiro admin (senha pela entrada padrão ou ADMIN_SENHA)
go run ./cmd/site                                      # site em http://localhost:8080 (migrações aplicadas na subida)
go test ./...                                          # testes; os de integração são pulados sem Postgres
go vet ./... && gofmt -l .                             # checagens
```

Variáveis: `DATABASE_URL`, `ENDERECO` (site); `TEST_DATABASE_URL` (testes; cada teste usa um schema próprio, apagado no fim).
Convenções do site: sem log de acesso (só erros, sem dados de visitante); CSP `default-src 'self'`; CSRF via `http.CrossOriginProtection`; avisos após redirect vão por chave fixa na URL, nunca com dados.
