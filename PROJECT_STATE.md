---
project: mascotapp
current_phase: "02"
current_task: "T-02-023"  # T-02-022 CERRADA: puerto de email + stub LogSender (primera mitad de PR-02-16), 158 impl / 469 total, entra en presupuesto
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-11 verdes en local, Judgment Day APROBADO. T-02-022 cerrada en feat/pr-02-16-email. Sigue T-02-023 (PR-02-12, RED de middleware_auth.go)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-05
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-022 CERRADA el 2026-09-05. Paquete nuevo apps/api/internal/email/ -- puerto Sender + adaptador LogSender, primera mitad de PR-02-16. Rama feat/pr-02-16-email, commits 3c5f137 (RED) y daef899 (GREEN). QUE HAY: email.go con Sender (una sola funcion, Send(ctx, to, msg) error), Message, Purpose, ErrNoRecipient, ErrNoPurpose y checkSend compartido para que un segundo adaptador del paquete rechace exactamente lo mismo y no aproximadamente; logsender.go con LogSender; email_test.go con nueve tests. Purpose sale con UN SOLO miembro, PurposeMagicLink, porque es el unico correo que la Fase 02 manda. LA DECISION DE TESTING QUE VALE LA PENA RECORDAR: el spec pide que el stub no haga ninguna llamada saliente, y eso se prueba DOS VECES a proposito. Cambiar http.DefaultTransport por un RoundTripper que falla prueba solo que ESTA llamada no salio por ESE cliente -- un http.Client armado a mano o un exec.Command('sendmail') pasan por al lado. El segundo test lee los imports de los .go no-test del propio paquete y falla ante net, net/http, net/smtp u os/exec: ese no prueba que no salio, prueba que NO TIENE POR DONDE SALIR. Y cuenta los archivos que parseo, fallando con menos de dos, porque sobre un directorio vacio pasaria igual de fuerte (mismo patron que obs-9e00e3a6413dc7d2). Efecto secundario que es el que mas vale: cuando llegue el adaptador de Resend va a necesitar su propio paquete o va a tener que borrar ese test A PROPOSITO, que es cuando la decision merece rediscutirse. EXCEPCION DE PII DECLARADA: LogSender registra la DIRECCION del destinatario, y la 5.4 dice que los logs nunca llevan PII. Se hace igual porque un registro que dice 'se le mando un mail a alguien' vuelve el flujo inauditable y hace IMPOSIBLE la asercion que T-02-030 necesita ('solo la direccion de la cuenta que existe recibe un correo', la mitad de la prueba de indistinguibilidad del magic link). La contencion es una sola: LogSender NUNCA es el adaptador de produccion, escrito en su comentario de doc porque nada en el sistema de tipos lo obliga. Lo que no registra nunca es Message.Body -- un cuerpo de magic link es una credencial viva de un solo uso -- y hay un test que busca la credencial en TODA la salida, no en el campo body. MUTACION: tres mutantes, tres muertos (loguear el cuerpo, sacar el chequeo de ctx.Err(), importar net/http). Cero residuo verificado por busqueda explicita. ENTORNO: Docker Desktop estaba caido al empezar; lo levante y la suite completa dio exit=0 CAPTURADO de verdad, con los nueve tests nombrados en la salida -v dentro del contenedor (ok <paquete> no prueba que un test corrio). El exit=1 de test-short era el unlinkat de Windows sobre el .test.exe con cero --- FAIL, verificado leyendo el archivo en vez de asumirlo. PRESUPUESTO: 158 implementacion de 250 y 469 total de 800 -- entra con holgura en el PR. Contra el est: 70 de la propia tarea es 2,3x, y el exceso es densidad de comentarios, no superficie (el paquete exporta una interfaz, un struct, un constructor, una constante y dos errores). OJO CON LO QUE VIENE: la proyeccion de PR-02-16 era 448 lineas totales y T-02-022 sola ya va en 469, con T-02-030 (est: 120) todavia sin escribir. El PR va camino a necesitar size:exception; queda dicho ahora, no cuando ya no haya alternativa. RAMAS -- DECISION QUE HAY QUE RESPETAR: el orden de TAREAS pone T-02-022 aca, pero el orden de MERGE pone PR-02-16 en la posicion 16. Si se encadenara feat/pr-02-12-... arriba de esta rama, PR-02-12 arrastraria los commits de PR-02-16 y el orden de merge se romperia. Entonces feat/pr-02-16-email sale de feat/pr-02-11-session, y PR-02-12 a PR-02-15 salen de feat/pr-02-11-session tambien, como HERMANAS. El paquete email no toca nada mas, asi que ninguna lo necesita. T-02-030 (segunda mitad de PR-02-16, el handler de magic link) rebasa sobre PR-02-15 cuando se escriba. CANDIDATO DE MEMORIA NUEVO, PENDIENTE DE APROBACION: .engram/queue/2026-09-05-probar-una-ausencia-por-comportamiento-solo-prueba-el-camino-que-tomaste.md, score 4, topic_key mascotapp/convention/structural-vs-behavioural-absence-tests. SIGUE T-02-023: RED de middleware_auth.go (PR-02-12), el criterio de exito de la Fase 02 -- el alcance de tenant sale SOLO del claim verificado del token. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en presupuesto) — `PR-02-16` merge en la posición 16, así que `PR-02-12`…`PR-02-15` salen de `feat/pr-02-11-session` como **hermanas** de esa rama, no encima. Sigue `/sdd-apply` desde `T-02-023` (`PR-02-12`, RED de `middleware_auth.go`). **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
