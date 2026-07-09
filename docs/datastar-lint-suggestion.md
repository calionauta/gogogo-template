# datastar-lint: vale adicionar regras pros "erros de path" desta sessão?

**Veredito rápido**

| Erro desta sessão | Vale pegar no datastar-lint? | Onde ele de fato deve ser pego |
|---|---|---|
| (A) CSS/asset servido como `text/html` (MIME errado no `<link>`) | **Não** — fora do escopo do linter | Wiring correto do static handler + checagem de MIME no build |
| (B) `PatchElements`/`MergeSignals` mirando `#todo-list` que não existe no fragmento (Bug C: a lista só atualizava p/ quem disparou) | **Sim** — `PATCH_TARGET_NO_ID` (e `SELECTOR_NO_TARGET`) | datastar-lint, sim (cai no modelo de atributos dele) |
| (C) `MergeFragments` inexistente (API inventada por LLM) | **Não** — redundante | `go build` já quebra (símbolo indefinido) |

Resumo: **uma** das três falhas é boa candidata ao datastar-lint. As outras duas são de camadas diferentes (asset/routing e compilação Go) e não pertencem a um linter de atributos.

---

## Recap dos "erros de path" desta sessão

### (A) `text/html` no lugar de CSS — não é caso de datastar-lint
A tela `/login` (e o resto) carregava `daisyui.min.css` (build v4 standalone) que não existia → o servidor respondia `text/html` (a página 404) e o browser barrava: *"Refused to apply style … because its MIME type ('text/html') is not a CSS MIME type"*.

- **Por que não é datastar-lint:** o linter opera sobre atributos `data-*` do Datastar. Um `<link rel="stylesheet" href="...">` não é atributo Datastar e o linter não tem conhecimento de rotas/MIME.
- **Onde corrigir de verdade:** (1) servir o build correto (Tailwind + DaisyUI v5 `app.min.css`), e (2) garantir `Content-Type: text/css` no handler de static (já ajustado em `templates/static` + `main.go`). Se quiser uma checagem automática, ela é de **asset/build**, não de atributo — ex.: um script que confere se todo `href`/`src` de `<link>`/`<script>` resolve para um arquivo estático real com MIME correto. Fora do datastar-lint.

### (B) Seletor `#todo-list` sem `id` correspondente no fragmento — **CANDIDATO REAL**
O handler fazia `sdk.PatchElements(sse, renderTodoList(...), sdk.WithSelector("#todo-list"))`, mas `renderTodoList` retornava `TodoList` (**sem `id="todo-list"`**). Resultado: o patch SSE não encontrava alvo → a lista só re-renderizava para quem disparou (POST da própria requisição), não para os outros clientes. O fix foi envolver em `TodoListRegion` com `id="todo-list"`.

- **Por que É caso de datastar-lint:** é um problema de contrato de *patch/merge* do Datastar — o fragmento que vai ser "patcheado" precisa de um `id` estável onde o cliente possa ancorar. É exatamente o tipo de regra agnóstica-a-linguagem que o datastar-lint já faz (ele escaneia o `.templ` gerado como HTML e vê o `id`).
- **Ressalva importante:** o datastar-lint é agnóstico a Go — ele **não** cruza o `.go` do handler com o `.templ`. Ou seja, ele pode verificar o **lado do template** (o fragmento tem `id="todo-list"`?), mas não pode verificar sozinho que o `WithSelector("#todo-list")` do Go bate com esse `id`. A concordância Go↔templ precisa de uma *convenção* (sempre envolver regiões patcháveis num componente com `id` conhecido) + esta regra do linter garantindo o lado do template.

### (C) `MergeFragments` inexistente — não vale
Variações como `MergeFragments` não existem no SDK `datastar-go` (só `MergeSignals`/`PatchElements`/etc.). É erro de compilação Go (`undefined: MergeFragments`). O compilador já pega; um heuristic no linter seria redundante e daria falso positivo. (A lição é: LLM não deve inventar API — documentar/exemplos no cali-go-stack cobrem isso melhor que lint.)

---

## Proposta concreta de regra (para o seu projeto datastar-lint)

### `PATCH_TARGET_NO_ID` — regra principal (vale a pena)
**Quando dispara:** um elemento que é raiz de uma região re-renderizada via SSE/patch **não possui `id`** (ou possui múltiplos elementos-raiz ambíguos).

**Heurística de detecção (no HTML gerado):**
- Elemento com assinatura de "stream root": `data-on-load="@get('.../stream')"` (ou outro `@get`/SSE subscription) → precisa de `id`.
- Elemento marcado explicitamente como alvo de patch (ex.: atributo `data-patch-region` convencionado, ou pai de `data-on:*` cujo efeito seja merge).
- Deve haver **exatamente um** elemento-raiz com aquele `id` no fragmento.

**Severidade:** `warning` (não `error`, pq há casos válidos de região sem id).

**Por que cabe no modelo atual:** o `walk.go`/`attrs.go` já caminha o HTML e categoriza elementos por atributos Datastar; basta, ao achar um "stream root", checar presença de `id` no próprio elemento (e unicidade). Zero dependência de Go.

**Exemplo de achado:**
```
todo_list.templ:18  PATCH_TARGET_NO_ID  region subscribed via data-on-load has no id; PatchElements/WithSelector('#todo-list') won't find a target. Add id="todo-list".
```

### `SELECTOR_NO_TARGET` — complementar (opcional)
**Quando dispara:** um `data-on:*`/SSE que referencia um seletor (`#id` ou `.class`) que não existe no mesmo fragmento/template.
- **Limitação:** só intra-arquivo (o linter não sabe o que o Go responde). Útil para pegar tipos (`#todo-lsit`). Severity: `hint`/`warning`.

---

## O que NÃO adicionar (e por quê)
- **MIME/`Content-Type` de assets** → pertence a um linter de build/static, não a atributos Datastar.
- **Nomes de API do SDK Go (`MergeFragments`, etc.)** → `go build` já falha; heuristic no HTML não tem como distinguir de string legítima.
- **Roteamento exato de SSE / build tags** (problemas desta sessão fora do "path") → são Go/routing, não atributo.

## Próximo passo recomendado
1. Adicionar `PATCH_TARGET_NO_ID` no `datastar-lint` (agnóstico, alto valor — pega o Bug C sem falso positivo).
2. Adotar a convenção no `cali-go-stack`: "toda região patchável é envolvida num componente com `id` estável", e o `WithSelector` do Go referencia esse mesmo `id`. O linter cobre o lado do template; a convenção cobre o acordo Go↔templ.
3. Deixar (A) e (C) fora do datastar-lint; (A) resolve-se com wiring/MIME correto + (opcional) checagem de asset no build, (C) já é pego pelo compilador.

> Nota operacional: o `Makefile` deste repo tem um alvo `datastar-lint` que roda `bin/datastar-lint`, mas o binário não é construído por este repo (é o seu projeto separado, dropado em `bin/`). Por isso `make check` quebra localmente na etapa do datastar-lint — não é blocking de CI (o CI roda `golangci-lint-action` + `go test`/`go build`, sem `make check`). Para fechar `make check`, basta buildar/`go install` o `datastar-lint` em `bin/` (ou adicionar um passo de build no `Makefile`).
