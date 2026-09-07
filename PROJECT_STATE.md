---
project: mascotapp
current_phase: "02"
current_task: "T-02-025"  # T-02-024 cerrada y con ella PR-02-12 COMPLETO: 227 impl / 675 total, entra en los dos presupuestos
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-12 completo en feat/pr-02-12-middleware (31ca14c RED, 03db472 GREEN). Sigue T-02-025 (PR-02-13, cors.go + csrf.go)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-06
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "T-02-024 CERRADA el 2026-09-06, y con ella PR-02-12 COMPLETO -- el criterio de exito de la Fase 02. apps/api/internal/httpapi/middleware_auth.go, 227 lineas, rama feat/pr-02-12-middleware (commits 31ca14c RED y 03db472 GREEN). DOS MIDDLEWARES, NO UNO: RequireAuth verifica el bearer y pone los claims en contexto; RequireTenant resuelve el scope y abre la transaccion. Separados a proposito porque hay rutas autenticadas y SIN refugio (elegir refugio, leer tu propio perfil) que si no tendrian que inventarse un tenant, que es justo lo que este archivo existe para impedir. EL ORDEN DE LOS CHEQUEOS ES EL CONTRATO: 1) autenticado siquiera -> 500 porque es bug de cableado (contestar 403 esconderia el error detras de un rechazo plausible en TODOS los requests); 2) el claim nombra un refugio real, no-nil Y no-uuid.Nil -> 403; 3) nada en el request lo contradice (path via chi.URLParam(ShelterIDPathParam), query shelter_id, header X-Shelter-ID) -> 403; 4) RECIEN AHI WithTenant. Todo rechazo ocurre ANTES del paso 4, asi que un request rechazado no llega a la base. DETALLES QUE IMPORTAN: authContextKey es su PROPIO tipo, no el contextKey de clientip.go -- dos bloques iota sobre un mismo tipo comparten valores en silencio y la colision le entrega el valor de un middleware al lector de otro. Un identificador que NO PARSEA es un desacuerdo (403), nunca un 'no vino ninguno', porque lo segundo convertiria un path malformado en bypass del chequeo entero. 403 y no 404 porque esconder el recurso hace del limite un oraculo de existencia para el que adivina bien y sigue filtrando para el que adivina mal. TenantScoper es una costura: evita que httpapi importe un pool y permite que el espia asierte 'nunca invocado' de forma exacta. UN FIXTURE MAL, NO LA IMPLEMENTACION: el primer GREEN fallo un test y era mi token forjado, construido con amr vacio, que Issue rechaza desde T-02-014. Una falsificacion tiene que estar mal en EXACTAMENTE UNA cosa (la firma) o el test pasa por el motivo equivocado. MUTACION -- SEIS MUTANTES, SEIS MUERTOS, cero residuo verificado con diff byte a byte. Dos con nombre propio: (a) scope tomado SOLO del path lo mata UNICAMENTE el test de la ruta sin segmento de refugio, y los otros siete pasan -- la afirmacion que escribi en el RED quedo MEDIDA, no argumentada; (b) abrir la transaccion ANTES del chequeo de mismatch lo mata la asercion de 'nunca invocado', NO un status code, porque el status seguia siendo 403: esa es la diferencia entre un rechazo y una disculpa. UN SEPTIMO INTENTO QUE NO FUE MUTANTE: el de aceptar cualquier esquema de Authorization no compilaba (scheme sin usar), no imprimio ni FAIL ni ok, y mi filtro de rg no matchea un error de compilacion -- casi lo cuento como sobreviviente. Lo agarre porque un mutante que no imprime NADA es sospechoso de por si. Reescrito con _ corrio y murio. REGLA: un mutante que no compila no es un mutante que sobrevive, es un mutante que nunca existio. LA ADVERTENCIA DE PRESUPUESTO NO SE CUMPLIO: ayer marque que PR-02-12 iba camino a romper los dos limites porque T-02-023 sola media 445 sobre un est de 290. Medido: 227 implementacion de 250 y 675 total de 800, ENTRA EN LOS DOS SIN EXCEPCION, y la proyeccion total (684) le pego con 1,3% de error. La advertencia queda TACHADA, NO BORRADA, en tasks.md: era razonable con la evidencia que habia y el registro de una alarma falsa es lo que mantiene honesta a la proxima. Leccion angosta: un RED que se va a 3x su est NO DICE NADA sobre el GREEN, porque el est cuenta superficie y las dos mitades se pasan por motivos sin relacion. VERIFICACION: suite completa en contenedor con exit=0 capturado, siete paquetes verdes, los OCHO tests nombrados en la salida -v dentro del contenedor, golangci-lint 0 issues (hubo uno de revive: mi comentario de resumen quedo pegado al const y pasaba por doc comment, se despego con una linea en blanco), gofmt limpio, govulncheck sin cambios. Docker Desktop estaba caido otra vez al empezar; lo levante. LO QUE ESTE MIDDLEWARE TODAVIA NO HACE: NO esta montado en el router. El cableado de router.go es T-02-040, la ultima de la fase, exactamente donde el diseno lo pone. Hasta entonces es una garantia probada y no conectada. SIGUE T-02-025: cors.go + csrf.go (PR-02-13) -- allowlist con credenciales y nunca comodin, verificacion de Origin/Sec-Fetch-Site en todo metodo que cambia estado y en /auth/refresh especificamente, FAIL-CLOSED cuando no viene ninguno de los dos. Su dod exige mutacion: los mutantes de comodin-con-credenciales y de fail-open-sin-header tienen que morir. NADA PUSHEADO, CERO PRs ABIERTOS."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-020` **+ `T-02-022`** cerradas en local (PR-02-01…PR-02-11 y la primera mitad de `PR-02-16`; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. **`T-02-021` Judgment Day CERRADA el 2026-09-05: veredicto APPROVED** — encontró un defecto severo real (la revocación de familia no era atómica), cerrado en dos rondas y con test de regresión. **`T-02-022` cerrada el 2026-09-05** en `feat/pr-02-16-email` (158 impl / 469 total, entra en los dos presupuestos sin excepción). **`PR-02-16` se partió en `PR-02-16a`** (el puerto, merge en la **posición 12**) **y `PR-02-16b`** (el handler de magic link, posición 17) — la cadena de ramas queda en **una sola línea**, sin hermanas. **`PR-02-12` COMPLETO el 2026-09-06** (`T-02-023` RED + `T-02-024` GREEN) en `feat/pr-02-12-middleware` — el **criterio de éxito de la fase**: el alcance de tenant sale solo del claim verificado, seis mutantes muertos. **227 impl / 675 total, entra en los dos presupuestos sin excepción** — la advertencia de riesgo que puse el 05-09 **no se cumplió** y queda tachada, no borrada, en `tasks.md`. Sigue `/sdd-apply` desde `T-02-025` (`PR-02-13`, `cors.go` + `csrf.go`). **`PR-02-11` excede LOS DOS presupuestos** (implementación **495** de 250, total **1.152** de 800 tras la corrección de Judgment Day; la excepción se aceptó sobre 381/930) — **`size:exception` aceptada por el usuario el 2026-09-04**. **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
