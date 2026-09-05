---
project: mascotapp
current_phase: "02"
current_task: "T-02-022"  # T-02-021 Judgment Day CERRADA con veredicto APPROVED. PR-02-11 habilitado para merge (push y PR los gatea el usuario). Presupuesto final 495/250 y 1152/800
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-11 verdes en local, Judgment Day APROBADO. Sigue T-02-022 (PR-02-16, puerto de email)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-05
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-021 JUDGMENT DAY CERRADA el 2026-09-05 con VEREDICTO APPROVED. Ledger completo en docs/vault/20-arquitectura/judgment-day-pr-02-11.md. Dos jueces ciegos en paralelo sobre target congelado f507692 (session.go, session_test.go, db/auth.go, migracion 00013, query/auth.sql -- 1253 lineas como UNA unidad). ENCONTRO UN DEFECTO DE SEGURIDAD REAL, CONFIRMADO POR LOS DOS: JD-1, la revocacion de familia NO era atomica. RevokeRefreshTokenFamily fija sus filas candidatas en el snapshot de su propia sentencia bajo READ COMMITTED, asi que un sucesor insertado por una rotacion concurrente SOBREVIVIA a la revocacion de su propia familia -- para siempre, porque nada revoca dos veces una familia ya revocada. El token quedaba permanentemente fuera de la contencion. Es justo lo que la deteccion de reutilizacion existe para impedir. RONDA 1 FALLO Y EL ERROR DE ANALISIS FUE DEL ORQUESTADOR: recomendo un indice unico parcial afirmando que cerraba JD-1. NO lo cierra -- un indice unico parcial hace cumplir un invariante DENTRO de una transaccion, y JD-1 es una carrera ENTRE dos. Con el indice puesto el test de concurrencia seguia fallando 3 de 4 corridas. El actor de correccion lo implemento tal cual, escribio el test, demostro que no servia, y PARO en vez de redisenar por su cuenta. Eso costo una de las dos rondas. LO QUE LA RONDA 1 SI COMPRO: el test de concurrencia, que convirtio JD-1 de hallazgo inferential en defecto REPRODUCIBLE; y el indice se QUEDA porque cierra un problema distinto y real (rotacion ordinaria con dos filas vivas transitorias) y forzo reordenar a revocar-insertar-backpointer, mejor orden igual. RONDA 2 LO CERRO: pg_advisory_xact_lock(hashtext(family_id::text)::bigint) tomado despues de leer la fila presentada y ANTES de decidir nada. 125 trials verdes (5x25); quitando el lock fallan las 3 corridas. EL RE-READ QUE ESCRIBI Y BORRE: la correccion incluia una re-lectura post-lock que yo creia 'la mitad que cierra el agujero'. LA MUTACION LA MATO -- sin ella no cambia nada observable, porque el guard revoked_at IS NULL de RevokeRefreshTokenIfLive ya rechaza una rotacion cuya familia fue revocada mientras esperaba; solo se pierde la CLASIFICACION del rechazo y esa alarma ya la levanto quien detecto el reuse. Los dos jueces lo confirmaron en ronda 2. CONDICION DE REAPERTURA QUE ENCONTRE YO Y NINGUN JUEZ: esa seguridad depende de que revoked_at sea MONOTONO, y eso lo garantiza la CONVENCION DEL CODIGO, NO EL ESQUEMA -- el grant de 00013_auth_role.sql:56 es UPDATE a nivel TABLA, asi que nada impide que alguien escriba una query que ponga revoked_at = NULL. Si eso pasa, quitar el re-read pasa de correcto a inseguro; el arreglo seria un grant por columna, patron que 00015 ya usa. HALLAZGOS RONDA 2: un WARNING confirmado por ambos (comentarios generados en sqlcgen todavia describian el re-read borrado porque edite auth.sql sin re-correr make generate) YA CORREGIDO, el diff resulto ser solo comentarios; y una SUGGESTION de un solo juez (CREATE UNIQUE INDEX sin CONCURRENTLY bloquea escritores durante la construccion) como FOLLOW-UP NO BLOQUEANTE -- no hay produccion y necesitaria -- +goose NO TRANSACTION. CANDIDATO DE MEMORIA PENDIENTE DE APROBACION en .engram/queue: 2026-09-05-un-indice-unico-parcial-no-cierra-una-carrera-entre-transacciones.md, score 5, topic_key mascotapp/arch/intra-vs-inter-transaction-invariants. La distincion reusable: UNIQUE/CHECK/FK/indice parcial hacen cumplir invariantes INTRA-transaccion; advisory lock / SELECT FOR UPDATE / SERIALIZABLE ordenan INTER-transaccion. Si el problema es 'quien ve que y cuando', una restriccion declarativa no lo resuelve por mas que su enunciado se parezca al sintoma. TROPIEZO DE HERRAMIENTA: corri la verificacion final con rg DENTRO del contenedor, donde rg no existe; el || echo 'SUITE VERDE' disparo por el fallo de rg y no por ausencia de FAIL. Es el patron ya guardado en Engram (un pipe se come el codigo de salida). Detectado y re-corrido capturando exit=0 de verdad. PRESUPUESTO FINAL DE PR-02-11: la correccion lo movio de 381/930 a 495 implementacion de 250 y 1152 total de 800. La size:exception se acepto sobre 381/930; NO se re-pregunto porque el usuario ordeno explicitamente las dos correcciones, pero la diferencia queda escrita. VERIFICACION FINAL: suite completa con exit=0 capturado de verdad, siete paquetes verdes, golangci-lint 0 issues sobre auth y db, govulncheck limpio, gofmt limpio, cero residuo de mutacion. ESTO NO ES UN RECIBO DE ENTREGA: Judgment Day no emite autoridad de entrega y no satisface ninguna puerta de commit, push, PR ni release. PR-02-11 queda habilitado EN LO QUE RESPECTA A ESTA REVISION; push y PR los sigue gateando el usuario. Commits de la rama: 8df43e9 (RED), c77eee8 (GREEN), f507692 (aprobacion+excepcion), 64bcf0b (correccion JD-1). SIGUE T-02-022: puerto de email + stub LogSender (PR-02-16). Con PR-02-11 desbloqueado, los 13 PRs que esperaban detras vuelven a estar en camino. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` cerradas en local (PR-02-01…PR-02-11; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. Sigue `/sdd-apply` desde `T-02-022` (`PR-02-16`, puerto de email). **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
