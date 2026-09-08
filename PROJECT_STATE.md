---
project: mascotapp
current_phase: "02"
current_task: "T-02-030"  # T-02-029 cerrada (115/662). EL CRITERIO E2E DE LA FASE ESTA EN VERDE: un owner con TOTP enrolado y la contrasena CORRECTA no obtiene sesion
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-030 (magic link, PR-02-16b) es la siguiente; la indistinguibilidad entre direccion conocida y desconocida es requisito EXPLICITO del spec. Rama a crear: feat/pr-02-16b-magic-link sobre feat/pr-02-16-totp-login
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-08
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-029 CERRADA el 2026-09-08 (RED 5054fdd, GREEN ea73e45, tests de los sobrevivientes 40323d9) en feat/pr-02-16-totp-login sobre feat/pr-02-15-login. EL CRITERIO E2E DE LA FASE ESTA EN VERDE. login.go +115, login_totp_test.go +547, 18 tests de login en total contra contenedor. LA DECISION QUE EL CONTRATO DEJABA ABIERTA: el spec pide TOTP obligatorio para owner/admin, pero el login todavia NO eligio refugio y un rol es propiedad de una MEMBRESIA, asi que no hay UN rol que consultar. Se pregunta sobre TODAS las membresias ACTIVAS, no sobre una: alguien puede ser owner de un refugio y volunteer en otro, y pedir el segundo factor por refugio dejaria entrar sin el y DESPUES intercambiar esa sesion por el refugio privilegiado, porque el intercambio verifica que la membresia este activa y no COMO se autentico la sesion. DOS PREGUNTAS MAS QUE EL CONTRATO NO CERRABA: un owner que nunca enrolo TOTP SE RECHAZA (la otra lectura vuelve el requisito opcional por inaccion y exime para siempre a todo owner creado antes de que existiera el enrolamiento); y el codigo de recuperacion se chequea ANTES que el TOTP, porque quien manda uno perdio el autenticador y caer al TOTP lo rechazaria por no producir un codigo que no puede producir. CINCO TESTS VIEJOS SE ROMPIERON Y EL CODIGO TENIA RAZON: los de T-02-028 registran un refugio, lo que convierte a esa persona en owner, y despues esperan entrar con contrasena sola -- eso ahora esta PROHIBIDO y es el criterio funcionando. Se cambio LA FIXTURE, no las cinco aserciones: seedAccount ahora crea un usuario SIN membresia, que es lo que es un adoptante y lo que esos tests siempre quisieron decir. MUTACION -- SIETE MUTANTES, DOS SOBREVIVIERON LA PRIMERA PASADA Y ERAN EL MISMO DEFECTO: condiciones escritas en el codigo, declaradas en su comentario, y afirmadas EN NINGUN LADO. Sacar admin del conjunto privilegiado no cambio nada porque todos los casos usaban owner -- una regla que se hace cumplir para uno de los dos roles que nombra tiene un agujero exactamente donde esta el segundo nombre; se agrego el caso de admin Y el de volunteer, porque sin este ultimo el arreglo podria haber sido toda-membresia-es-privilegiada. Y el chequeo de active estaba sin testear: una invitacion invited que exija segundo factor deja afuera a alguien por un rol que TODAVIA NO TIENE, una denegacion de servicio que cualquiera dispara mandando una invitacion. Los dos cerrados, 7/7, cero residuo verificado con diff y sha256. UNA NOTA SOBRE FIXTURES: las dos brechas eran invisibles por la misma razon, el helper solo sabia construir owners. La fixture es el LIMITE de lo que un test puede decir, y el hueco no se ve leyendo los tests, se ve mutando el codigo. PRESUPUESTO: 115 de 250 y 662 de 800, entra en los dos sin excepcion; la implementacion es chica porque totp.go, recovery.go y envelope.go ya existian y estaban probados -- esta tarea es CABLEADO Y DECISIONES, no criptografia. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, 18 tests de login nombrados en -v con cero fallos, golangci-lint 0 issues, gofmt limpio verificado en contenedor. SIGUE T-02-030 (magic link, PR-02-16b): la INDISTINGUIBILIDAD entre una direccion conocida y una desconocida es requisito EXPLICITO del spec -- misma respuesta, mismo timing, y el correo sale solo si la cuenta existe. Rama a crear: feat/pr-02-16b-magic-link sobre feat/pr-02-16-totp-login. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`PR-02-12` COMPLETO el 2026-09-06** (`T-02-023` RED + `T-02-024` GREEN) en `feat/pr-02-12-middleware` — el **criterio de éxito de la fase**: el alcance de tenant sale solo del claim verificado, seis mutantes muertos. **227 impl / 675 total, entra en los dos presupuestos sin excepción** — la advertencia de riesgo que puse el 05-09 **no se cumplió** y queda tachada, no borrada, en `tasks.md`. **`T-02-025` cerrada el 2026-09-06** (`cors.go` + `csrf.go`, allowlist de orígenes + CSRF por `Origin`/`Sec-Fetch-Site`, siete mutantes muertos) en `feat/pr-02-13a-cors-csrf`. **`PR-02-13` se partió** en `PR-02-13a` (cerrado, 277/639) y `PR-02-13b` (`config.go`) — segundo corte de la fase tras `PR-02-16`, y las dos mitades entran en los dos presupuestos **sin excepción**. Sigue `/sdd-apply` desde `T-02-026`. **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
   el texto completo de cada decisión está versionado en `docs/vault/70-conocimiento/`.
3. Abrir `docs/vault/30-fases/FASE-<current_phase>.md`. Buscar el primer `[~]`; si no hay, el primer `[ ]`.
4. Leer el spec enlazado y la última entrada de `docs/vault/40-bitacora/`.
5. Confirmar la tarea al usuario en una línea. **Entonces** empezar.

## Ritual de cierre de tarea — obligatorio

1. Tests en verde, lint limpio.
2. Marcar `[x]`; mover el `[~]` a la siguiente tarea.
3. Actualizar este archivo, incluido `next_action`.
4. Añadir entrada a `docs/vault/40-bitacora/<fecha>.md`.
5. Evaluar candidatos contra `.engram/RUBRIC.md` → escribir a `.engram/queue/` → tras el sí del usuario y el `mem_save`, mover a `docs/vault/70-conocimiento/` → `make engram-index`.
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
