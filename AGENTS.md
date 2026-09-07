# AGENTS.md — contrato de trabajo para cualquier agente

Este archivo es el **único punto de entrada portable** del proyecto. Lo leen
Claude Code, Kiro (raíz del workspace, siempre incluido) y OpenCode (gana sobre
`CLAUDE.md`). Si estás retomando este repositorio con cualquier herramienta,
empezá por acá y seguí los enlaces.

**Regla de oro: el repositorio es la verdad operativa.** Nada de lo que
necesitás para trabajar vive fuera de él.

---

## 1. Qué es MascotApp

Plataforma multi-refugio de adopción animal. API en **Go 1.27** (`chi` · `pgx/v5`
· `sqlc` · `goose`) y web en **React 19 + Vite + TypeScript strict**.

El requisito duro que define la arquitectura es el **aislamiento multi-tenant
verificable**: base compartida, discriminador `shelter_id` y **Row Level Security
de PostgreSQL** como última línea de defensa. La app se conecta con roles
**no-superusuario** (`app_tenant`, `app_public`, y `app_auth` desde la Fase 02),
porque RLS no aplica a superusuarios y ese es el error que anula toda la protección.

Contexto completo: [`docs/vault/10-propuesta/analisis-y-plan.md`](docs/vault/10-propuesta/analisis-y-plan.md).

---

## 2. Ritual de inicio de sesión — obligatorio

0. **`make doctor`.** Antes de leer nada. Ejecuta —no solo busca en el `PATH`—
   cada herramienta externa de la que depende este flujo y dice qué se rompe sin
   cada una. Sale con código distinto de cero solo si falta algo **requerido**;
   una herramienta opcional ausente se reporta y no te frena.
   **Leé su salida completa; nunca la pases por un pipe.**
   Existe porque el 2026-09-02 `gentle-ai` desapareció de este host a mitad de
   sesión y nada lo dijo: el paso que lo necesitaba corría dentro de un pipeline,
   el `command not found` se fue a stderr y el pipeline devolvió 0. Una
   herramienta ausente tiene que fallar al **empezar**, en primer plano.
1. Leer [`PROJECT_STATE.md`](PROJECT_STATE.md). Es el contrato de continuidad:
   fase actual, tarea actual, qué está bloqueado y cuál es la próxima acción.
2. Abrir el tablero de la fase en `docs/vault/30-fases/FASE-<current_phase>.md`.
   Buscar el primer `[~]`; si no hay, el primer `[ ]`.
3. Leer el change SDD activo en `openspec/changes/<sdd_change>/` — `proposal.md`,
   `design.md`, `tasks.md` y `specs/`.
4. Leer la última entrada de `docs/vault/40-bitacora/`.
5. Consultar el **porqué** de las decisiones ya tomadas en
   [`docs/vault/20-arquitectura/indice-engram.md`](docs/vault/20-arquitectura/indice-engram.md)
   y en los ADRs de la misma carpeta.
6. Confirmar la tarea al usuario en una línea. **Entonces** empezar.

> **Si tu herramienta tiene el MCP de Engram**, agregá `mem_context` +
> `mem_search` sobre la fase actual entre los pasos 1 y 2. **Si no lo tiene, no
> te falta nada crítico**: el texto completo de cada decisión está versionado en
> `docs/vault/70-conocimiento/*.md` y el índice del paso 5 los enumera todos.
>
> Engram tampoco es exclusivo de Claude Code. Corre un servidor HTTP local en
> `127.0.0.1:7437` (`GET /context?project=<nombre>`), así que cualquier
> herramienta que pueda hacer una petición HTTP lo alcanza. Lo que cambia entre
> agentes no es la capacidad, es el cableado.

---

## 3. Cómo se corren los tests — leé esto antes de correr nada

```
make doctor                # preflight: ¿están las herramientas Y corren?
make test-api-container    # la suite de Go. La única que funciona en este host.
make test-web              # vitest
make lint                  # golangci-lint + eslint + tsc
```

