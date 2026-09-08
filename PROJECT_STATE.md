---
project: mascotapp
current_phase: "02"
current_task: "T-02-029"  # T-02-028 cerrada (215/693, entra en los dos). Login emite RoleUnscoped, no el rol de la unica membresia del fundador
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-029 (TOTP obligatorio + codigos de recuperacion, PR-02-16) es la siguiente y es el CRITERIO E2E DE LA FASE; rama a crear: feat/pr-02-16-totp-login sobre feat/pr-02-15-login
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-08
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-028 CERRADA el 2026-09-08 (RED 1782256, GREEN 46b8064, tests de los sobrevivientes d41a651) en feat/pr-02-15-login sobre feat/pr-02-14-registration. login.go (215) + login_test.go (478), 10 tests contra contenedor. ISSUE RECHAZO EL PRIMER GREEN Y ENCONTRO UNA TENSION QUE EL CONTRATO NO RESOLVIA: validate exige un role no vacio (RBAC lo lee, y '' es ambiguo entre sin-permisos y no-seteado), pero un rol es propiedad de una MEMBERSHIP y una membership es de un refugio, y este token a proposito no nombra ninguno. El atajo tentador -- leer la unica membresia del fundador y copiar su rol -- produce un token que afirma owner SIN refugio que lo acote, la forma exacta que POST /auth/session/shelter existe para impedir. LA SALIDA FUE NOMBRAR EL ESTADO: httpapi.RoleUnscoped, que no es un rol sin permisos sino la afirmacion de que todavia no se reclamo ningun rol con tenant; cualquier chequeo de RBAC que lo matchee es un bug en ESE chequeo. Hay test que lo fija y que ademas nombra los cuatro valores que produciria el atajo. DOS MUTANTES SOBREVIVIERON Y ERAN EL MISMO DEFECTO -- enumeracion de cuentas. M7 (el rechazo dice cual chequeo fallo) sobrevivio A MI PROPIO TEST DE INDISTINGUIBILIDAD, y el defecto estaba en el test: comparaba los dos rechazos ENTRE SI, y los dos pasan por unauthorizedLogin, UNA sola funcion, asi que cambiarla cambia los dos a la vez y siguen identicos. AFIRMABA UNA RELACION Y NUNCA UN CONTENIDO. Ahora nombra nueve palabras que contestarian la pregunta. M8 (return temprano si la cuenta no existe) sobrevivio a TODOS los tests porque las respuestas siguen siendo byte a byte iguales: lo que cambia es EL TIEMPO -- una direccion desconocida sale enseguida, una conocida paga Argon2id completo (64 MiB, tres pasadas, ~100ms), y un cronometro lee eso sin leer una sola respuesta. Cerrado con un hash dummy real contra el que se compara siempre. El test se escribio como RAZON (floor := knownAccount/10) y no como presupuesto fijo en milisegundos, que fallaria en una maquina mas lenta y se aflojaria hasta no significar nada; mediana de cinco intentos. MUTACION: 8 mutantes, 8 muertos, cero residuo verificado con diff y sha256. UN NOLINT DE SEGURIDAD CON SU RAZON Y SU TEST: golangci-lint marco G124 sobre SameSite=None; es falso positivo real -- Secure y HttpOnly estan los dos puestos y None es DECISION de P2-D9, no omision, y con origenes separados Lax o Strict nunca mandarian la cookie. Sigue el precedente de bootstrap.go: el nolint NOMBRA el test que afirma la propiedad en vez de sostenerse solo, y config.Load es la segunda red si los origenes convergen. PRESUPUESTO: 215 de 250 y 693 de 800, entra en los dos sin excepcion. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los 10 tests nombrados en -v, golangci-lint 0 issues, gofmt limpio verificado EN CONTENEDOR (local sigue bloqueado por Smart App Control). SIGUE T-02-029 (TOTP obligatorio para owner/admin + respaldo por codigo de recuperacion, PR-02-16): es el CRITERIO E2E DE LA FASE -- un owner con TOTP enrolado que manda solo la contrasena correcta NO obtiene sesion. Cinco escenarios. Rama a crear: feat/pr-02-16-totp-login sobre feat/pr-02-15-login. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS."
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
