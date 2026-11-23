#!/bin/bash
# Bateria de testes controlada, de ponta a ponta (Git Bash no Windows ou Linux).
#
# Sobe um ambiente isolado (banco "livraria_bateria", site em :18080, CDN em
# :18081, segredo aleatório, pastas temporárias), cadastra um catálogo
# conhecido pelo admin e exercita o programa local (CLI). Cada caso cita o
# requisito; o resultado vai para a tela e para bateria.log.
#
# Pré-requisitos: build_app.bat (executáveis em bin\), Postgres do
# docker-compose no ar e a chave do pacote em site/chave_pacote.ed25519 (a
# mesma embutida no livraria.exe).
#
# Uso: bash "Claude outputs/bateria.sh" [--sem-go-test]

set -u
RAIZ=$(cd "$(dirname "$0")/.." && pwd)
BIN=$RAIZ/bin
SAIDA="$RAIZ/Claude outputs/bateria.log"
TMP=$(mktemp -d)
PS=18080
PC=18081
SITE=http://localhost:$PS
CDN=http://localhost:$PC
BANCO=livraria_bateria
URL_BANCO="postgres://livraria:livraria_dev@localhost:5432/$BANCO?sslmode=disable"
SEG=$(head -c 24 /dev/urandom | base64 | tr '+/' '-_')
SENHA=$(head -c 18 /dev/urandom | base64 | tr -d '+/=')
EXE=""
[ -f "$BIN/site.exe" ] && EXE=.exe
L="$BIN/livraria$EXE"
JAR=$TMP/cookies
PASSOU=0
FALHOU=0
FALHAS=""
PIDS=""

exec > >(tee "$SAIDA") 2>&1

sql() { docker exec -i tcc-postgres-1 psql -U livraria -d "$BANCO" -Atc "$1"; }
ms() { date +%s%3N; }

caso() { # id requisito descrição -- comando...
	local id=$1 req=$2 desc=$3
	shift 4
	echo "--- $id" >>"$TMP/casos.log"
	if (set -x; "$@") >>"$TMP/casos.log" 2>&1; then
		PASSOU=$((PASSOU + 1))
		printf 'PASSOU  %-4s %-16s %s\n' "$id" "$req" "$desc"
	else
		FALHOU=$((FALHOU + 1))
		FALHAS="$FALHAS $id"
		printf 'FALHOU  %-4s %-16s %s\n' "$id" "$req" "$desc"
	fi
}

encerrar() {
	for p in $PIDS; do
		w=$(cat /proc/$p/winpid 2>/dev/null)
		kill "$p" 2>/dev/null
		[ -n "$w" ] && taskkill //F //PID "$w" >/dev/null 2>&1
	done
	sleep 1
	cp "$TMP/casos.log" "$DETALHE" 2>/dev/null
	docker exec tcc-postgres-1 psql -U livraria -d livraria -qc "DROP DATABASE IF EXISTS $BANCO WITH (FORCE)" >/dev/null 2>&1
	rm -rf "$TMP"
}
trap encerrar EXIT
DETALHE="$RAIZ/Claude outputs/bateria-detalhe.log"

esperar() { # url
	for _ in $(seq 60); do
		curl -s -o /dev/null "$1" && return 0
		sleep 0.5
	done
	echo "não subiu: $1"
	exit 1
}

echo "=== Bateria de testes controlada — $(date '+%Y-%m-%d %H:%M')"
echo "=== Ambiente: $(uname -s), $(nproc) núcleos; banco $BANCO; site $SITE; CDN $CDN"

# --- Ambiente isolado ------------------------------------------------------
docker exec tcc-postgres-1 psql -U livraria -d livraria -qc "DROP DATABASE IF EXISTS $BANCO WITH (FORCE)" >/dev/null
docker exec tcc-postgres-1 psql -U livraria -d livraria -qc "CREATE DATABASE $BANCO" >/dev/null || exit 1
mkdir -p "$TMP/caixa" "$TMP/livros" "$TMP/arquivos"