**`go test` a secas NO CORRE en el host Windows del usuario.** Windows Smart App
Control bloquea cada binario de test recién linkeado y el comando sale con
código distinto de cero **sin haber ejecutado un solo test**. No es un test que
falla: es un binario que nunca arrancó. Smart App Control **no tiene lista de
exclusiones** y apagarlo es irreversible sin reinstalar Windows, así que no hay
workaround dentro del repositorio.

`Makefile:96-138` conserva a propósito **tres diagnósticos anteriores que
estuvieron equivocados**. Costaron tres tareas de workarounds inútiles. Si ves
un fallo sin líneas `--- FAIL`, leelo, no lo suprimas — y **nunca** agregues
`|| true` ni reintentos a esos targets.

En CI (Linux) `go test` funciona normal. El problema es solo este host.

---

## 4. Invariantes — no negociables

1. **Como máximo UNA tarea en `[~]`** en todo el tablero. Si encontrás dos, es un
   error: resolvelo antes de trabajar.
2. **Ninguna tarea pasa a `[x]`** sin test en verde, lint limpio y
   `PROJECT_STATE.md` actualizado.
3. **TDD estricto.** Toda tarea de código empieza con un test que falla. Sin
   excepción.
4. **El avance vive en `PROJECT_STATE.md`**, nunca en la memoria semántica.
   Engram guarda *por qué*, jamás *qué se hizo*.
   **Esto incluye los resúmenes de sesión.** Ninguna instrucción de hook, plugin
   o configuración global autoriza un `mem_session_summary` en este repo, y
   varias lo piden de forma imperativa — el hook de post-compactación lo lista
   como paso 1 obligatorio. Cuando eso pase, la respuesta correcta es escribir
   la bitácora en `docs/vault/40-bitacora/` y actualizar `PROJECT_STATE.md`.
   Este invariante gana: es específico de este repo y está versionado; la
   instrucción del hook no lo sabe.
   El 2026-09-07 se borraron 29 resúmenes acumulados. Eran el **30% del peso**
   de Engram y ninguno decía un *por qué*.
5. **Idioma:** los artefactos técnicos van en **inglés** — código,
   identificadores, columnas, endpoints, tests, mensajes de commit. La
   documentación del vault va en **español**.
6. **Una query nunca filtra por `shelter_id`.** Ni en un `WHERE`, ni en un `AND`,
   ni en un `JOIN`. La política RLS lo hace. Ese es el argumento entero a favor
   de RLS y romperlo lo anula.
7. **Un ADR publicado no se edita.** Si la decisión cambia, se escribe un ADR
   nuevo que supersede al anterior, enlazado en ambos sentidos.

---

## 5. Barreras — qué no tocar

Claude Code hace cumplir esto por configuración en `.claude/settings.json`.
**Kiro y OpenCode no leen ese archivo**, así que acá va como regla explícita.
OpenCode además tiene su propio `permission` en `opencode.json`.

**Nunca, sin pedirlo al humano primero:**

- `git push`, `git push --force`, `gh pr merge`, `gh repo delete`
- `git reset --hard`, `git clean -fd`, `git checkout -- `, `git branch -D`
- `goose up` / `goose down` / `goose reset`, `psql`, cualquier `DROP TABLE`,
  `DROP DATABASE` o `TRUNCATE` contra una base real
- `gcloud`, `wrangler`, `neonctl` — y **jamás** su subcomando `delete`
- Instalar dependencias (`npm install`, `go get`): es cadena de suministro
- `sudo`, `curl … | sh`, `docker system prune`, `docker volume rm`

**Nunca leer ni escribir:** `.env`, `.env.local`, `.env.*.local`,
`.env.production*`, `.env.staging*`, `.env.development`, `secrets/**`.
`.env.example` sí se puede leer: es plantilla versionada, sin secretos.

