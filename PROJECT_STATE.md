---
project: mascotapp
current_phase: "02"
current_task: "T-02-014"  # T-02-012 y T-02-013 (PR-02-08b) cerradas en local, branch feat/pr-02-08b-totp; entra en los dos presupuestos nuevos sin excepcion
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-08b verdes en local. Sigue T-02-014 (PR-02-09, token.go / JWT)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "PR-02-08b (T-02-012 + T-02-013) CERRADO EN LOCAL el 2026-09-04, rama feat/pr-02-08b-totp sobre feat/pr-02-08a-envelope, commits e7d90d2 (RED, 288) y el GREEN (192). totp.go: el segundo factor de §5.2 para owner y admin. SEGUNDO PR BAJO LA REGLA NUEVA Y TAMBIEN SIN EXCEPCION: implementacion 192 de 250, diff total 480 de 800. El estimador re-baselineado acerto: predijo 190 de implementacion contra 192 medidos, 1 por ciento. El total sigue cayendo corto (proyecto 448, medi 480) y en PR-02-08a tambien (proyecto 472, medi 565): el factor 2,36x es un PISO, no un centro, asi que el proximo PR que proyecte cerca de 800 hay que tratarlo como si ya lo pasara. DESVIACION DE DISENO APLICADA Y DOCUMENTADA: P2-D8 mandaba github.com/pquerna/otp y rechazaba el HMAC propio con el argumento de que la cripto que escribis es cripto que mantenes para siempre. Se revirtio y se construyo sobre la biblioteca estandar: crypto/hmac + crypto/sha1 + net/url, go.mod y go.sum SIN TOCAR, cero dependencias nuevas. Tres condiciones tenian que darse a la vez y se dieron: RFC 6238 esta congelado desde 2011 asi que no hay mantenimiento que heredar, sus vectores del Apendice B fijan la correccion DESDE AFUERA de este repo, y la dependencia caia en internal/auth donde un compromiso no se recupera. SI FALTA CUALQUIERA DE LAS TRES GANA LA REGLA ORIGINAL: esto NO es precedente para el JWT de T-02-014, donde P2-D11 sigue mandando golang-jwt/v5 porque JOSE falla las tres. Razonamiento completo en docs/vault/20-arquitectura/desviacion-p2-d8-totp-stdlib.md y P2-D8 quedo enmendado en design.md para que nadie en frio corra go get. LOS VECTORES DEL RFC SE COMPUTARON, NO SE RECORDARON: un programa aparte de crypto/hmac + crypto/sha1 reprodujo las formas de 8 digitos que publica el RFC antes de fijar las de 6. Fijar una constante de memoria es exactamente como un test termina aseverando el numero equivocado y despues se arregla para que coincida con una implementacion mal. MUTACION, dos mutantes: ampliar la ventana de tolerancia de uno a dos pasos mata EXACTAMENTE two_steps_early y two_steps_late y ningun otro caso; fijar en cero el offset de truncacion dinamica mata los cinco vectores del RFC y dispara primero el guard de anti-vacuidad de RejectsACodeFromAnotherSecret. LO QUE ESTO NO RESUELVE Y ESTA ESCRITO EN EL CODIGO: VerifyTOTP no hace que un codigo sea de un solo uso. Un codigo presentado dos veces dentro de su propia ventana se acepta dos veces, y rechazarlo necesita un registro de codigos gastados que es del handler de login, pendiente para PR-02-11. SIGUE T-02-014: RED de token.go (JWT), rama feat/pr-02-09-token sobre feat/pr-02-08b-totp. NADA PUSHEADO, CERO PRs ABIERTOS: la cadena entera vive en local y el push esta gateado por el usuario. Judgment Day sigue pendiente antes de mergear PR-02-11 (session.go, rotacion y deteccion de reuso)."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-013` y `T-02-016` cerradas en local (PR-02-01…PR-02-08b; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue `/sdd-apply` desde `T-02-014` (`PR-02-09`, `token.go` / JWT). **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
