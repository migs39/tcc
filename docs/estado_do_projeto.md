# Estado do projeto — Livraria Digital com OT

Atualizado em: 2026-11-22 (app completo: site, CDN e programa local)

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
| D-001 | Execução do OT | Síncrona na compra; a semente do PRNG do receptor fica salva e permite reextrair as chaves a partir da resposta do e-mail | Decidida e implementada; determinismo testado; recuperação pelo e-mail testada ponta a ponta |
| D-002 | Biblioteca de OT | libOTe (C++, MIT), OT base Masny–Rindal [MR19], via binário C++ auxiliar | Decidida e implementada (`ot-helper/`); compila no Windows (MSVC) e no Linux |
| D-003 | Stack do site | Go + PostgreSQL + HTML renderizado no servidor; catálogo inteiro com filtro no navegador; libOTe via binário C++ auxiliar | Decidida |
| D-003 (compl.) | Ferramentas | Um módulo Go por componente; `net/http` padrão; `pgx` v5; migrações `goose`; argon2id; sessão de admin própria; Postgres via Docker Compose; identificadores em português | Decidida |
| D-004 | Programa local | Deixa de ser offline; usa rede só com o site e a CDN | Decidida; especificação v3 atualizada, PRD pendente |
| D-005 | Modelo do catálogo | Livro completo com capa (Postgres, 2 MB); faixas de preço editáveis e imutáveis; categorias controladas; nova versão de grupo só quando a composição muda | Decidida e implementada (passo 2) |
| D-006 | Navegação e busca | No programa local, sobre o pacote completo do catálogo; site sem vitrine pública; miniaturas 200×300 JPEG | Decidida; miniaturas implementadas, pacote no passo 3 |
| D-007 | Formato do pacote | ZIP único determinístico (JSON + miniaturas), pacote completo com ETag, assinatura Ed25519 + hash público, versão só cresce | Decidida e implementada (passo 3) |
| D-008 | Variante de OT | 1-de-n de Naor–Pinkas repetido N vezes, sobre os OTs 1-de-2 do MR19 | Decidida e implementada |
| D-009 | Simétrico | AES-256-GCM, chave por livro, ID do livro como dado associado | Decidida e implementada |
| D-010 | Grupo, reenvio, pagamento | Grupo mínimo de 5; reenvio em 30 dias; gateway mock; o programa local intermedeia o pagamento | Decidida e implementada |
| D-011 | Tokens e reporte | Uso único; RSA-3072 RFC 9474 com uma chave por faixa, publicada no pacote assinado; reporte em lotes de 50 ou diário, embaralhado | Decidida e implementada |
| D-012 | Formatos e programa local | JSON + base64; o e-mail leva a resposta do OT e as assinaturas cegas; CLI em Go; semente em arquivo só do usuário | Decidida e implementada |
| D-013 | Código comum | Módulo `protocolo/` usado pelo site, pela CDN e pelo programa local; toda a aleatoriedade do cliente vem da semente | Decidida e implementada |

**Descartados, com motivo registrado no CLAUDE.md:**
- emp-ot (não suporta Windows);
- circl `simot`;
- mpz e swanky ("não usar em produção");
- OT via OPRF (foge do foco do TCC);
- receptor do OT no navegador (o invariante 1 viraria premissa).

---

## 3. Decisões em aberto

Nenhuma. Os pontos que restavam foram decididos na D-014 (2026-11-22): remuneração por tarifa fixa por token, semente sem senha, comportamento uniforme contra falha seletiva, gateway mock com SMTP opcional, sem atraso no download e janela WebView2. O tema visual foi escolhido em 2026-11-23: estilo 2, "Cofre criptográfico".

Pendência de implementação: integração com um gateway de pagamento real, que exige conta de lojista.

---

## 4. Plano do site e situação

