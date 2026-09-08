---
project: mascotapp
current_phase: "02"
current_task: "T-02-027"  # T-02-026 cerrada (153/454) y con ella PR-02-13 ENTERO: 13a cors+csrf (277/639) y 13b config.go. El corte confirmado -- sin partir habria sido 430/250 y 1.093/800
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: not_started  # T-02-026 CERRADA. T-02-027 (handler de registro, PR-02-14) es la siguiente; rama a crear: feat/pr-02-14-registration sobre feat/pr-02-13b-config
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-07
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-026 CERRADA el 2026-09-07 (RED 528d8a7 + GREEN e142ad5) en la rama feat/pr-02-13b-config sobre main, que ya contiene PR-02-13a. Cierra PR-02-13 entero. apps/api/internal/config/config.go +153, config_test.go +301: DatabaseURLAuth, JWTSecret, AuthKEK, WebOrigins, APIPublicOrigin y el rechazo de arranque de P2-D9. 8 tests nuevos, 35 subtests. LA DEPENDENCIA: golang.org/x/net pasa a DIRECTA (v0.58.0) con aprobacion explicita del usuario bajo la regla de supply chain; ya estaba en el grafo de modulos como transitiva en v0.57.0, asi que promueve un modulo listado en vez de admitir uno nuevo, y solo se compila el paquete publicsuffix. EL FALLBACK REGISTRABLE_DOMAINS_DIFFER=true NO HIZO FALTA y era peor: convertia un chequeo calculado en uno declarado, y una declaracion equivocada arranca igual en silencio. govulncheck reporta 1 vulnerabilidad PREEXISTENTE que este cambio no trajo: golang.org/x/crypto@v0.56.0 (Argon2id), Fixed in N/A, y el codigo no la llama. LA MUTACION CORRIGIO MI PROPIO COMENTARIO: escribi que una comparacion ingenua de las dos ultimas etiquetas falla EN LAS DOS DIRECCIONES; medido, la matan solo dos subtests y los dos son la MISMA direccion. La comparacion ingenua SOBRE-AGRUPA (lee pages.dev y com.mx como dominios registrables), asi que colapsa inquilinos no relacionados en un solo dueno y se niega a arrancar sobre una configuracion CORRECTA; nunca se le escapa un parentesco real. Eso cambia que compra la dependencia: no compra tapar un agujero, compra no rechazar un deploy sano. El comentario ahora dice lo medido -- mismo patron que obs-9e00e3a6413dc7d2, y otra vez lo agarro la mutacion y no una lectura. TRES TRAMPAS PINNEADAS: AUTH_KEK se mide DESPUES de decodificar (32 caracteres base64 decodifican a 24 bytes, y un chequeo sobre len(raw) le entrega a AES-256-GCM una clave que tiene que rechazar); los valores de auth se validan SI ESTAN, no se exigen (un JWT_SECRET vacio es un secreto ausente, no uno debil, y exigirlo frenaria el runner de migraciones -- escrito COMO TEST, no como comentario); un origen que no se puede evaluar SE RECHAZA, no se saltea (saltearlo apaga el chequeo para ese valor mientras el proceso arranca sin decir nada, una exencion que nadie verifica derivando hacia lo permisivo). MUTACION: siete mutantes, siete muertos, cero residuo verificado con diff contra copia pristina y su sha256. El harness clasifica en TRES estados -- MUERTO, SOBREVIVIO y NO_COMPILA -- porque un mutante que no compila no imprime ni FAIL ni ok y un filtro de dos estados lo lee como la conclusion opuesta; el baseline sin mutar se corrio primero para comprobar el clasificador. PRESUPUESTO: 153 implementacion de 250, 454 total de 800, 1.18x contra su est de 130. CONFIRMA EL CORTE: sin partir, PR-02-13 habria medido 430/250 y 1.093/800, excepcion en los DOS; partido, las dos mitades entran sin ninguna. Segundo caso de la fase con el mismo resultado. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los 8 tests nuevos nombrados en la salida -v dentro del contenedor con 35 subtests, golangci-lint 0 issues, gofmt limpio, go mod tidy deja x/net en el bloque directo sin // indirect. SIGUE T-02-027 (handler de registro, PR-02-14): primera tarea que escribe auth_handlers.go; el shelter aterriza en pending_verification IGNORANDO cualquier status que mande el llamador, y el flujo de dos transacciones de P2-D6 (WithAuthUser inserta users idempotente sobre el UNIQUE citext, despues WithTenant inserta shelters Y la membresia owner del fundador atomicamente), con la compensacion no-hacer-nada aseverada directamente. Rama a crear: feat/pr-02-14-registration sobre feat/pr-02-13b-config. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS."
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
