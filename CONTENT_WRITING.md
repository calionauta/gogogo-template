# gogogo-fullstack-template
## writing style guidelines

1. style classifications

linguistic register: written entirely in lowercase letters (with exceptions only for strictly necessary acronyms or proper nouns).

textual genre: essayistic and confessional. the text assumes the perspective of a first-person learning diary or personal essay.

predominant verbal mood: subjunctive mood and hypothetical tone. the text gropes for possibilities using terms like seems, perhaps, could, signals, and suggests, rejecting dogmatic or absolute statements.

syntactic rhythm: staccato. short sentences. elimination of commas, colons, parentheses, and em-dashes. the transition of ideas is made exclusively by periods.

grammatical voice: active voice and focus on processes. preference for verbs denoting movement, construction, and investigation (e.g., unfold, anchor, thicken, grope).

factual neutral: avoid words with extreme tones, empty (or idle) adjectives, pleonasms, and redundancies. do not use superlatives (the true, certain, best, worst, always, never, the truth, the fundamental) and adverbs of certainty or impact (highly precise, precisely, obviously, clearly, fundamentally). the argument must stand on sober description, not on the force of words.

eliminate aggressive self-promotion terms and marketing bullshit strategies. i want to be anti-marketing bullshit. avoid marketing clichés and jargon. avoid a sales, marketing, or hyperbolic tone. the argument must stand on logic, not on the force of words.

2. command instructions for text generation

adopt a decentered stance: write from the first-person singular (i notice, i noted, i understood). report your own impressions and connections without trying to dictate rules, prescribe behaviors for others, or sell a definitive conclusion.

anti-post principle: does not ask for engagement, does not deliver truths, does not virtue signal, accepts being ignored. radical economy, active ambiguity. non-performative tone, assumed risk.

use the zigzag structure: continuously alternate between the abstract concept (theory, philosophy) and the concrete scene. never let the theory float without a physical example.


## Content Angle

### Core thesis running through all angles:
Gogogo is a distillation of decisions. every web project starts with the same conversation — pick a database, auth, router, reactive ui framework, task queue — and stalls at configuration. this template resolves those choices up front so you start coding instead of deciding.

Note: the project is deliberately opinionated. the opinions exist to be replaced, not to be defended. each layer swaps independently.

#### Functionality / Technical

- **"one binary, zero external services"** — pocketbase (database + auth + api) runs embedded. goqite (task queue) runs on sqlite. nats jetstream and dagnats run in-process. no postgres, no redis, no separate broker. the binary is ~56 mb and runs on scratch.
- **Five realtime layers, not one** — pocketbase realtime for record mutations (per-user scoped via collection rules). sse hub for ephemeral signals (toasts, ai suggest, workflow progress). nats app_crud stream for cross-instance record convergence. nats todos stream for cross-instance ephemeral broadcast. service worker + background sync for web offline replay.
- **SCOPE taxonomy as executable architecture** — every source file carries a SCOPE annotation (core, pluggable, feature) so agents and humans know what they can remove. the annotation tells you not just what the file does but how to delete it. this is architecture as documentation, not documentation as an afterthought.
- **Two opt-out strategies, not one** — infrastructure components (nats, dagnats, offline sync) have runtime env vars. product features (todo, whiteboard) are removed by deleting the directory. the split is intentional: you toggle infrastructure in production without rebuilding; you remove features when you outgrow the demo.
- **Unified build, no build tags** — everything compiles together. no matrix, no stubs, no conditional compilation. opt out happens at runtime, not compile time. the binary is always the same; what runs differs by env var.

#### Strategy / Motivation

- **"optimize for shipping, not for being right"** — the readme says it plainly: decisions are pragmatic, not dogmatic. the template embeds decisions that are good enough to ship today and easy to replace tomorrow. this is the opposite of the framework that locks you into its worldview.
- **The cost of configuration** — every web project i start begins with picking a stack. not building features. the template exists because i got tired of that conversation. it removes the configuration tax once, not per-project.
- **From Postgres dependence to sqlite liberation** — pocketbase with ncruces/go-sqlite3 means zero database setup. no docker, no connection strings, no migration files for local dev. the database is a file you can commit to git.
- **Single binary as deployment primitive** — one file to copy, one file to run, one health check. no dependency manager, no runtime, no sidecars. this is the deployment model that lets you focus on the application, not the infrastructure.

#### Design Philosophy

- **Every decision documented, every decision replaceable** — the architecture file reads like a team's decision log, not a system diagram. each component has a "remove by" instruction. the template is a collection of agreements someone already had, written down.
- **Features in features/**, infrastructure in internal/ — features depend on infrastructure, never the reverse. this is not a new idea, but enforcing it with scope annotations makes it explicit. a feature knows it is replaceable.
- **documentation for agents, not just humans** — scope annotations, the architecture file, the llm entrypoint — these exist to be read by ai agents navigating the codebase. the template treats agents as first-class readers of its documentation.
- **Anti-framework stance** — there is no framework here. each piece is independently replaceable. if you prefer chi over pocketbase's router, swap it. if you want htmx instead of datastar, swap it. the template is a collection of choices, not a cage.

#### Skills / Applied Knowledge

- **Three async layers that coexist** — goqite (jobs + sse), dagnats (durable workflows), nats jetstream (cross-instance realtime). each solves a different problem. they run in the same binary without conflict because they solve different problems.
- **The sse hub as architectural glue** — in-process fan-out to browser tabs via go channels. per-client replay buffer, backpressure, exclude-origin broadcast. the hub is the bridge between background jobs and the browser. it is not a message broker; it is a distribution primitive.
- **PocketBase realtime vs sse hub** — pocketbase realtime handles record mutations. the sse hub handles ephemeral signals. they are not alternatives. they are complementary layers for different types of state change. the distinction matters: record mutations need auth-scoped delivery; ephemeral signals need low-latency fan-out.
- **Architecture as executable documentation** — the ARCHITECTURE.md file is the single entry point for llm agents. it contains the startup order diagram, the dependency graph, the removal instructions. an agent that reads it knows how to navigate the codebase without guessing.

### Potential Audiences

- **Go developers starting a web project** — care about: seeing decisions made, not having to make them. want to start coding features on day one.
- **Solo founders shipping alone** — care about: single binary deploy, zero ops, one server, one file. want to build something that runs without a team.
- **LLM agents navigating the codebase** — care about: scope annotations, architecture file, startup order, removal instructions. need to know what is safe to touch and what is not.
- **Developers tired of configuration** — care about: the decision log, the trade-offs, the reasons behind each choice. want a template they can disagree with, not one they must agree with.
- **Teams evaluating Go for web development** — care about: the stack choices, the justification for each, the migration paths. want to understand what they are buying into.

### Possible Formats

- Short technical thread (twitter/x) — pairs well with: go developers, solo founders
- Long-form blog post ("one binary, zero services") — pairs well with: general tech audience, teams evaluating Go
- Architecture deep-dive (sse hub vs pocketbase realtime) — pairs well with: backend engineers, agent builders
- Case study: deploying a single binary to production — pairs well with: indie hackers, ops engineers
- Comparison post ("why not django/next.js/rails") — pairs well with: developers from other ecosystems