| # | Entrega | Requisitos | Situação |
|---|---|---|---|
| 0 | Esqueleto: repositório, módulo Go, Postgres (Docker), migrações, testes | — | **Concluído**: `docker-compose.yml` (Postgres 17), migrações goose embutidas e aplicadas na subida, bancos de teste isolados por schema |
| 1 | Admins: cadastro, login, gestão; primeiro admin por comando de seed | RF01, UC01 | **Concluído**: argon2id, sessão com hash do token, desativar/reativar (nunca o último ativo), troca de senha encerra sessões, CSRF, CSP. 13 testes passando (9 de integração contra Postgres) |
| 2 | Catálogo: CRUD de livros, preços discretos, categorias, capas, grupos versionados | RF02, RF03, UC02, UC03 | **Concluído** (D-005): telas de livros, preços e categorias; capa no Postgres; RF03 imposto também pelo banco; versões de grupo imutáveis. miniatura 200×300 para o pacote (D-006); original só na área admin. 24 testes passando no site |
| 3 | Pacote do catálogo: ZIP único e determinístico (catalogo.json + miniaturas), igual para todos, assinado (Ed25519) com hash público; a busca roda no programa local (D-006, D-007) | RF04, RNF02, UC04 | **Concluído**: `GET /catalogo.zip` (ETag, 304) e `GET /catalogo/hash`; versão nova só quando o conteúdo muda. Página pública de apresentação e download em `/` (mostra versão, hash e chave pública do catálogo; sem cookies nem terceiros). 29 testes passando no site |
| 4 | Checkout sem conta: e-mail, gateway, aprovado/recusado | RF05, RF06, RF13 | **Concluído**: `POST /api/compras`; gateway mock atrás da interface `Gateway`; recusado não grava nada |
| 5 | E-mail com a resposta do OT (stub), assíncrono | RF14, RNF04 | **Concluído**: fila com novas tentativas; `.eml` em `CAIXA_SAIDA` com `resposta-ot.json` (sem chave) |
| 6 | OT real no lado sender (ot-helper com libOTe/MR19) + Naor–Pinkas | RF07, RF08 | **Concluído**: compila no Windows e no Linux; determinismo por semente testado; 1-de-n de Naor–Pinkas em `protocolo/` (D-008) |
| 7 | Assinatura cega (RSA-3072, RFC 9474, circl) | RF16 | **Concluído**: chave por faixa, a pública vai no pacote assinado (D-011) |
| 8 | Recebimento e validação do reporte da CDN | RF18, UC17 | **Concluído**: `POST /api/cdn/reporte` com segredo compartilhado; confere assinaturas, recusa repetidos, registra quantidade por lote |
| 9 | Reenvio da resposta do OT | RF15 | **Concluído**: `POST /api/compras/{id}/reenvio`, 30 dias, só para o e-mail original; resposta igual exista ou não a compra |
| 10 | Testes de privacidade e carga | RNF02, RNF03, RNF05 | **Parcial**: testes de privacidade do fluxo (tamanhos e registros independentes da escolha; e-mail sem chave; colunas das tabelas) e benchmarks da compra no site, sequencial e concorrente (`BenchmarkCompraConcorrente`): 1.213 compras/min com 4 núcleos, contra 600/min em RNF05 |

**CDN** (`cdn/`, **concluída**): guarda os livros cifrados enviados pelo site, entrega um livro por token válido da faixa do livro e ainda não usado, e reporta em lotes embaralhados, sem horário nem livro (D-011). Lê a faixa de cada livro e as chaves públicas do pacote assinado do catálogo.

**Interface (2026-11-23):** tema "Cofre" comum (`protocolo/tema`).
- **Site:** página pública e área administrativa redesenhadas; nova página `/admin/cdn` com os lotes e o valor a pagar.
- **CDN:** novo `/painel`.
- **Programa local:** janela gráfica `Livraria OT.exe` (WebView2) com catálogo e busca, compra privada, compras (baixar, reenviar e-mail, recuperar pelo anexo) e biblioteca (abrir livros).

**Programa local** (`programa-local/`, CLI `livraria`, **concluído**): sincroniza e confere o catálogo, faz a busca local, grava a semente antes de enviar, cega os tokens, compra, baixa da CDN, extrai as chaves e decifra. Também recupera uma compra a partir do e-mail (D-001) e pede o reenvio.

**Medições (2026-11-22, Linux, 2 núcleos):**
- Uma compra no site com n = 100 livros e N = 5 leva cerca de 58 ms. Isso inclui o sender do OT, as cifras de Naor–Pinkas, a cobrança mock, 5 assinaturas RSA-3072 e a gravação. São cerca de 1.000 compras/min num núcleo, contra 600/min em RNF05.
- A requisição do OT mede 1.256 bytes para 19 OTs e a resposta do sender, 64 bytes.

**Medições (2026-11-22, Windows 11, i7-12700H, Postgres no Docker Desktop):** compra com n = 100 e N = 5, 300 compras por rodada, requisições geradas antes do cronômetro.

