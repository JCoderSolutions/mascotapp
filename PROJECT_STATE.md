---
project: mascotapp
current_phase: "02"
current_task: "T-02-036"  # T-02-034 cerrada (444 lineas hand-written de api/openapi.yaml contra una estimacion de 90 -- FLAGGEADO, no absorbido en silencio; ver next_action.cerrado). T-02-035 sigue bloqueada por el usuario y NO gatea nada mas: PR-02-21 (T-02-036/037, matriz RBAC) es pura e independiente, sin dependencia de PR-02-20 ni de T-02-035.
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-036 (RED -- authz/matrix.go tests exhaustivos table-driven, PR-02-21) es pura e independiente; no depende de T-02-034 ni de T-02-035. Rama a crear: feat/pr-02-21-rbac-matrix sobre feat/pr-02-20-openapi
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso -- T-02-036 y todo lo que sigue NO esta bloqueado por esto."
last_updated: 2026-09-28
sdd_store: hybrid
last_agent: claude-sonnet-5
next_action:
  cerrado: >
    T-02-034 CERRADA el 2026-09-28 en feat/pr-02-20-openapi sobre
    feat/pr-02-19-session-exchange. GREEN-only (sin RED de dominio; el RED
    de esta tarea es el diff-check de codigo generado, no un test). Se
    documentaron los 8 endpoints de T-02-027...T-02-033 (register, login,
    magic-link, refresh, logout, totp/enrol, totp/verify,
    session/shelter) en api/openapi.yaml, leyendo cada forma de
    request/response directamente del codigo Go de cada handler, no
    adivinada. Se confirmo contra el codigo que NINGUNA ruta /auth/* esta
    wireada en router.go todavia -- eso es T-02-040, al final de la fase,
    exactamente donde lo pone el diseno; esta tarea no toca router.go.
    oapi-codegen regenero internal/api/openapi.gen.go, openapi-typescript
    regenero apps/web/src/api/schema.gen.ts. PRESUPUESTO: 444 lineas
    hand-written de api/openapi.yaml contra una estimacion de 90 -- 4.9x,
    y por encima del presupuesto de 250 hand-written. FLAGGEADO
    explicitamente en tasks.md (no absorbido en silencio); la decision de
    entrega (aceptar tal cual o partir con size:exception) queda para el
    usuario, segun ask-on-risk. Los ~1.7k de lineas generadas no cuentan
    para ese presupuesto por convencion. DECISION TECNICA: se probo
    format: uuid / format: email en los campos shelter_id/user_id/email y
    se revirtio -- tira github.com/oapi-codegen/runtime/types como
    dependencia nueva, contra el minimalismo que internal/api/config.yaml
    ya explica (embedded-spec apagado, ADR-0003, cold start de Cloud
    Run). Quedaron como type: string simple, igual que lo que los
    handlers ya validan a mano con uuid.Parse. VERIFICACION: make
    generate corrido dos veces seguidas contra una base stageada, diff
    identico byte a byte (idempotente); go build ./... verde; make
    test-api-container verde (los 7 paquetes); tsc -b --noEmit de
    apps/web verde; golangci-lint run ./... con un solo hallazgo gosec
    preexistente en internal/db/dbtest/container.go (confirmado con git
    log que viene de T-02-003, no tocado en esta tarea). NOTA DE
    ENTORNO: oapi-codegen no estaba instalado en go/bin (no era que
    estuviera stale, faltaba del todo) -- se instalo v2.8.0 igual que
    .devcontainer/postCreate.sh. Windows Smart App Control borro el
    binario recien linkeado entre la instalacion y una corrida posterior
    de make generate -- la misma clase de problema que ya esta
    documentada para binarios de test de Go, confirmada aca tambien para
    un binario de herramienta instalado con go install. Se resolvio
    reinstalando inmediatamente antes de cada make generate, en el mismo
    comando. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS, CERO COMMITS
    CREADOS TODAVIA (la sesion dejo los cambios en el working tree +
    staged, sin commitear, precisamente porque el presupuesto esta
    flaggeado y no hay una decision ask-on-risk del usuario todavia).
  decision: >
    Sin decision nueva del usuario en esta sesion. Las decisiones
    vigentes de T-02-031 (amr de un refresh es [pwd]) y T-02-032 siguen
    aplicando sin cambios; ver las entradas de esas tareas para el
    detalle completo. PENDIENTE: el usuario todavia tiene que decidir
    sobre el presupuesto de T-02-034 (aceptar 444/444 tal cual como
    size:exception, o pedir que se parta -- ver la nota de "cerrado"
    arriba y el detalle en tasks.md linea ~622).
  sigue: >
    T-02-036 (RED -- authz/matrix.go, tests exhaustivos table-driven,
    PR-02-21) NO INICIADA. Es pura e independiente: no depende de
    T-02-034 (PR-02-20) ni de T-02-035 (PR-02-24, bloqueada). tasks.md
    linea 1081 lo confirma: la dependencia de PR-02-21 es "-- (pure,
    independent; placed here per the design's slice order)". Rama a
    crear: feat/pr-02-21-rbac-matrix sobre feat/pr-02-20-openapi (una vez
    que T-02-034 este commiteada), segun la tabla de dependencias de PRs
    de tasks.md.
  bloqueadores: >
    Ninguno funcional para T-02-036 ni para nada de la cadena PR-02-21
    en adelante. .env.example (T-02-035, PR-02-24, ultimo de la cadena)
    sigue bloqueado por confirmacion del usuario, sin cambios esta
    sesion -- NO afecta a T-02-036 ni a ninguna tarea posterior; la
    cadena funcional entera mergea sin eso, T-02-035 solo bloquea su
    propio PR final. Ademas: la decision ask-on-risk del presupuesto de
    T-02-034 (ver "decision" arriba) esta pendiente antes de commitear
    esta tarea.
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`PR-02-12` COMPLETO el 2026-09-06** (`T-02-023` RED + `T-02-024` GREEN) en `feat/pr-02-12-middleware` — el **criterio de éxito de la fase**: el alcance de tenant sale solo del claim verificado, seis mutantes muertos. **227 impl / 675 total, entra en los dos presupuestos sin excepción** — la advertencia de riesgo que puse el 05-09 **no se cumplió** y queda tachada, no borrada, en `tasks.md`. **`T-02-025` cerrada el 2026-09-06** (`cors.go` + `csrf.go`, allowlist de orígenes + CSRF por `Origin`/`Sec-Fetch-Site`, siete mutantes muertos) en `feat/pr-02-13a-cors-csrf`. **`PR-02-13` se partió** en `PR-02-13a` (cerrado, 277/639) y `PR-02-13b` (`config.go`) — segundo corte de la fase tras `PR-02-16`, y las dos mitades entran en los dos presupuestos **sin excepción**. Sigue `/sdd-apply` desde `T-02-026`. **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

## Invariantes

1. **Como máximo UNA tarea en estado `[~]`** en todo el tablero. Si encontrás dos, es un error: resolvelo antes de trabajar.
2. **Ninguna tarea pasa a `[x]`** sin test en verde, lint limpio y `PROJECT_STATE.md` actualizado.
3. **El avance vive acá, no en Engram.** Engram guarda *por qué*, nunca *qué se hizo*.
4. **Los artefactos técnicos se escriben en inglés** (código, identificadores, columnas, endpoints, commits, tests). La documentación del vault va en español.

## Ritual de inicio de sesión

Ver [AGENTS.md](AGENTS.md) §2 "Ritual de inicio de sesión — obligatorio" — no
se repite acá (fuente única de reglas de proceso).

## Ritual de cierre de tarea

Ver [AGENTS.md](AGENTS.md) §6 "Ritual de cierre de tarea — obligatorio" — no
se repite acá (fuente única de reglas de proceso).

## Enlaces

- **Contrato de trabajo para cualquier agente: [AGENTS.md](AGENTS.md)**
- Plan maestro: [docs/vault/10-propuesta/analisis-y-plan.md](docs/vault/10-propuesta/analisis-y-plan.md)
- Tablero de la fase actual: [docs/vault/30-fases/FASE-02.md](docs/vault/30-fases/FASE-02.md)
- Change SDD activo: [openspec/changes/phase-02-auth-and-multitenancy/](openspec/changes/phase-02-auth-and-multitenancy/)
- Decisiones: [docs/vault/20-arquitectura/](docs/vault/20-arquitectura/)
- Índice de memoria (sin MCP): [docs/vault/20-arquitectura/indice-engram.md](docs/vault/20-arquitectura/indice-engram.md)
- Rúbrica de memoria: [.engram/RUBRIC.md](.engram/RUBRIC.md)
