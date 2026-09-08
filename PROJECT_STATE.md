---
project: mascotapp
current_phase: "02"
current_task: "T-02-028"  # T-02-027 cerrada con size:exception (270/250 impl, 772/800 total): un solo handler, no dos unidades de revision, y el corte por defecto NO aplica -- medido antes de decidir
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-027 CERRADA con size:exception (270/250 impl, 772/800 total). T-02-028 (login, camino de contrasena, PR-02-15) es la siguiente; rama a crear: feat/pr-02-15-login sobre feat/pr-02-14-registration
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-08
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-027 CERRADA el 2026-09-08 (RED 0553351, GREEN f1d5bd1, test de fuga 964473a) en la rama feat/pr-02-14-registration sobre feat/pr-02-13b-config. apps/api/internal/httpapi/auth_handlers.go (237) + auth_handlers_test.go (398) + dos queries nuevas. 8 tests, todos contra contenedor. FALTABAN DOS QUERIES Y EL CONTRATO NO LAS NOMBRABA: ninguna sentencia del repo insertaba en users ni en shelters. InsertUser (auth.sql, app_auth, las cinco columnas de 00013, ON CONFLICT DO NOTHING que es lo que hace FUNCIONAR la compensacion no-hacer-nada de P2-D6 en vez de solo documentarla, y NO pisa password_hash) y CreateShelter (shelters.sql, app_tenant, status AUSENTE). EL ESQUEMA RECHAZO MI PRIMER GREEN Y TENIA RAZON: cinco tests con 42501 sobre memberships porque mi INSERT escribia accepted_at, que 00015 concede en UPDATE y NO en INSERT porque pertenece al flujo invitar-despues-aceptar. La correccion fue del HANDLER, no de la migracion: un fundador no acepto ninguna invitacion, asi que NULL es el valor honesto, y ampliar un grant para que entre el codigo propio es como se erosiona una barrera -- ademas es el grant exacto sobre el que se levanto B1. RE-LEER EL ID NO ES DEFENSIVO: ON CONFLICT DO NOTHING no escribe fila, asi que en el segundo registro de una direccion el id generado nombra un usuario inexistente y la membresia falla su FK; preguntarle a la base que id tiene esa direccion es la unica respuesta correcta en los dos casos, y sin eso la idempotencia existe en la query pero no llega al flujo. STATUS SE RECHAZA DOS VECES: el handler no tiene ese campo, y app_tenant no tiene privilegio de INSERT sobre shelters.status (00015), asi que una sentencia que lo nombrara la rechazaria el grant antes de que corriera ninguna policy. MUTACION -- SIETE MUTANTES, SEIS MUERTOS EN LA PRIMERA PASADA Y M6 SOBREVIVIO: devolver el texto de error del driver en vez de un mensaje fijo. Todos los tests afirmaban lo que se ESCRIBIO y ninguno leia el cuerpo de un FALLO, asi que el oraculo de enumeracion que el propio comentario del handler declaraba cerrado no tenia nada que lo verificara -- y es el endpoint ABIERTO A CUALQUIERA, donde PostgreSQL nombra constraint, tabla y valor que choco. Se agrego el test (registra dos veces el mismo slug, verifica que la respuesta no lleve ninguna de las seis cosas que el driver diria) y M6 muere. 7/7, cero residuo verificado con diff contra copia pristina. Tercera entrada del proyecto sobre un comentario que afirmaba de mas, y otra vez la agarro la mutacion. PRESUPUESTO -- SIZE:EXCEPTION ACEPTADA POR EL USUARIO el 2026-09-08: 270 de 250 en implementacion (8% de exceso), 772 de 800 en total. EL CORTE POR DEFECTO NO APLICA Y SE MIDIO ANTES DE DECIDIR: los dos cortes anteriores partieron PRs que llevaban DOS unidades de revision; este lleva UNA (un handler y las dos queries que solo el usa; de 237 lineas, 150 son codigo y 65 comentario). Partirlo dejaria una primera mitad que nadie llama, codigo muerto hasta que llegue la segunda. GOFMT Y GO VET LOCALES NO CORREN: Smart App Control los bloquea, y mi primer chequeo leyo la salida vacia como exito reportando limpio sobre un comando que nunca corrio; corrido en contenedor encontro internal/db/auth.go y auth_test.go sucios, DEUDA PREEXISTENTE de 6bffd3c que no se toca aca. sqlc local bloqueado por lo mismo, se corrio en contenedor. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los 8 tests nombrados en la salida -v, golangci-lint 0 issues, gofmt de internal/httpapi limpio verificado en contenedor. SIGUE T-02-028 (login, camino de contrasena, PR-02-15): contrasena correcta pasa el chequeo de credencial, la equivocada no emite sesion, y en exito Set-Cookie __Secure-mascotapp_refresh con HttpOnly Secure SameSite=None Path=/api/v1/auth (P2-D9) mas un access token de 15 minutos en el cuerpo. Rama a crear: feat/pr-02-15-login sobre feat/pr-02-14-registration. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS."
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