(cd "$RAIZ/site" && DATABASE_URL=$URL_BANCO ENDERECO=localhost:$PS CHAVE_PACOTE="$RAIZ/site/chave_pacote.ed25519" \
	OT_HELPER="$BIN/ot-helper$EXE" CDN_URL=$CDN SEGREDO_CDN=$SEG CAIXA_SAIDA="$TMP/caixa" \
	exec "$BIN/site$EXE") >"$TMP/site.log" 2>&1 &
PIDS="$PIDS $!"
esperar $SITE/
(cd "$RAIZ/cdn" && DATABASE_URL=$URL_BANCO ENDERECO=localhost:$PC SEGREDO_CDN=$SEG \
	CHAVE_PACOTE_PUB=$(cat "$BIN/chave_pacote.pub") PASTA_LIVROS="$TMP/livros" SITE_URL=$SITE \
	LOTE_MINIMO=6 INTERVALO_LOTE=24h exec "$BIN/cdn$EXE") >"$TMP/cdn.log" 2>&1 &
PIDS="$PIDS $!"
esperar $CDN/

# --- Catálogo conhecido, cadastrado pelo admin ------------------------------
DATABASE_URL=$URL_BANCO ADMIN_SENHA=$SENHA "$BIN/admin-seed$EXE" -nome Bateria -email bateria@livraria.test >/dev/null
post() { curl -s -c "$JAR" -b "$JAR" -o /dev/null -w '%{http_code}' "$@"; }
post -d email=bateria@livraria.test --data-urlencode "senha=$SENHA" $SITE/admin/entrar >/dev/null
post -d valor=19,90 $SITE/admin/precos >/dev/null
post -d valor=39,90 $SITE/admin/precos >/dev/null
post -d nome=Romance $SITE/admin/categorias >/dev/null
FA=$(sql "SELECT id FROM faixas_preco WHERE valor_centavos=1990")
FB=$(sql "SELECT id FROM faixas_preco WHERE valor_centavos=3990")
CAT=$(sql "SELECT id FROM categorias WHERE nome='Romance'")
livro() { # titulo autor faixa
	post --data-urlencode "titulo=$1" --data-urlencode "autor=$2" -d ano=1900 -d categoria_id=$CAT -d faixa_id=$3 \
		--data-urlencode "descricao=Livro da bateria." $SITE/admin/livros >/dev/null
	local id
	id=$(sql "SELECT max(id) FROM livros")
	printf 'Conteúdo do livro %s (%s), id %s.\n' "$1" "$2" "$id" >"$TMP/arquivos/$id.txt"
	post -F "arquivo=@$TMP/arquivos/$id.txt" $SITE/admin/livros/$id/arquivo >/dev/null
}
livro "Alfa" "Autora Um" $FA
livro "Beta" "Autora Um" $FA
livro "Gama" "Autor Dois" $FA
livro "Delta" "Autor Dois" $FA
livro "Epsilon" "Autor Tres" $FA
livro "Zeta" "Autor Tres" $FA
livro "Eta" "Autor Quatro" $FB
livro "Teta" "Autor Quatro" $FB
mapfile -t A < <(sql "SELECT id FROM livros WHERE faixa_id=$FA ORDER BY id")
mapfile -t B < <(sql "SELECT id FROM livros WHERE faixa_id=$FB ORDER BY id")
echo "=== Catálogo: ${#A[@]} livros a R\$ 19,90 (ids ${A[*]}), ${#B[@]} a R\$ 39,90 (ids ${B[*]})"
echo

export SITE_URL=$SITE CDN_URL=$CDN OT_HELPER="$BIN/ot-helper$EXE"
cli() { LIVRARIA_DADOS="$TMP/$1" "$L" "${@:2}"; }

# --- Casos ------------------------------------------------------------------
t_admin_exige_login() { [ "$(curl -s -o /dev/null -w '%{http_code}' $SITE/admin/)" = 303 ]; }
caso T01 RF01 "área admin exige login" -- t_admin_exige_login

