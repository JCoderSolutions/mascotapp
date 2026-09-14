---
project: mascotapp
current_phase: "02"
current_task: "T-02-031"  # T-02-030 cerrada (164/651) y con ella PR-02-16 entero. El clasificador de mutacion tenia un falso-muerto: go test sobre build failure imprime FAIL [build failed]
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-031 (refresh + logout, PR-02-17) es la siguiente; cablea la rotacion y la deteccion de reutilizacion de session.go. Rama a crear: feat/pr-02-17-refresh sobre feat/pr-02-16b-magic-link
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-14
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-030 CERRADA el 2026-09-14 (RED b505400, GREEN 4578379, tests de los sobrevivientes af6f42d) en feat/pr-02-16b-magic-link sobre feat/pr-02-16-totp-login. Cierra PR-02-16 entero, partido el 2026-09-05. magic_link.go +164, magic_link_test.go +487, 8 tests contra contenedor. EL ENVIO ES ASINCRONO Y ESO ES EL DISENO: uno sincrono hace que el camino CON cuenta pague un viaje al proveedor de correo que el camino SIN cuenta nunca paga, y eso es un oraculo de tiempo leido a traves de dos cuerpos identicos. Obliga a context.WithoutCancel porque escribir la respuesta cancela r.Context() y email.checkSend refusa un contexto cancelado, asi que pasarle el contexto del request a la goroutine convertiria todo envio en un no-op silencioso que ademas parece funcionar. HUECO DE ALCANCE ESCRITO EN EL COMENTARIO DEL HANDLER, NO SOLO ACA: esta fase NO tiene endpoint de redencion ni tabla para estos tokens (T-02-031 es refresh, T-02-032 es enrolamiento de TOTP, nada consume un magic link). El token es inadivinable y HOY IRREDIMIBLE; el uso unico no se hace cumplir y la vigencia que el correo anuncia tampoco. Guardar un hash compra las dos cosas y PERTENECE al endpoint que lo redime, asi que esa decision queda para esa tarea en vez de adivinarse. MUTACION -- TRES DE SIETE SOBREVIVIERON LA PRIMERA PASADA Y DOS ERAN DEFECTOS DE LOS TESTS: el extractor de token tomaba todo lo que seguia al ultimo / y se tragaba la prosa posterior al enlace, asi que CUALQUIER token medía largo y uno de ocho caracteres pasaba; y los dobles ignoraban el contexto cancelado que el puerto real refusa -- un doble mas permisivo que aquello que reemplaza vuelve INTESTABLE el bug que esconde. El tercero era un caso faltante de cuenta suspendida. Y LA OTRA MITAD DEL SEGUNDO NO ESTABA EN EL DOBLE SINO EN EL ARNES: httptest.NewRecorder NUNCA cancela el contexto del request porque no tiene servidor que lo haga, asi que el handler que le pasa el contexto del request a una goroutine que sobrevive a la respuesta se ve sano bajo todos los tests y en produccion no manda un solo correo, sin error, sin log y con un 202 al llamador; el test nuevo corre contra un httptest.Server real. EL HALLAZGO QUE MAS VALE -- EL CLASIFICADOR MENTIA: go test sobre un error de compilacion imprime FAIL <pkg> [build failed], y un filtro que chequea ^FAIL ANTES que el build lo lee como mutante muerto. DOS DE LOS SIETE ERAN FALSOS MUERTOS. El proyecto ya tiene anotada la lectura opuesta (un mutante que no compila tomado por sobreviviente); el falso muerto es PEOR y la asimetria importa: un sobreviviente falso hace ruido y te manda a escribir un test que no hacia falta, un muerto falso es SILENCIOSO y reclama cobertura que no existe. El clasificador ahora chequea el build PRIMERO y su caso por defecto es INDETERMINADO. Tambien: el mutante del envio sincrono sobrevivio la primera vez PORQUE EL DOBLE ERA RAPIDO -- con un sender en memoria sincrono y asincrono cuestan lo mismo; la propiedad no es que el envio sea rapido sino que la respuesta no dependa del envio, y solo un sender LENTO las distingue (el test nuevo usa uno de 500ms). 7/7 muertos, cero residuo verificado con diff. PRESUPUESTO: 164 de 250 y 651 de 800, entra en los dos sin excepcion. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los 8 tests de magic link nombrados en -v con cero fallos, golangci-lint 0 issues, gofmt limpio verificado en contenedor. SIGUE T-02-031 (refresh + logout, PR-02-17): cablea la rotacion y la deteccion de reutilizacion de session.go, que ya existe y paso por Judgment Day en T-02-021. Rama a crear: feat/pr-02-17-refresh sobre feat/pr-02-16b-magic-link. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS."
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