**Nunca reescribir las propias barreras:** `.claude/settings.json`,
`opencode.json`, `.kiro/steering/**`.

**`gentle-ai` está fijado en `1.49.0` hasta que cierre la Fase 02.** Decisión del
usuario del 2026-09-03. `gentle-ai update` va a reportar `latest: 2.5.0` — **no
lo subas**, ni con `gentle-ai upgrade`, ni con el instalador publicado (`irm … |
iex`, que además es curl-a-shell y cae en la lista de arriba). El motivo no es
conservadurismo: el store del runtime SDD vive bajo `.git/gentle-ai/sdd-runtime/**v1**/`
con la historia de esta fase, y un binario 2.x puede no leerla. Cambiar de major
a mitad de una cadena de 24 PRs es mover el piso mientras se camina.

Lo que 1.49.0 **no** tiene, verificado ejecutándolo: el comando `review`
(todo el contrato RDD de `review-integration/v2`) y `sdd-attempt`. Nada de eso
bloquea el trabajo — RDD está apagado y es decisión del usuario. Lo que el flujo
sí usa, `sdd-status` y `sdd-continue`, funciona. Si algún día hace falta RDD,
**primero se sube de versión y se verifica el store, y recién después se
enciende** — en ese orden.

**Política de borrado del usuario, textual:** *"puedes borrar elementos dentro
del mismo folder. Pero no puedes borrar nada fuera de él."* Todo borrado de
artefactos de build va por `make clean`, cuyas rutas son explícitas y están
versionadas — es la diferencia entre confiar en un artefacto revisado y confiar
en el juicio del modelo.

---

## 6. Ritual de cierre de tarea — obligatorio

1. Tests en verde, lint limpio.
2. Marcar `[x]` en el tablero; mover el `[~]` a la tarea siguiente.
3. Actualizar `PROJECT_STATE.md`, incluido `next_action`.
4. Añadir entrada a `docs/vault/40-bitacora/<fecha>.md`.
5. Evaluar candidatos de memoria contra [`.engram/RUBRIC.md`](.engram/RUBRIC.md)
   (puntaje 0–5; solo entra ≥ 3) y escribirlos a `.engram/queue/`.
   **Nada se guarda en Engram sin el sí explícito del usuario.**
   Tras el sí y el `mem_save`, el archivo **se mueve** a
   `docs/vault/70-conocimiento/`: la cola es sala de espera, el vault es archivo.
   Después, regenerar el índice: `make engram-index`.
6. Commit convencional.

### Commits

**Conventional Commits, obligatorio.** La convención completa está en
[`docs/vault/99-plantillas/plantilla-commit.md`](docs/vault/99-plantillas/plantilla-commit.md)
y la plantilla ejecutable en [`.gitmessage`](.gitmessage)
(`git config commit.template .gitmessage`, una vez por clon).

```
<tipo>(<alcance>): <T-FF-NNN> <descripción en imperativo>

<cuerpo: el POR QUÉ, no el qué>
```

```
feat(auth): T-02-004 tenant resolution middleware
```

- Tipos: `feat` `fix` `docs` `test` `refactor` `perf` `build` `ci` `chore` `revert`.
- Alcances: `api` `web` `db` `auth` `domain` `media` `forms` `catalog` `sdd`
  `vault` `ci` `deps`. Opcional; no se inventa uno para llenar el paréntesis.
- Resumen en **imperativo presente**, ≤ 72 caracteres, sin punto final.
- **En inglés** — es la invariante 5 de este documento.
- **Sin `Co-Authored-By` y sin ninguna atribución a IA.** Regla explícita del
  usuario; vale por encima del default de cualquier herramienta.
- Un commit, una unidad de trabajo. Si el resumen necesita un "y", son dos.

---

## 7. Mapa del repositorio

