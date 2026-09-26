---
project: mascotapp
current_phase: "02"
current_task: "T-02-032"  # T-02-031 cerrada (226/698). Refresh emite amr [pwd] por decision del usuario; logout revoca la FAMILIA bajo el lock
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # T-02-032 (enrolamiento y verificacion de TOTP, PR-02-18) tiene RED (ca88072, 2026-09-24) y GREEN (27b2032, 2026-09-25) ya commiteados en feat/pr-02-18-totp-enrol sobre feat/pr-02-17-refresh. Falta: mutation testing, verificacion de suite/lint en contenedor y el commit docs de cierre.
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-14
sdd_store: hybrid
last_agent: claude-opus-5
next_action:
  cerrado: >
    T-02-031 CERRADA el 2026-09-14 (RED 452d73c, GREEN b04c74e, tests del
    mutante partido 960ef4f) en feat/pr-02-17-refresh sobre
    feat/pr-02-16b-magic-link. refresh.go nuevo + correcciones en login.go y
    auth_handlers.go; 12 tests de refresh/logout contra contenedor.
    MUTACION: 9 mutantes, 9 muertos, cero residuo, clasificador con chequeo
    de build primero. PRESUPUESTO: 226 de 250 y 698 de 800, entra en los
    dos. VERIFICACION: suite completa en contenedor con exit=0 capturado,
    siete paquetes verdes, golangci-lint 0 issues, gofmt limpio en
    contenedor. NADA PUSHEADO DE ESTA RAMA, CERO PRs ABIERTOS.
  decision: >
    DECISION DEL USUARIO 2026-09-14 -- AMR DE UN REFRESH ES [pwd]:
    refresh_tokens no guarda como se autentico la sesion; preservar otp
    pedia migracion Y hacia que una cookie de refresh robada portara el
    segundo factor toda la vida de la familia. Con [pwd] un refresh nunca
    afirma un factor que no verifico. Test pinneado con un OWNER que entro
    con codigo TOTP valido, el caso tentador. EL ORDEN DEL CHEQUEO DE
    ORIGEN SE PRUEBA SIN ESPIA: origen rechazado + cookie malformada da 403
    no 401; origen rechazado + cookie valida la deja utilizable despues.
    LOGOUT REVOCA LA FAMILIA BAJO EL LOCK, pinneado por test antes del
    codigo: revocar solo la fila presentada no hace nada con una cookie
    vieja y el sucesor sobrevive; el mismo hueco aparece sin cookie vieja si
    un refresh esta en vuelo. Falla de infraestructura en logout responde
    500 SIN limpiar la cookie. DOS CORRECCIONES A TRABAJO ANTERIOR:
    serverError respondia y logueaba registration failed para TODO
    llamador, asi que los 500 de login decian eso desde T-02-028 (verificado
    que ningun test dependia del texto; ahora generico con path en el log);
    y los atributos de la cookie vivian solo en login, ahora en
    setRefreshCookie compartido. UN MUTANTE COMBINADO ESCONDIA UNA CLAUSULA
    SIN PROBAR: la rama que convierte uuid-cero y carrera-perdida en 401 se
    muto como uno solo; el uuid cero es deterministicamente alcanzable (16
    bytes en cero parsean como uuid) y un test solo para el habria matado
    al mutante combinado dejando la carrera sin probar bajo un reporte
    verde. Partido en M7a y M7b con un test cada uno; el de carrera espeja
    el test concurrente de auth (pruebas repetidas, largada comun) y su
    muerte se corrio DOS veces porque una muerte dependiente del tiempo
    observada una vez prueba poco.
  sigue: >
    T-02-032 (enrolamiento y verificacion de TOTP, PR-02-18) EN CURSO en
    feat/pr-02-18-totp-enrol sobre feat/pr-02-17-refresh: RED (ca88072,
    2026-09-24) y GREEN (27b2032, 2026-09-25) ya commiteados -- vuelve
    alcanzable desde HTTP el totp_secret_enc que hoy los tests escriben a
    mano. Falta antes de cerrarla: mutation testing, suite/lint en
    contenedor, entrada de bitacora y el commit docs de cierre.
  bloqueadores: >
    Ninguno. Los tres candidatos de memoria que estaban pendientes en
    .engram/queue (muerto-falso, doble-permisivo, mutante-combinado) fueron
    aprobados por el usuario el 2026-09-24 y guardados en Engram
    (obs-0210963cf0f98b48, obs-296d31be8cc336f7 -- este ultimo actualiza en
    el mismo lugar la entrada previa que extendia --, obs-a13da1b17fdd6562);
    movidos a docs/vault/70-conocimiento/ y referenciados de vuelta en
    tasks.md y en el tablero de FASE-02.md.
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`PR-02-12` COMPLETO el 2026-09-06** (`T-02-023` RED + `T-02-024` GREEN) en `feat/pr-02-12-middleware` — el **criterio de éxito de la fase**: el alcance de tenant sale solo del claim verificado, seis mutantes muertos. **227 impl / 675 total, entra en los dos presupuestos sin excepción** — la advertencia de riesgo que puse el 05-09 **no se cumplió** y queda tachada, no borrada, en `tasks.md`. **`T-02-025` cerrada el 2026-09-06** (`cors.go` + `csrf.go`, allowlist de orígenes + CSRF por `Origin`/`Sec-Fetch-Site`, siete mutantes muertos) en `feat/pr-02-13a-cors-csrf`. **`PR-02-13` se partió** en `PR-02-13a` (cerrado, 277/639) y `PR-02-13b` (`config.go`) — segundo corte de la fase tras `PR-02-16`, y las dos mitades entran en los dos presupuestos **sin excepción**. Sigue `/sdd-apply` desde `T-02-026`. **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

## Invariantes

1. **Como máximo UNA tarea en estado `[~]`** en todo el tablero. Si encontrás dos, es un error: resolvelo antes de trabajar.
2. **Ninguna tarea pasa a `[x]`** sin test en verde, lint limpio y `PROJECT_STATE.md` actualizado.
3. **El avance vive acá, no en Engram.** Engram guarda *por qué*, nunca *qué se hizo*.
4. **Los artefactos técnicos se escriben en inglés** (código, identificadores, columnas, endpoints, commits, tests). La documentación del vault va en español.

## Ritual de inicio de sesión

Ver [AGENTS.md](AGENTS.md) §2 "Ritual de inicio de sesión — obligatorio" — no
se repite acá (fuente única de reglas de proceso).

## Ritual de cierre de tarea

Ver [AGENTS.md](AGENTS.md) §6 "Ritual de cierre de tarea — obligatorio" — no
se repite acá (fuente única de reglas de proceso).

## Enlaces

- **Contrato de trabajo para cualquier agente: [AGENTS.md](AGENTS.md)**
- Plan maestro: [docs/vault/10-propuesta/analisis-y-plan.md](docs/vault/10-propuesta/analisis-y-plan.md)
- Tablero de la fase actual: [docs/vault/30-fases/FASE-02.md](docs/vault/30-fases/FASE-02.md)
- Change SDD activo: [openspec/changes/phase-02-auth-and-multitenancy/](openspec/changes/phase-02-auth-and-multitenancy/)
- Decisiones: [docs/vault/20-arquitectura/](docs/vault/20-arquitectura/)
- Índice de memoria (sin MCP): [docs/vault/20-arquitectura/indice-engram.md](docs/vault/20-arquitectura/indice-engram.md)
- Rúbrica de memoria: [.engram/RUBRIC.md](.engram/RUBRIC.md)
