# Roteiro — apresentação de avanços (5 min)

Slides: https://claude.ai/artifact/VXvoMrbmPYVnv74LEsXGSd
Duração total: cerca de 4 min 50 s, com 7 slides.

**Combinar antes:** quem apresenta cada bloco. Sugestão: uma pessoa nos slides 1–3 (projeto), outra nos 4–5 (privacidade e página), outra nos 6–7 (implementação e próximos passos).

**Ponto de partida:** a apresentação anterior já cobriu as escolhas de programas, bibliotecas e arquitetura. Esta mostra só o que veio depois.

---

## 1. Capa — 15 s · [quem: ______]

> "Na última apresentação mostramos as escolhas de programas, bibliotecas e arquitetura. Hoje vamos mostrar o que mudou no projeto desde então e a primeira parte do sistema, que já está rodando."

## 2. Onde estamos — 35 s · [quem: ______]

> "Das seis fases do método, as duas primeiras estão concluídas, e a especificação foi revisada para a versão 3. Na fase de projeto, somamos três decisões novas às que já tínhamos apresentado. E começamos a implementação pelo site: quatro dos dez passos estão prontos, com 29 testes automatizados rodando."

**Destacar:** as fases 3 e 4 são o avanço desde a última vez.

## 3. O que mudou no projeto — 60 s · [quem: ______]

> "Três decisões novas. A principal mexe na arquitetura: **a navegação no catálogo saiu do site e foi para o programa local**. O programa baixa o catálogo inteiro e faz a busca no computador do cliente. Assim o site não sabe nem o que a pessoa procurou, e não existe vitrine pública.
>
> A segunda é sobre o catálogo. O preço de uma faixa não muda depois de criado, e cada mudança na composição de um grupo gera uma versão nova e imutável. Isso garante que o índice usado no OT não muda no meio de uma compra.
>
> A terceira: o catálogo é publicado como um pacote único, igual para todos, assinado e com hash público. Dá para provar que ninguém recebeu um catálogo diferente."

## 4. O que o site consegue saber — 50 s · [quem: ______]

> "Este é o resumo do que importa do ponto de vista de privacidade. O site **não sabe quais livros foram escolhidos**, e isso é garantia do protocolo de OT. Também não sabe **o que o cliente procurou**, porque a busca é local e o site não guarda log de acesso. Mas isso é escolha de engenharia, não prova formal. **Se todos viram o mesmo catálogo** passa a ser verificável, pelo pacote assinado. O que o site sabe é o necessário para cobrar: e-mail, pagamento, grupo e quantidade.
>
> Em todo o trabalho separamos garantia formal, premissa, como a não conivência entre site e CDN, e escolha de engenharia."

## 5. A página pública — 45 s · [quem: ______]

> "Esta é a página pública do site, rodando. Ela explica a proposta e como funciona, e mostra a versão, o hash e a chave pública do catálogo atual, para qualquer pessoa conferir. A página não usa cookies, scripts nem nada de terceiros, e todos os visitantes recebem exatamente a mesma página. O botão de download indica que o programa local ainda está em desenvolvimento."

**Observação:** o catálogo mostrado é de demonstração, com clássicos em domínio público.

## 6. O que já roda — 45 s · [quem: ______]

> "No site, temos a base com banco e testes, a área administrativa com login e gestão de contas, o catálogo com livros, capas, faixas de preço e grupos versionados, e o pacote assinado. São 29 testes automatizados. Alguns testam diretamente as propriedades de privacidade: o banco recusa um preço fora do conjunto, e dois visitantes recebem exatamente o mesmo pacote.
>
> E sendo transparentes: **o programa local, a CDN e o OT em si ainda não começaram.**"

## 7. Próximos passos — 40 s · [quem: ______]

> "A seguir: a compra sem conta com pagamento, o OT com o libOTe, os tokens com assinatura cega, a CDN e o programa local. Em paralelo, temos decisões a tomar, a principal é a variante de OT. Também queremos validar com o orientador a escolha do libOTe e a construção de 1-de-n a partir de 1-de-2."

---

## Perguntas prováveis

**"Se a busca é local, o site não descobre o interesse pelo download?"**
Não: todo mundo baixa o mesmo pacote inteiro, com a mesma requisição. Não há download por livro nem atualização parcial; esta última revelaria quando o cliente sincronizou pela última vez.

**"O site não pode mandar um catálogo diferente para uma pessoa e descobrir a escolha?"**
É o ataque de falha seletiva. O pacote assinado e o hash público permitem detectar um catálogo personalizado. No passo do OT vamos incluir no pacote compromissos das chaves dos livros, para o programa conferir as chaves que extraiu.

**"Por que o programa local deixou de ser offline?"**
Por usabilidade: o cliente não precisa levar arquivos entre o navegador e o programa. A privacidade da escolha continua sendo garantia do protocolo, porque o código que monta a requisição do OT não vem do site. O programa só conversa com o site e com a CDN.

**"E se o libOTe não compilar no Windows?"**
É um risco aceito e registrado. O teste de compilação e o de determinismo pela semente são obrigatórios no passo do OT; se falharem, revemos a decisão.

**"Quanto pesa o pacote do catálogo?"**
Medimos: cada miniatura de capa tem de 1,6 a 19,8 KB, então mil livros ficam abaixo de cerca de 20 MB.

---

## Se quiserem mostrar ao vivo

Na raiz do repositório:

```bash
docker compose up -d
cd site && go run ./cmd/site
```

Depois abrir http://localhost:8080. A área administrativa fica em `/admin/entrar`; antes é preciso criar o primeiro admin com `go run ./cmd/admin-seed -nome "..." -email ...`.