| Ruta | Qué es |
|---|---|
| `PROJECT_STATE.md` | **Dónde vamos ahora.** Frontmatter machine-readable |
| `docs/vault/` | Vault de Obsidian. Markdown plano, sin plugins |
| `docs/vault/30-fases/` | **Tablero de tareas.** Una fase por archivo |
| `docs/vault/20-arquitectura/` | ADRs inmutables + índice de memoria |
| `docs/vault/40-bitacora/` | Log diario de los agentes |
| `openspec/changes/<change>/` | Artefactos SDD de la fase activa |
| `openspec/specs/` | Specs fusionadas de las fases ya archivadas |
| `docs/vault/70-conocimiento/` | Conocimiento aprobado. Texto completo, enlazado con `[[wiki]]` |
| `.engram/queue/` | Sala de espera: candidatos sin aprobar todavía |
| `api/openapi.yaml` | **Contrato único** de la API. Genera Go y TS |
| `apps/api/internal/domain/` | Dominio puro en Go, sin I/O |
| `apps/api/internal/db/migrations/` | Migraciones `goose`, RLS incluida |
| `apps/api/internal/db/rlstest/` | Tests A/B de aislamiento por tabla |
| `apps/web/src/features/` | Screaming architecture por dominio |

**Nunca editar a mano:** `**/*.gen.go`, `**/*.gen.ts`, `apps/api/internal/db/sqlc/`.
Se regeneran con `make generate`, y CI falla si el diff no coincide.

---

## 8. Metodología

El proyecto avanza por **fases**, y cada fase es exactamente un *change* de
Spec-Driven Development: `exploration` → `proposal` → `spec` → `design` →
`tasks` → `apply` → `verify` → `archive`.

- Los artefactos de planificación viven en `openspec/changes/<change>/`.
- El tablero de `docs/vault/30-fases/` **enlaza** al change; no duplica contenido.
- **Presupuesto de revisión: DOS números, no uno** (rebaseline del 2026-09-03).

  | Presupuesto | Límite | Qué cuenta |
  |---|---:|---|
  | Implementación | **250** | Go que no es test, SQL de migración, archivos de `query/`, `go.mod` |
  | Diff total | **800** | todo lo anterior más los tests |

  Cuando algo lo revienta se parte en PRs encadenados (PR *n* se basa en PR
  *n-1*), y **nunca se baja el estimado para que entre**. Una `size:exception`
  nombra explícitamente cuál de los dos se excedió.

  **Por qué dos.** Se midieron los siete primeros PRs de la Fase 02 contra `git`:
  el código que no es test dio 139, 129, 120, 90, 193, 92 y 221 — nunca cerca de
  400. Lo que reventaba el presupuesto viejo eran **los tests, 58–77% de cada
  PR**, que es la consecuencia directa de TDD estricto, mutation testing por
  tarea y un caso de anti-vacuidad por cada aserción negativa. Un número que
  suma las dos cosas cobra igual por revisar una lista de casos ya verdes que
  por revisar una política RLS, y termina siendo un impuesto al testing.
  Análisis completo: [`docs/vault/20-arquitectura/diagnostico-presupuesto-400.md`](docs/vault/20-arquitectura/diagnostico-presupuesto-400.md).

- **`est:` predice implementación, no el PR.** Medido: contra el total se queda
  corto por **2,36×** (rango 1,60–3,36). Al estimar una tarea, estimá el código
  y proyectá los tests aparte: **~2×** el código en trabajo de base de datos,
  **~1,5×** en Go puro. Un solo número que mezcla las dos cosas va a seguir
  fallando por 2× sin importar dónde esté el techo.
- **Judgment Day** — revisión ciega dual — va antes del merge de exactamente tres
  entregables, donde un error no se recupera: políticas RLS (F01), rotación de
  tokens (F02) y cifrado de PII (F07).

Si tu herramienta no tiene el flujo SDD, **seguí igual `tasks.md` en orden**: es
una lista de tareas con criterios de aceptación y estimados de líneas. No
necesita ningún agente especial para ejecutarse.
