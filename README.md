# cali-go-stack

Uma cola inicial pra projetos web em Go. Single binary, zero SaaS, LLM-friendly.

---

Já comecei uns 20 projetos web em Go. Toda vez a mesma conversa: escolher banco, fila, template engine, auth, deploy… e no fim o projeto empacava na decisão, não no código. 

Esse template é minha tentativa de ter um ponto de partida que já resolve 80% dessas escolhas sem te prender num ecossistema fechado.

## O que vem com o pacote

Tudo que você precisa pra construir um app web moderno, num binário só:

| Camada | Escolha | Por quê |
|--------|---------|---------|
| **Linguagem** | Go 1.26 | Compila rápido, deploy fácil, runtime enxuto |
| **Banco + Auth + API** | [PocketBase](https://pocketbase.io) embarcado | Zero config pra ter auth, REST, admin UI, arquivos — tudo SQLite |
| **UI** | [Datastar](https://data-star.dev) (SSE reativo) + [Templ](https://templ.guide) (componentes) | HTML puro vindo do servidor, sem JS framework, sem build step |
| **CSS** | [DaisyUI](https://daisyui.com) + TailwindCSS | Componentes prontos, customizáveis, 34kB minificado |
| **Fila** | [goqite](https://github.com/maragudk/goqite) + SSE Hub | Jobs em background com streaming pro frontend, sem Redis |
| **LLM** | [GoAI](https://github.com/zendev-sh/goai) | Chamadas pra qualquer provider (OpenAI, Anthropic, Groq, Ollama…) |
| **Secrets** | age + `~/.secrets/` | Criptografia local, sem vault, sem nuvem |
| **Tempo real** | NATS JetStream (opt-in) | Multi-usuário real-time, só ativa quando precisar |

Tudo CGO-free. Binário único. `make build` e pronto.

## Stack em camadas, não em silos

Uma das coisas que mais me incomodava em templates prontos é que eles assumem **uma** solução pra cada problema. Mas a realidade é que você precisa de fila **e** de workflow **e** de tempo real — cada um pra uma coisa.

Esse template resolve isso com **três camadas assíncronas complementares**:

```
goqite    → jobs background + SSE pro browser (padrão, sempre ativo)
turbine   → workflows duráveis multi-passo (opcional)
JetStream → tempo real multi-usuário (opt-in, build tag)
```

Elas coexistem no mesmo binário. Não competem.

## O exemplo: Todo App com SSE

O template já vem com um Todo App funcional:

- CRUD completo via PocketBase
- UI reativa com Datastar + DaisyUI
- Streaming SSE em tempo real
- Retry com exponential backoff e jitter
- Testes que rodam com `-race`

É o suficiente pra entender o padrão e começar seu próprio feature module.

## Pra quem é esse template

- **Pra você que cansa de configurar a mesma stack repetidas vezes**
- **Pra você que quer um binário único pra deploy, sem depender de Redis, Postgres, ou SaaS**
- **Pra você que quer LLM integrado sem adicionar uma camada inteira de orquestração**
- **Pra você que prefere HTML vindo do servidor do que SPAs de 2MB**

Não é um framework. Não tem lock-in. Cada peça pode ser substituída individualmente — você pode trocar PocketBase por SQLite puro, goqite por Redis, Datastar por HTMX. O template só te dá um ponto de partida que já funciona.

## Começar

```bash
git clone https://github.com/calionauta/cali-go-stack.git meu-projeto
cd meu-projeto
make dev
```

Abre `http://localhost:8080` e vê o Todo App rodando.

### Outros comandos

```bash
make build           # Gera binário
make test            # Roda testes com race detector
make check           # Lint + tamanhos + dead code
make dev             # Live reload com Air
make build-jetstream # Build com NATS JetStream
make setup           # Instala hooks de pre-commit
```

## Estrutura

```
cmd/web/main.go           # Entrada — só inicializa e conecta as peças
config/                   # Config por ambiente (dev/prod)
db/                       # Setup do PocketBase + seed
internal/
  secrets/                # Decriptação de secrets com age
  queue/                  # goqite + SSE Hub + workers + retry
  nats/                   # JetStream (build-tag gated)
  llm/                    # SDK pra LLM (GoAI)
  datastar/               # Helpers pra Datastar
features/
  todo/                   # Exemplo funcional: Todo MVC
web/resources/            # Assets estáticos (JS embarcado)
router/                   # Rotas registradas no PocketBase
references/               # Documentação de referência
```

## Licenciamento, feedback

Esse projeto é aberto a feedback, PRs, e adaptações. Se algo não faz sentido, se a stack não encaixa no seu problema, ou se você tem uma ideia melhor — abre uma issue.

Feito com intenção de ser útil, não de ser certo.