t_csrf() { [ "$(post -H 'Sec-Fetch-Site: cross-site' -d nome=X $SITE/admin/categorias)" = 403 ]; }
caso T02 RF01 "POST de outra origem recusado (CSRF)" -- t_csrf

t_utf8() { [ "$(post -H 'Content-Type: application/x-www-form-urlencoded' --data-binary 'nome=Fic%E7%E3o' $SITE/admin/categorias)" = 422 ]; }
caso T03 RF02 "formulário fora de UTF-8 recusado (422)" -- t_utf8

t_pagina_publica() {
	local h
	h=$(curl -s -D - -o "$TMP/inicio.html" $SITE/)
	! grep -qi '^set-cookie' <<<"$h" && grep -qi "content-security-policy: default-src 'self'" <<<"$h" &&
		! grep -Eqi '<(script|link|img)[^>]+(src|href)="https?://' "$TMP/inicio.html"
}
caso T04 D-006 "página pública sem cookies nem recursos externos" -- t_pagina_publica

t_catalogo() { cli c1 catalogo | grep -q "8 livros, 2 grupos (assinatura conferida)"; }
caso T05 "RF04 D-007" "pacote baixado com assinatura Ed25519 conferida" -- t_catalogo

t_mesmo_pacote() {
	local h1 h2
	h1=$(curl -s $SITE/catalogo.zip | sha256sum | cut -c1-64)
	h2=$(curl -s $SITE/catalogo/hash | grep -oE '[0-9a-f]{64}' | head -1)
	[ "$h1" = "$h2" ]
}
caso T06 D-007 "pacote igual ao hash público" -- t_mesmo_pacote

t_busca() {
	local t0 t1
	t0=$(ms)
	cli c1 buscar -autor "Autor Dois" >"$TMP/busca.txt"
	t1=$(ms)
	echo "busca: $((t1 - t0)) ms" >>"$TMP/medidas"
	[ "$(grep -cE 'Gama|Delta' "$TMP/busca.txt")" = 2 ] && ! grep -q Alfa "$TMP/busca.txt" && [ $((t1 - t0)) -lt 5000 ]
}
caso T07 "RF04 RNF02" "busca local por autor correta e < 5 s" -- t_busca

t_grupo_pequeno() { ! cli c1 comprar -email a@livraria.test -livros ${B[0]} && [ "$(sql 'SELECT count(*) FROM compras')" = 0 ]; }
caso T08 D-010 "compra em grupo com < 5 livros recusada" -- t_grupo_pequeno

t_grupos_mistos() { ! cli c1 comprar -email a@livraria.test -livros ${A[0]},${B[0]}; }
caso T09 RF07 "livros de grupos diferentes recusados" -- t_grupos_mistos

t_recusado() {
	! cli c9 comprar -email a@livraria.test -livros ${A[0]} -pagamento recusar &&
		[ "$(sql 'SELECT count(*) FROM compras')" = 0 ] && [ -z "$(ls "$TMP/caixa")" ]
}
caso T10 RF13 "pagamento recusado: nada gravado, nenhum e-mail" -- t_recusado

t_compra() {
	local t0 t1
	t0=$(ms)
	cli c1 comprar -email cliente@livraria.test -livros ${A[1]},${A[4]} >"$TMP/compra1.txt" || return 1
	t1=$(ms)
	echo "compra completa (OT, tokens, CDN, decifragem), N=2, n=6: $((t1 - t0)) ms" >>"$TMP/medidas"
	cmp -s "$TMP/c1/biblioteca/${A[1]} - Beta.txt" "$TMP/arquivos/${A[1]}.txt" &&
		cmp -s "$TMP/c1/biblioteca/${A[4]} - Epsilon.txt" "$TMP/arquivos/${A[4]}.txt" &&
		[ "$(ls "$TMP/c1/biblioteca" | wc -l)" = 2 ] && [ $((t1 - t0)) -lt 60000 ]
}
caso T11 "RF05–RF12 RNF03" "compra de 2 livros decifrados iguais ao original, < 1 min" -- t_compra
C1=$(grep -oE '[0-9a-f]{32}' "$TMP/compra1.txt" | head -1)

