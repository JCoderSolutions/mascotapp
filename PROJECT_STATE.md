---
project: mascotapp
current_phase: "02"
current_task: "T-02-018"  # T-02-017 (RED de recovery.go) cerrada en local, branch feat/pr-02-10-recovery sobre feat/pr-02-09-token. PR-02-09 cerrado con size:exception ACEPTADA por el usuario el 2026-09-04
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-09 verdes en local. T-02-017 (RED) cerrada. Sigue T-02-018 (GREEN de recovery.go)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-017 (RED de recovery.go) CERRADA EN LOCAL el 2026-09-04, rama feat/pr-02-10-recovery sobre feat/pr-02-09-token. recovery_test.go: 467 lineas, nueve funciones -- cuatro puras (generacion y hash, sin contenedor) y cinco de integracion contra el contenedor. RED confirmado por `go vet`: no compila, `undefined: auth.NewRecoveryCodes`. Verificado ademas con rg que los OCHO simbolos que el test fija estan ausentes del paquete, no solo el primero que reporta el compilador. `go build ./...` sigue verde: nada mas se rompio. API QUE EL TEST FIJA PARA T-02-018: RecoveryCodeCount (=10), RecoveryCode{Plaintext, Hash}, NewRecoveryCodes(), HashRecoveryCode(string) []byte, RegenerateRecoveryCodes(ctx, pgx.Tx, uuid.UUID, []RecoveryCode) error, RedeemRecoveryCode(ctx, pgx.Tx, string) error, ErrRecoveryCodeInvalid. LAS DOS FUNCIONES DE BASE TOMAN UN pgx.Tx, NUNCA UN POOL: el SET LOCAL app.user_id y el limite de la transaccion son de db.WithAuthUser, y una funcion que tomara un pool podria correr fuera de ese alcance. EL TEST DECODIFICA Y RE-HASHEA A MANO: base32 con decoder propio y crypto/sha256 directo, nunca un helper del paquete bajo prueba -- misma disciplina que token_test.go. DISCIPLINA DE CAPAS APLICADA POR ADELANTADO (obs-9e00e3a6413dc7d2, cuya cuarta instancia fue T-02-015): rlstest/totp_recovery_codes_test.go YA fija el uso unico a nivel de tabla con el predicado used_at IS NULL, y RLS ya fija el rechazo entre usuarios. Los dos tests de rechazo de este archivo DICEN EN SU PROPIO COMENTARIO que capa contesta y nombran lo unico que este paquete puede reclamar: traducir cero-filas-afectadas a ErrRecoveryCodeInvalid en vez de a nil. EL TEST QUE AISLA ESTA CAPA ES EL POSITIVO -- un codigo valido tiene que ser ACEPTADO, y es el unico caso capaz de distinguir 'hasheo bien y encontro' de 'hasheo mal y no encontro'. PRESUPUESTO: est 100, actual 467. El PR-02-10 proyectaba 190 totales y el RED solo ya lo cuadruplico. Esto confirma en el acto la nota escrita al aceptar el size:exception de PR-02-09: la senal no es 'proyecta cerca de 800' sino 'el tema del PR es critico para seguridad'. PR-02-10 va a necesitar size:exception casi con certeza -- hay que decirselo al usuario ANTES de escribir el GREEN, no al medir. SIGUE T-02-018: GREEN de recovery.go, misma rama. Un mutante obligatorio: hacer que RedeemRecoveryCode devuelva nil cuando afecta cero filas, y comprobar que muere el test de segunda redencion. NADA PUSHEADO, CERO PRs ABIERTOS. Judgment Day sigue pendiente antes de mergear PR-02-11 (session.go)."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-015` y `T-02-016` cerradas en local (PR-02-01…PR-02-09; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue `/sdd-apply` desde `T-02-018` (`PR-02-10`, GREEN de `recovery.go`). **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
