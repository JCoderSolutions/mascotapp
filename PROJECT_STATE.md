---
project: mascotapp
current_phase: "02"
current_task: "T-02-019"  # T-02-017 y T-02-018 (PR-02-10) cerradas en local, branch feat/pr-02-10-recovery. PR-02-10 entra en los dos presupuestos (197/250 y 721/800), sin excepcion
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-10 verdes en local. Sigue T-02-019 (PR-02-11, session.go) -- GATEADO POR JUDGMENT DAY antes del merge
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "PR-02-10 (T-02-017 + T-02-018) CERRADO EN LOCAL el 2026-09-04, rama feat/pr-02-10-recovery sobre feat/pr-02-09-token. recovery.go: diez codigos de 128 bits, base32 sin padding, SHA-256 SIN SAL (apartamiento deliberado de password.go: un estirador compensa baja entropia y aca no hay nada que compensar; sin sal porque la redencion tiene que ENCONTRAR la fila desde el codigo enviado). DOS ESTRENOS: primer consumidor de sqlcgen en codigo de produccion, y primer uso de uuid.NewV7 fuera de fixtures. Las dos funciones de base toman pgx.Tx, nunca un pool. HALLAZGO DE LA MUTACION, EL MAS IMPORTANTE: el mutante que mueve la guarda del set vacio DEBAJO del delete SOBREVIVIO, y el comentario del test afirmaba explicitamente que ese caso lo mataba. Contesta el ROLLBACK de db.WithAuthUser, no la posicion de la guarda. QUINTA instancia de obs-9e00e3a6413dc7d2 y la primera donde la capa intrusa es la TRANSACCION, no una policy ni un trigger ni un chequeo propio. Probado equivalente en vez de asumido: un tercer mutante saca la guarda entera y ahi el test muere en la linea del err == nil, o sea que la guarda es load-bearing aunque su posicion no lo sea. ES UNA CLASE, NO UN CASO: dentro de una misma transaccion el orden de una guarda respecto de una escritura destructiva es inobservable para el llamador si el error provoca rollback. La correccion NO fue de codigo -- la guarda se queda arriba porque emitir una sentencia que vas a deshacer es trabajo al pedo -- fue de los DOS COMENTARIOS, que ahora dicen que capa contesta cada asercion. CANDIDATO DE MEMORIA PENDIENTE DE APROBACION en .engram/queue (2026-09-04-un-rollback-contesta-antes-que-el-orden-de-tu-guarda.md, score 4). LA DISCIPLINA DE CAPAS SE APLICO POR ADELANTADO Y LA MUTACION LA CONFIRMO: el mutante que hace a RedeemRecoveryCode hashear distinto de lo que guarda Regenerate mato SOLO la sonda positiva; RefusesACodeThatWasNeverIssued y RefusesAnotherUsersCode siguieron VERDES con el hasheo completamente roto, exactamente como decia el comentario. AGREGADO MAS ALLA DE LA FILA DE LA TAREA: RegenerateRecoveryCodes refusa un set vacio (borraria los diez sin escribir ninguno y reportaria exito; ninguna restriccion de la base lo atrapa). Escrito test-first. PRESUPUESTO: 197 implementacion de 250 y 721 total de 800 -- ENTRA EN LOS DOS, sin excepcion. MI PREDICCION ESTUVO MAL: al cerrar PR-02-09 avise que PR-02-10 iba a necesitar size:exception casi con certeza. La heuristica 'tema critico para seguridad' tuvo contraejemplo el mismo dia. Lo que maneja el costo es la SUPERFICIE ADVERSARIAL, no la etiqueta: token.go tenia que argumentar sobre un formato de entrada hostil, recovery.go es un delete mas diez inserts. Enmendado en tasks.md en el mismo bloque donde estaba la version equivocada. SIGUE T-02-019: RED de session.go (rotacion de refresh + deteccion de reutilizacion), rama feat/pr-02-11-session sobre feat/pr-02-10-recovery. PR-02-11 ESTA GATEADO POR JUDGMENT DAY (T-02-021) ANTES DEL MERGE y detras suyo esperan 13 PRs; su proyeccion de 755 SI sigue en pie, porque rotacion y reutilizacion son superficie adversarial pura. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-018` cerradas en local (PR-02-01…PR-02-10; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue `/sdd-apply` desde `T-02-019` (`PR-02-11`, `session.go`) — **gateado por Judgment Day (`T-02-021`) antes del merge**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
