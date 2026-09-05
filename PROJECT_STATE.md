---
project: mascotapp
current_phase: "02"
current_task: "T-02-021"  # JUDGMENT DAY. T-02-019 y T-02-020 cerradas en local, branch feat/pr-02-11-session. PR-02-11 excede los dos presupuestos (381/250 y 930/800): size:exception PENDIENTE
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-10 verdes en local; PR-02-11 completo pero GATEADO POR JUDGMENT DAY (T-02-021) antes del merge
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-019 y T-02-020 CERRADAS EN LOCAL el 2026-09-04, rama feat/pr-02-11-session sobre feat/pr-02-10-recovery. PR-02-11 NO ES MERGEABLE TODAVIA: LO GATEA JUDGMENT DAY (T-02-021), y detras esperan 13 PRs. HALLAZGO PRINCIPAL, ES UNA REGLA DE DISENO NO UN BUG PUNTUAL: UN RECHAZO QUE ESCRIBE NO PUEDE VIAJAR COMO ERROR DE UN CALLBACK TRANSACCIONAL. db.WithAuthUser hace rollback cuando el callback devuelve error (internal/db/auth.go:68 y :77). La deteccion de reutilizacion es el UNICO rechazo del sistema que ESCRIBE: revoca toda la familia. Devolver ErrRefreshTokenReused desde adentro del callback hacia rollback de la revocacion junto con el error -- el ladron recibia 401 y SE QUEDABA CON LA SESION VIVA, o sea el resultado exacto que la revocacion de familia existe para impedir. No fallaba nada: sin excepcion, sin log, la funcion 'funcionaba' porque rechazaba; solo que la contencion nunca ocurria. ARREGLO DE CONTRATO, NO PARCHE: RotateRefreshToken devuelve (RotationOutcome, error) donde el error es SOLO infraestructura y el rechazo viaja en RotationOutcome.Refusal, asi que el callback devuelve nil y la transaccion COMMITEA con la revocacion adentro. Excepcion nombrada en su propio comentario: perder una carrera de rotacion concurrente SI usa el error, porque ahi ya se inserto la fila sucesora y hay que deshacerla. La regla no es 'los rechazos nunca son errores', es 'un rechazo commitea si y solo si dejo algo escrito que tiene que sobrevivir'. COMO APARECIO, QUE IMPORTA TANTO COMO EL QUE: lo encontro el PRIMER GREEN, no la mutacion ni una revision, y solo porque el RED habia escrito el ESCENARIO COMPLETO DEL ROBO (rotar, presentar la copia vieja, y despues verificar que el token que el usuario legitimo tenia en la mano quedo revocado) en vez de limitarse a 'devuelve ErrRefreshTokenReused'. Un test que solo aserta el error habria pasado con el bug puesto. Ese tercer escenario del spec parecia redundante al escribirlo y no lo era. ES LA OTRA CARA DE obs-9e00e3a6413dc7d2: un PR antes, en recovery.go, el MISMO rollback volvio EQUIVALENTE a un mutante (mover una guarda debajo de un DELETE no cambiaba nada porque el rollback preservaba las filas) -- ahi el rollback salvaba, aca destruye. CANDIDATO DE MEMORIA PENDIENTE DE APROBACION en .engram/queue (2026-09-04-un-rechazo-que-escribe-no-puede-viajar-como-error.md, score 5, topic_key mascotapp/arch/refusals-with-side-effects). Aplica a todo flujo que audite/marque/contenga mientras rechaza: registrar un login fallido, bloquear una cuenta al enesimo intento, escribir auditoria de acceso denegado, consumir un intento de rate limiter. MUTACION: SEIS MUTANTES, SEIS MUERTOS. family_id->id, IS NULL->IS NOT NULL, la reutilizacion como error de infraestructura (el bug original, ahora mutante permanente), rotacion que arranca familia nueva, chequeo de expiracion quitado, rama de reutilizacion quitada entera. OTRAS DECISIONES: RotateRefreshToken recibe el SECRETO nunca el user_id parseado; ErrRefreshTokenReused ENVUELVE a ErrRefreshTokenInvalid; la expiracion NO quema la familia; RefreshTokenLifetime = 30 dias fijado aca porque el spec no fija ninguno (marcado para revision). Se agregaron dos queries a query/auth.sql (InsertRefreshToken, MarkRefreshTokenRotated) y se regenero sqlcgen con make generate. PRESUPUESTO: EXCEDE LOS DOS. Implementacion 381 de 250 (+131), total 930 de 800 (+130); sqlcgen excluido por la regla del est: (linea 56 de tasks.md), las 21 lineas de query/auth.sql SI cuentan. NECESITA size:exception DEL USUARIO. Mi prediccion al cerrar PR-02-10 fue 700-750 'probablemente adentro' y midio 930: acerte la direccion y erre la magnitud. DEUDA NO ARREGLADA A PROPOSITO: internal/db/auth.go y auth_test.go estan sucios de gofmt, preexistentes de 6bffd3c (T-02-002) y sin tocar por este PR. No se arreglan aca porque el diff de este PR es exactamente lo que Judgment Day revisa; queda anotado en T-02-020 y va a T-02-021, que ya tiene auth.go en su alcance. NOTA DE HERRAMIENTA: go vet en el host se colgo dos minutos sin salida (Smart App Control) y hubo que usar el contenedor. SIGUE T-02-021: JUDGMENT DAY sobre session.go + la ruta de acceso a refresh_tokens (auth.go, 00013) como una sola unidad, ANTES del merge. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` cerradas en local (PR-02-01…PR-02-11; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue **`T-02-021` — Judgment Day** sobre `PR-02-11`, que **no se mergea hasta que esa revisión cierre**. **`PR-02-11` excede LOS DOS presupuestos** (implementación 381 de 250, total 930 de 800) y **necesita `size:exception`**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

## Invariantes

1. **Como máximo UNA tarea en estado `[~]`** en todo el tablero. Si encontrás dos, es un error: resolvelo antes de trabajar.
2. **Ninguna tarea pasa a `[x]`** sin test en verde, lint limpio y `PROJECT_STATE.md` actualizado.
3. **El avance vive acá, no en Engram.** Engram guarda *por qué*, nunca *qué se hizo*.
4. **Los artefactos técnicos se escriben en inglés** (código, identificadores, columnas, endpoints, commits, tests). La documentación del vault va en español.

## Ritual de inicio de sesión — obligatorio

0. **`make doctor`** — antes de leer nada. Ejecuta cada herramienta externa del
   flujo (no solo la busca en el `PATH`) y dice qué se rompe sin cada una.
   **Leé su salida entera; nunca la pases por un pipe.** Si es tu primera sesión
   con este repositorio, leer también [AGENTS.md](AGENTS.md).
1. Leer este archivo.
2. `mem_context` + `mem_search` sobre `current_phase`. **Si tu herramienta no
   tiene el MCP de Engram** (Kiro, OpenCode), reemplazar este paso por
   [docs/vault/20-arquitectura/indice-engram.md](docs/vault/20-arquitectura/indice-engram.md):
   el texto completo de cada decisión está versionado en `.engram/queue/`.
3. Abrir `docs/vault/30-fases/FASE-<current_phase>.md`. Buscar el primer `[~]`; si no hay, el primer `[ ]`.
4. Leer el spec enlazado y la última entrada de `docs/vault/40-bitacora/`.
5. Confirmar la tarea al usuario en una línea. **Entonces** empezar.

## Ritual de cierre de tarea — obligatorio

1. Tests en verde, lint limpio.
2. Marcar `[x]`; mover el `[~]` a la siguiente tarea.
3. Actualizar este archivo, incluido `next_action`.
4. Añadir entrada a `docs/vault/40-bitacora/<fecha>.md`.
5. Evaluar candidatos de memoria contra `.engram/RUBRIC.md` → escribir a `.engram/queue/` → `make engram-index`.
6. Commit convencional: `feat(<area>): T-00-005 <descripción en inglés>`.
   Convención completa: [docs/vault/99-plantillas/plantilla-commit.md](docs/vault/99-plantillas/plantilla-commit.md).

## Enlaces

- **Contrato de trabajo para cualquier agente: [AGENTS.md](AGENTS.md)**
- Plan maestro: [docs/vault/10-propuesta/analisis-y-plan.md](docs/vault/10-propuesta/analisis-y-plan.md)
- Tablero de la fase actual: [docs/vault/30-fases/FASE-02.md](docs/vault/30-fases/FASE-02.md)
- Change SDD activo: [openspec/changes/phase-02-auth-and-multitenancy/](openspec/changes/phase-02-auth-and-multitenancy/)
- Decisiones: [docs/vault/20-arquitectura/](docs/vault/20-arquitectura/)
- Índice de memoria (sin MCP): [docs/vault/20-arquitectura/indice-engram.md](docs/vault/20-arquitectura/indice-engram.md)
- Rúbrica de memoria: [.engram/RUBRIC.md](.engram/RUBRIC.md)