| Núcleos (`-cpu`) | Sequencial (ms/compra) | Concorrente (compras/min) |
|---|---|---|
| 1 | 238 | 396 |
| 4 | — | 1.213 |
| 8 | — | 1.702 |

- RNF05 (≥ 600/min) é atendido a partir de 4 núcleos; com 1 núcleo, não. RNF03 (≤ 1 min) é atendido com folga.
- Por compra, no perfil de CPU: ≈ 89 ms nas 5 assinaturas cegas RSA-3072 (o `circl` exponencia com `math/big`, ≈ 18 ms cada) e ≈ 57 ms no `ot-helper` (processo, sender e pipes). O resto é espera fora da CPU, provavelmente o banco atrás do Docker Desktop (estimativa, sem medição separada).
- Comando: `go test -run x -bench Compra -benchtime 300x -cpu 1,4,8 ./internal/compra` (em `site/`, com `OT_HELPER` absoluto).

**Bateria controlada (2026-11-22, Windows):** `Claude outputs/bateria.sh` sobe um ambiente isolado (banco próprio, portas 18080/18081, segredo aleatório), cadastra 8 livros pelo admin e roda 24 casos de ponta a ponta com a CLI, mais o `go test` dos 4 módulos (48 testes). Resultado: 24/24 e 48/48. Compra completa em 285 ms e busca em 85 ms. Detalhes na seção 5.3 da monografia.

**Acesso externo (D-015):** Tailscale instalado e logado; falta ativar o Funnel na tailnet (link no console do Tailscale) e rodar `abrir_acesso_publico.bat`. Depois, `set PUBLICO=https://<máquina>.<tailnet>.ts.net` + `build_app.bat` geram `bin\LivrariaOT-cliente.zip` para distribuir.

**Teste ponta a ponta no Windows (2026-11-22):** `build_app.bat` + `iniciar_demo.bat`; testes dos 4 módulos sem nenhum pulado. A CLI e a janela compraram 2 livros cada e os decifraram; a recuperação pelo anexo do e-mail funcionou numa compra interrompida.

**Teste ponta a ponta (2026-11-22, Linux):** um banco novo, o site, a CDN e o programa local. O admin cadastra 7 livros e envia os arquivos. O cliente compra 2 livros do grupo de 6 e os decifra. As compras em grupo com menos de 5 livros, com livros de grupos diferentes e com pagamento recusado são recusadas. Uma compra com o estado local truncado é recuperada pelo e-mail. A CDN reportou 3 tokens num lote e o site os aceitou.



---

## 5. Ambiente de desenvolvimento

| Ferramenta | Situação na máquina de desenvolvimento |
|---|---|
| Go | Instalado (1.27) |
| Git | Instalado |
| Docker Desktop | Funcionando (WSL instalado); Postgres sobe com `docker compose up -d` |
| CMake + compilador C++ | Build Tools 2026 (MSVC 19.52, CMake 4.3, Ninja); libOTe instalado em `ot-helper/deps/install` |
| LaTeX (MiKTeX) | Instalado; a especificação v3 compila sem erros |

---

## 6. Pendências de documentação

- ~~Atualizar a especificação~~ Feito: especificação **v3** (`docs/Especificacao_Detalhada_v3.pdf`, fonte em `docs/especificacao/`). Falta a lacuna L4 (política de chaves dos tokens), que depende de decisão.
- **Monografia no modelo PCS (2026-11-22):** a especificação v3 foi migrada para `docs/monografia/` (modelo TCC-PCS-EPUSP, sumário recomendado pelo PCS, pré-textuais, fontes em figuras e tabelas, siglas). Regras em `CLAUDE.md`, seção "Monografia". Falta:
  - revisar o resumo e o abstract (rascunhos) e completá-los com os resultados;
  - ficha catalográfica oficial (site da biblioteca da Poli);
  - Cap. 6 (Considerações Finais) e o plano de testes da 5.3: só depois de o app estar concluído e testado (o `Cap6-Consideracoes.tex` existe, mas está fora do `main.tex`).
- Atualizar o PRD (documento de outra pessoa do grupo, não refeito): lista do que mudar em `docs/prd_alteracoes.md`.
- Corrigir no PRD as divergências D1 (gestão de admins) e D2 (UC17 sem RF).
- Levar ao orientador: a escolha do libOTe/MR19 e a construção 1-de-n sobre 1-de-2.
