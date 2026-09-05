---
project: mascotapp
current_phase: "02"
current_task: "T-02-024"  # T-02-023 RED cerrada: middleware_auth_test.go, 445 lineas, falla al compilar con cinco simbolos indefinidos
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # T-02-023 RED en feat/pr-02-12-middleware (commit 31ca14c). Sigue T-02-024, el GREEN de middleware_auth.go
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-05
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-023 RED CERRADA el 2026-09-05 en feat/pr-02-12-middleware (commit 31ca14c, rama que sale de feat/pr-02-16-email). apps/api/internal/httpapi/middleware_auth_test.go, 445 lineas, DIEZ tests, cero implementacion. RED verificado por FALLO DE COMPILACION, no por asercion que falla: ClaimsFromContext, TxFromContext, ShelterIDPathParam, RequireAuth y RequireTenant todos indefinidos. Cubre los TRES escenarios de authorization-rbac / 'Tenant scope derives only from the verified token claim', que es el criterio de exito de la Fase 02. LA API QUE EL RED FIJA, y que T-02-024 tiene que satisfacer: RequireAuth(*auth.TokenIssuer, func() time.Time) func(http.Handler) http.Handler; RequireTenant(TenantScoper) func(http.Handler) http.Handler; type TenantScoper = func(ctx context.Context, shelterID uuid.UUID, fn func(context.Context, pgx.Tx) error) error -- una clausura sobre db.WithTenant + el pool de tenant; la costura existe para que el espia pueda asertar 'nunca invocado' de forma exacta; ClaimsFromContext(ctx) (auth.AccessClaims, bool); TxFromContext(ctx) (pgx.Tx, bool); const ShelterIDPathParam. EL ORDEN DE LOS CHEQUEOS ES PARTE DEL CONTRATO: verificar el token (401) -> exigir un claim shelter_id no-nil y no-cero (403) -> comparar path/query/header (403 si discrepan) -> RECIEN AHI WithTenant. La propiedad bajo test NO es 'vuelven las filas correctas'. RLS evalua correctamente contra el scope que le den, asi que darle el refugio equivocado es una evaluacion correcta contra el tenant equivocado, sin error en ningun lado. Lo que se aserta es que el valor que llega a WithTenant salio del claim verificado y de NADA MAS. DOS TESTS EXISTEN PORQUE LOS OBVIOS NO DISTINGUEN LAS FUENTES: (1) scope en una ruta SIN segmento de refugio en el path -- en un request que coincide, un middleware que lee el claim y uno que parsea el path producen el mismo uuid, asi que solo una ruta sin el segmento los distingue; una implementacion que tome el scope del path pasa TODOS los demas tests del archivo. (2) el uuid CERO se rechaza -- es no-nil en Go, asi que un chequeo de presencia escrito como claims.ShelterID != nil sobre un decode que defaulteo el campo lo deja pasar hasta WithTenant, que lo rechaza una capa demasiado tarde. LOS DOS ESPIAS FALLAN EL TEST AL CONTACTO, asi que 'nunca invocado' es el default y llegar a ellos hay que habilitarlo explicitamente. Todo rechazo se aserta DOS VECES: el status que ve el cliente, y que ni el scoper ni el handler corrieron. Un 403 escrito despues de que WithTenant ya abrio una transaccion sobre el refugio del atacante no es un rechazo: es una fuga con un status code que pide disculpas. Los tokens son REALES y estan REALMENTE firmados -- un verificador stubbeado dejaria todo el archivo en verde sobre un middleware que no verifica nada. ADVERTENCIA DE PRESUPUESTO PARA PR-02-12: est 290, proyeccion 684. T-02-023 sola mide 445 con T-02-024 (est: 140) sin escribir. El limite total (800) esta en riesgo real y el de implementacion (250) depende enteramente de como caiga T-02-024. Dicho ahora, no cuando ya no haya alternativa. RAMAS -- YA RESUELTO: PR-02-16 se partio en 16a (T-02-022, el puerto, merge en posicion 12) y 16b (T-02-030, el handler de magic link, posicion 17). La cadena queda EN UNA SOLA LINEA: feat/pr-02-11-session -> feat/pr-02-16-email -> feat/pr-02-12-middleware. Regla que quedo escrita: si el orden de merge y la topologia de ramas se pelean, el que esta mal es el CORTE DEL PR, no la topologia; nunca ramas hermanas, porque PROJECT_STATE.md tiene que tener una sola respuesta a 'donde vamos'. SIGUE T-02-024: el GREEN de middleware_auth.go. Su DoD exige MUTACION -- la rama del 403-por-mismatch y la de 'WithTenant solo desde el claim' son las dos garantias que tienen que sobrevivir a todo mutante. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`T-02-023` RED cerrada el 2026-09-05** en `feat/pr-02-12-middleware` — 445 líneas de test que fallan al compilar, los tres escenarios del criterio de éxito de la fase. Sigue `/sdd-apply` desde `T-02-024` (`PR-02-12`, GREEN de `middleware_auth.go`). **⚠️ `PR-02-12` en riesgo de presupuesto**: `est:` 290, `T-02-023` sola mide 445 y `T-02-024` está sin escribir. **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
