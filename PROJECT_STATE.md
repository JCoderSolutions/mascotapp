---
project: mascotapp
current_phase: "02"
current_task: "T-02-026"  # T-02-025 cerrada (cors.go + csrf.go). BLOQUEADA: PR-02-13 ya paso el limite de implementacion, el usuario decide entre size:exception y partir el PR
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: blocked  # T-02-026 espera la decision de presupuesto de PR-02-13. Ver blocked_by
blocked_by: "(1) PR-02-13: T-02-025 sola mide 277 de 250 lineas de implementacion, con T-02-026 sin escribir. El usuario decide entre size:exception y partir el PR (cors/csrf y config no dependen entre si). BLOQUEA T-02-026. (2) .env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-06
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-025 CERRADA el 2026-09-06 (RED+GREEN en la misma tarea, dos commits) en la rama feat/pr-02-13-http-security sobre feat/pr-02-12-middleware: fae4d87 RED y 06646ca GREEN. apps/api/internal/httpapi/cors.go (190) + csrf.go (87) + cors_csrf_test.go (362). 14 tests, 42 casos con subtests. EL SPEC Y EL DISENO SE CONTRADICEN Y GANA EL DISENO, ANOTADO NO INFERIDO: el requisito del spec pide 'un token CSRF valido'; P2-D9, escrito despues, RECHAZA el double-submit y lo reemplaza por verificacion de Origin/Sec-Fetch-Site (el navegador pone Origin en cross-site y JavaScript no lo puede forjar; la allowlist es la que CORS ya usa; no hay nada que guardar, rotar ni que un arranque en frio resetee). El tercer escenario se implementa y testea como 'Origin/Sec-Fetch-Site faltante o en desacuerdo'. Cambio el mecanismo, no la propiedad, y esta escrito en el encabezado del archivo de test. LA TRAMPA ALREDEDOR DE LA CUAL ESTA CONSTRUIDO TODO: los origenes separados estan decididos (Decision 4), asi que TODOS los requests legitimos del navegador llegan con Sec-Fetch-Site: cross-site. Un chequeo escrito de la forma obvia ('rechaza cross-site') rechaza el 100% del trafico real y lee, para un revisor, exactamente como lo que una defensa CSRF deberia decir. Origin ES el control; Sec-Fetch-Site es SOLO el respaldo para un request que legitimamente no trae Origin. MEDIDO, NO ARGUMENTADO: el mutante que implementa esa regla lo mata UN SOLO test, el que escribi para el; los otros trece pasan mientras la API rechaza todo. Mismo patron que ayer con el test de la ruta sin segmento de refugio -- el test que justifica su existencia es el que mata un mutante que ningun otro mata. UN ORIGIN PRESENTE LO CONTESTA LA ALLOWLIST Y NADIE MAS: si viene y no esta en la lista es 403, NO se cae a mirar Sec-Fetch-Site, porque solo los navegadores estan atados a la regla de forbidden header names y un cliente que no es navegador pondria Sec-Fetch-Site: same-origin para rescatar un origen que la allowlist acaba de rechazar. CORS RECHAZA DEL LADO DEL SERVIDOR, que es MAS de lo que CORS es: CORS estandar es un aviso, el servidor describe y el NAVEGADOR hace cumplir, y un cliente que no es navegador ignora las cabeceras. El spec pide rechazo antes del handler, o sea un control y no una descripcion. Lo que NO rechaza es un request SIN Origin: eso no es un request CORS, y rechazarlo romperia server-to-server, health probes y curl. DETALLES: Allows es igualdad exacta de strings (HasSuffix matchea https://evil.app.mascotapp.test, Contains matchea cualquier cosa, normalizar inventa un match que el navegador nunca mando). Vary: Origin se escribe ANTES QUE NADA, rechazos y camino sin Origin incluidos, porque sin eso un cache compartido le sirve a un origen la cabecera Access-Control-Allow-Origin de otro. UNA SOLA OriginAllowlist compartida por los dos middlewares, no por economia sino porque P2-D9 eligio Origin sobre double-submit precisamente porque la lista que CORS ya necesita es tambien la respuesta a 'vino de nosotros'; dos listas divergirian en silencio. La allowlist rechaza EN CONSTRUCCION el comodin (ilegal con credenciales: DESACTIVA la allowlist en vez de ampliarla), null (iframe sandboxeado), http://, y cualquier cosa con path/query/userinfo que un navegador nunca manda. MUTACION -- SIETE MUTANTES, SIETE MUERTOS, cero residuo verificado con diff byte a byte sobre los dos archivos: comodin-con-credenciales y fail-open-sin-cabeceras (los dos que exige el dod), HasSuffix por igualdad, rechaza-cross-site, Origin-rechazado-cae-a-Sec-Fetch-Site, se saca Vary, y metodos seguros gateados. ADVERTENCIA DE PRESUPUESTO QUE ESTA VEZ NO ES PROYECCION SINO MEDICION: PR-02-13 tiene limite 250 implementacion y 800 total; T-02-025 SOLA mide 277 implementacion y 639 total, con T-02-026 (config.go, est 130) TODAVIA SIN ESCRIBIR. El limite de implementacion YA ESTA ROTO; el total todavia tiene aire. Dos salidas razonables: (1) size:exception como en los seis PRs anteriores que la necesitaron, o (2) PARTIR EL PR como se hizo con PR-02-16 -- cors.go/csrf.go por un lado y config.go por el otro, que NO dependen entre si porque config.go solo va a leer WEB_ORIGINS y pasarselo a NewOriginAllowlist, que ya existe y esta probado. SE LE PREGUNTO AL USUARIO ANTES DE ARRANCAR T-02-026, no despues. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los 14 tests nombrados en la salida -v dentro del contenedor con 28 subtests, golangci-lint 0 issues, gofmt limpio, govulncheck 0 vulnerabilidades. La corrida local dio ok con exit=1 por el unlinkat de Windows sobre el .test.exe con cero --- FAIL, verificado LEYENDO el archivo. NO ESTA CABLEADO AL ROUTER: eso es T-02-040, la ultima de la fase. SIGUE T-02-026 (config.go: DSN de auth, JWT_SECRET, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el rechazo EN EL ARRANQUE si SameSite=None con dominio registrable compartido, via golang.org/x/net/publicsuffix -- OJO: agregar esa dependencia directa cae en la lista ask de supply chain y hay que preguntar; el fallback documentado en P2-D9 es un REGISTRABLE_DOMAINS_DIFFER=true explicito con el mismo rechazo de arranque). BLOQUEADA hasta que el usuario decida entre size:exception y partir el PR. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`PR-02-12` COMPLETO el 2026-09-06** (`T-02-023` RED + `T-02-024` GREEN) en `feat/pr-02-12-middleware` — el **criterio de éxito de la fase**: el alcance de tenant sale solo del claim verificado, seis mutantes muertos. **227 impl / 675 total, entra en los dos presupuestos sin excepción** — la advertencia de riesgo que puse el 05-09 **no se cumplió** y queda tachada, no borrada, en `tasks.md`. **`T-02-025` cerrada el 2026-09-06** (`cors.go` + `csrf.go`, allowlist de orígenes + CSRF por `Origin`/`Sec-Fetch-Site`, siete mutantes muertos) en `feat/pr-02-13-http-security`. **⚠️ `T-02-026` BLOQUEADA**: `PR-02-13` ya rompió el límite de implementación con una sola tarea (277 de 250) — el usuario decide entre `size:exception` y partir el PR. **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
