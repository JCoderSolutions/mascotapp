---
project: mascotapp
current_phase: "02"
current_task: "T-02-020"  # T-02-019 (RED de session.go) cerrada en local, branch feat/pr-02-11-session sobre feat/pr-02-10-recovery
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-10 verdes en local. T-02-019 (RED) cerrada. Sigue T-02-020 (GREEN de session.go) -- PR-02-11 GATEADO POR JUDGMENT DAY antes del merge
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-019 (RED de session.go) CERRADA EN LOCAL el 2026-09-04, rama feat/pr-02-11-session sobre feat/pr-02-10-recovery. session_test.go: 536 lineas, nueve funciones -- cinco puras (formato de cookie, parseo, secreto, hash) y seis de integracion contra el contenedor (una de las puras es tabla de once casos). RED confirmado por go vet (undefined: auth.RefreshSecretLength) y verificado con rg que los DIEZ simbolos que el test fija estan ausentes. go build ./... sigue verde. API QUE EL TEST FIJA PARA T-02-020: RefreshSecretLength (=32), RefreshTokenLifetime, RefreshCookie{UserID, Secret}, FormatRefreshCookie(uuid, []byte) string, ParseRefreshCookie(string) (RefreshCookie, error), NewRefreshSecret() ([]byte, error), HashRefreshSecret([]byte) []byte, IssueRefreshToken(ctx, pgx.Tx, uuid, time.Time) (string, error), RotateRefreshToken(ctx, pgx.Tx, secret []byte, now time.Time) (string, error), ErrRefreshTokenInvalid, ErrRefreshTokenReused. TRES DECISIONES QUE TOMA ESTE RED. (1) RotateRefreshToken RECIBE EL SECRETO, NUNCA EL user_id PARSEADO: el prefijo es input controlado por el atacante y su unico trabajo es decirle al LLAMADOR que scope de WithAuthUser abrir. Pasarselo tambien a la rotacion le daria una segunda respuesta, mas debil, a una pregunta que RLS ya contesta; el user_id del token nuevo sale de la fila que devolvio la base dentro de ese scope. La firma ES la garantia: la funcion no puede confiar en el cliente porque nunca ve lo que el cliente afirmo. (2) ErrRefreshTokenReused ENVUELVE a ErrRefreshTokenInvalid: la reutilizacion es un EVENTO de seguridad que el servidor tiene que poder alarmar aparte, pero la respuesta HTTP es identica a cualquier cookie mala, asi que errors.Is(err, ErrRefreshTokenInvalid) vale para las dos y ningun llamador puede tratar la reutilizacion como exito por accidente. Testeado en las dos direcciones. (3) RefreshTokenLifetime = 30 dias SE FIJA ACA: ni el spec ni el design fijan un numero (5.2 solo fija los 15 minutos del access token), o sea que es una eleccion y no una transcripcion -- MARCADA COMO TAL PARA REVISION DEL USUARIO. AGREGADO MAS ALLA DE LA FILA: un token expirado se rechaza y su familia SOBREVIVE. La expiracion es el final ordinario de una sesion quieta, no evidencia de robo; quemar la familia por eso desloguearia al usuario de todos sus dispositivos y dispararia la alarma de reutilizacion sobre un no-evento. DISCIPLINA DE CAPAS: los tests de reutilizacion nombran que contesta. El rechazo Y la revocacion de familia son AMBOS de este paquete -- nada en la base rechaza un token revocado (GetRefreshTokenByHash no lleva predicado mas alla del hash) y nada revoca una familia por su cuenta. El test de prefijo mangleado nombra el caso opuesto: ahi RLS es lo que esconde la fila, y el test lo dice. PRESUPUESTO: est 170, actual 536. Es exactamente lo que anticipe al cerrar PR-02-10: la proyeccion de 755 para PR-02-11 se sostiene porque rotacion y deteccion de reutilizacion son superficie adversarial pura. Con el GREEN (est 150) esto va a rondar 700-750, o sea CERCA del limite de 800 pero probablemente adentro. SIGUE T-02-020: GREEN de session.go, misma rama. DoD exige mutacion: TODO mutante sobre la clausula WHERE family_id = $1 AND revoked_at IS NULL tiene que morir. GATE CRITICO: PR-02-11 ESTA GATEADO POR JUDGMENT DAY (T-02-021) ANTES DEL MERGE, y detras suyo esperan 13 PRs. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-019` cerradas en local (PR-02-01…PR-02-10 y el RED de PR-02-11; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue `/sdd-apply` desde `T-02-020` (`PR-02-11`, GREEN de `session.go`) — **gateado por Judgment Day (`T-02-021`) antes del merge**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
