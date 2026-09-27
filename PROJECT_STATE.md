---
project: mascotapp
current_phase: "02"
current_task: "T-02-033"  # T-02-032 cerrada (197/532 contra la base de PR-02-17). TOTP enrol/verify: la invalidacion de codigos de recuperacion nunca probo que hubiera algo que invalidar (control positivo agregado); la etiqueta de cuenta de la URI otpauth:// nunca se verifico contra el email real; un body JSON malformado en verify nunca se probo
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-033 (Session/shelter-exchange handler, POST /auth/session/shelter, PR-02-19) depende de token.go de PR-02-09. Rama a crear: feat/pr-02-19-session-exchange sobre feat/pr-02-18-totp-enrol
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-26
sdd_store: hybrid
last_agent: claude-sonnet-5
next_action:
  cerrado: >
    T-02-032 CERRADA el 2026-09-26 (RED ca88072 y GREEN 27b2032, ambos de
    una sesion anterior del 2026-09-24/25; tests del mutante partido
    3bd370d, esta sesion) en feat/pr-02-18-totp-enrol sobre
    feat/pr-02-17-refresh. Esta sesion RE-VERIFICO ambos commits contra
    HEAD (no confio en el mensaje de GREEN) antes de correr la mutacion
    pendiente. totp_handlers.go nuevo, sin cambios esta sesion; solo se
    agregaron tests. MUTACION: 9 mutantes generados, 8 muertos (5 ya
    cerrados por los tests existentes, 3 cerrados con tests nuevos), 1
    EQUIVALENTE documentado (el chequeo de error de envelope.Open: todo
    return de Open() en error da secret=nil y VerifyTOTP ya rechaza
    longitud distinta de TOTPSecretLength, confirmado leyendo cada return
    de envelope.go, no solo por la supervivencia del mutante). Cero
    residuo (fuera del equivalente), verificado con diff contra copia
    pristina. PRESUPUESTO: 197 de 250 implementacion (sin cambios, GREEN
    de sesion anterior) y 532 de 800 total contra la base de PR-02-17,
    entra en los dos. VERIFICACION: suite completa en contenedor con
    exit=0 capturado, siete paquetes verdes, los ocho tests de TOTP (cinco
    originales + tres nuevos) nombrados en -v con cero fallos,
    golangci-lint run ./internal/httpapi/... con 0 issues, gofmt limpio.
    NOTA APARTE (fuera de alcance): golangci-lint run ./... sobre todo
    apps/api reporta un gosec G101 preexistente en
    internal/db/dbtest/container.go, de cb31c18/7392bd6, muy anterior a
    esta tarea; no se toco. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS.
  decision: >
    Sin decision nueva del usuario en esta sesion de cierre. La decision
    vigente de T-02-031 (amr de un refresh es [pwd]) sigue aplicando sin
    cambios; ver la entrada de esa tarea para el detalle completo.
  sigue: >
    T-02-033 (Session/shelter-exchange handler, POST /auth/session/shelter,
    PR-02-19) NO INICIADA. Depende de token.go de PR-02-09. Rama a crear:
    feat/pr-02-19-session-exchange sobre feat/pr-02-18-totp-enrol, segun la
    tabla de dependencias de PRs de tasks.md.
  bloqueadores: >
    Ninguno para T-02-033. .env.example (T-02-035, PR-02-24, ultimo de la
    cadena) sigue bloqueado por confirmacion del usuario, sin cambios esta
    sesion -- no afecta a T-02-033.
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