t_cobranca() { [ "$(sql "SELECT quantidade||'|'||valor_centavos FROM compras")" = "2|3980" ]; }
caso T12 "RF13 inv.4" "cobrança = preço da faixa × N" -- t_cobranca

t_email() {
	local eml
	eml=$(ls "$TMP/caixa"/*.eml | head -1)
	[ -n "$eml" ] && grep -q resposta-ot.json "$eml" || return 1
	python -I -c "
import email, sys
m = email.message_from_binary_file(open(sys.argv[1], 'rb'))
for p in m.walk():
    if p.get_filename() == 'resposta-ot.json':
        open(sys.argv[2], 'wb').write(p.get_payload(decode=True))
" "$eml" "$TMP/resp1.json" || return 1
	# Nenhuma chave de livro (hex ou base64) no e-mail.
	for k in $(sql "SELECT encode(chave,'hex') FROM chaves_livros") $(sql "SELECT encode(chave,'base64') FROM chaves_livros"); do
		grep -qF "$k" "$eml" "$TMP/resp1.json" && return 1
	done
	return 0
}
caso T13 "RF14 inv.2" "e-mail leva a resposta do OT e nenhuma chave" -- t_email

t_reenvio() { cli c1 reenviar "$C1" && [ "$(ls "$TMP/caixa"/*.eml | wc -l)" = 2 ]; }
caso T14 RF15 "reenvio da resposta ao e-mail original" -- t_reenvio

t_privacidade_tamanhos() {
	cli c2 comprar -email outro@livraria.test -livros ${A[0]},${A[5]} >/dev/null || return 1
	[ "$(sql 'SELECT count(DISTINCT octet_length(resposta_ot)) FROM compras')" = 1 ] &&
		[ "$(sql 'SELECT count(DISTINCT (quantidade, valor_centavos, versao_grupo_id)) FROM compras')" = 1 ]
}
caso T15 "RF07 inv.1" "escolhas diferentes: registros e respostas de mesmo tamanho" -- t_privacidade_tamanhos

t_colunas() {
	[ "$(sql "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_schema='public' AND table_name='compras'")" = \
		"criada_em,email,id,pagamento_id,quantidade,reenvio_ate,resposta_ot,valor_centavos,versao_grupo_id" ]
}
caso T16 "RF08 RF16" "compras sem índices, chaves ou tokens" -- t_colunas

t_logs() {
	for t in Beta Epsilon Alfa Zeta cliente@livraria.test; do
		grep -qF "$t" "$TMP/site.log" "$TMP/cdn.log" && return 1
	done
	return 0
}
caso T17 "inv.1 inv.3" "logs do site e da CDN sem livro nem e-mail" -- t_logs

t_recuperar() {
	cli c3 comprar -email rec@livraria.test -livros ${A[2]},${A[3]} -adiar >/dev/null || return 1
	local f eml
	f=$(ls "$TMP/c3/compras"/*.json)
	python -I -c "
import json, sys
d = json.load(open(sys.argv[1])); d.pop('resposta', None); json.dump(d, open(sys.argv[1], 'w'))
" "$f"
	eml=$(grep -l "rec@livraria.test" "$TMP/caixa"/*.eml | head -1)
	python -I -c "
import email, sys
m = email.message_from_binary_file(open(sys.argv[1], 'rb'))
for p in m.walk():
    if p.get_filename() == 'resposta-ot.json':
        open(sys.argv[2], 'wb').write(p.get_payload(decode=True))
" "$eml" "$TMP/resp3.json"
	cli c3 recuperar "$TMP/resp3.json" >/dev/null &&
		cmp -s "$TMP/c3/biblioteca/${A[2]} - Gama.txt" "$TMP/arquivos/${A[2]}.txt" &&
		cmp -s "$TMP/c3/biblioteca/${A[3]} - Delta.txt" "$TMP/arquivos/${A[3]}.txt"
}
caso T18 "D-001 RF11 RF14" "compra interrompida recuperada pela semente + e-mail" -- t_recuperar

t_sem_semente() {
	mkdir -p "$TMP/c4"
	! cli c4 recuperar "$TMP/resp3.json"
}
caso T19 "RF11 inv.6" "sem a semente local, a resposta do e-mail não abre nada" -- t_sem_semente

t_token_unico() {
	mkdir -p "$TMP/c5/compras"
	local f
	f=$(ls "$TMP/c1/compras"/*.json)
	python -I -c "
import json, sys
d = json.load(open(sys.argv[1])); d.pop('baixados', None); json.dump(d, open(sys.argv[2], 'w'))
" "$f" "$TMP/c5/compras/$(basename "$f")"
	cp "$TMP/c1/catalogo".* "$TMP/c5/" 2>/dev/null
	cli c5 baixar 2>&1 | grep -q "token já usado"
}
caso T20 RF17 "token usado não baixa de novo" -- t_token_unico

t_token_forjado() {
	[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Token AAAA' $CDN/livros/${A[0]})" = 403 ]
}
caso T21 RF17 "token inválido recusado pela CDN" -- t_token_forjado

t_reporte() {
	# 6 tokens usados (3 compras × 2) = LOTE_MINIMO; a CDN verifica a cada minuto.
	for _ in $(seq 80); do
		[ "$(sql 'SELECT coalesce(sum(quantidade),0) FROM lotes_cdn')" = 6 ] && break
		sleep 1
	done
	[ "$(sql 'SELECT coalesce(sum(quantidade),0) FROM lotes_cdn')" = 6 ] &&
		[ "$(sql 'SELECT count(*) FROM tokens_reportados')" = 6 ] &&
		[ "$(sql "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_name='tokens_reportados'")" = "faixa_id,resumo" ] &&
		[ "$(sql "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_name='tokens_usados'")" = "reportado,resumo,token" ]
}
caso T22 "RF18 D-011" "reporte em lote: só tokens e quantidade, sem livro nem horário" -- t_reporte

t_remuneracao() { [ "$(sql 'SELECT sum(valor_centavos) FROM lotes_cdn')" = 300 ]; }
caso T23 D-014 "remuneração da CDN = tokens × tarifa" -- t_remuneracao

t_reporte_forjado() {
	[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer errado' -H 'Content-Type: application/json' \
		-d '{"tokens":[],"quantidade":0}' $SITE/api/cdn/reporte)" != 200 ]
}
caso T24 RF18 "reporte sem o segredo da CDN recusado" -- t_reporte_forjado

echo
echo "=== Medidas"
cat "$TMP/medidas"

if [ "${1:-}" != "--sem-go-test" ]; then
	echo
	echo "=== Testes automatizados (go test) dos 4 módulos"
	for m in protocolo site cdn programa-local; do
		(cd "$RAIZ/$m" && OT_HELPER="$BIN/ot-helper$EXE" go test -count=1 -v ./... 2>&1) >"$TMP/go-$m.log"
		printf '%-15s passou %3d  falhou %2d  pulado %2d\n' "$m" \
			"$(grep -c -- '--- PASS' "$TMP/go-$m.log")" "$(grep -c -- '--- FAIL' "$TMP/go-$m.log")" "$(grep -c -- '--- SKIP' "$TMP/go-$m.log")"
		grep -q -- '--- FAIL' "$TMP/go-$m.log" && FALHOU=$((FALHOU + 1)) && FALHAS="$FALHAS go:$m"
	done
fi

echo
echo "=== Resultado: $PASSOU casos passaram, $FALHOU falharam${FALHAS:+ (${FALHAS# })}"
[ "$FALHOU" = 0 ]
