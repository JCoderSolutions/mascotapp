---
project: mascotapp
current_phase: "02"
current_task: "T-02-017"  # T-02-014 y T-02-015 (PR-02-09) cerradas en local, branch feat/pr-02-09-token; excede los dos presupuestos, size:exception ACEPTADA por el usuario el 2026-09-04
sdd_change: "phase-02-auth-and-multitenancy"
rdd_enabled: false  # fase marcada RDD; el usuario decide, no se activa solo
task_status: in_progress  # PR-02-01..PR-02-09 verdes en local. Sigue T-02-017 (PR-02-10, recovery.go)
blocked_by: ".env.example: denegado a los agentes por regla global. Bloquea SOLO T-02-035 (PR-02-24, ultimo de la cadena). El usuario tiene que confirmar si JWT_SECRET y RESEND_API_KEY ya existen y pegar DATABASE_URL_AUTH, AUTH_KEK, WEB_ORIGINS, API_PUBLIC_ORIGIN y el password de app_auth. La cadena funcional entera mergea sin eso."
last_updated: 2026-09-04
sdd_store: hybrid
last_agent: claude-opus-5
next_action: "PR-02-09 (T-02-014 + T-02-015) CERRADO EN LOCAL el 2026-09-04, rama feat/pr-02-09-token sobre feat/pr-02-08b-totp, commits ac1e100 (RED, 515), a2ae1e4 (RED ampliado, +45) y el GREEN. token.go: el access token de identity-and-session y P2-D11, HS256, 15 minutos, claim shelter_id. DEPENDENCIA INSTALADA CON APROBACION EXPLICITA DEL USUARIO: github.com/golang-jwt/jwt/v5 v5.3.1. Huella minima: un require directo, dos lineas de go.sum, nada transitivo -- el jwt v3 vulnerable que se bajo durante la resolucion NO quedo en el grafo, go mod tidy lo saco. govulncheck ./... limpio; el unico hallazgo de modulo es golang.org/x/crypto/openpgp (sin mantenimiento, Fixed in N/A), preexistente via Argon2id y nunca importado. POR QUE LIBRERIA ACA Y STDLIB EN totp.go: JOSE falla las tres condiciones de la desviacion de T-02-013 --familias de algoritmos, modos de confusion conocidos, historia ACTIVA de vulnerabilidades de implementacion. Parsear tokens de un atacante es justo el trabajo que una libreria mantenida debe hacer. EL TEST DECODIFICA A MANO igual: parte en punto, base64url, y recalcula el MAC con crypto/hmac. Parsear con la misma libreria con la que se escribe la implementacion prueba un ROUND-TRIP, no correccion. HALLAZGO DE LA MUTACION, EL MAS IMPORTANTE DE LA TAREA: el primer mutante --sacar jwt.WithValidMethods-- SOBREVIVIO. Mis dos tests de falsificacion (alg none y alg HS512) estaban rojos por la razon equivocada: los payloads forjados no tenian amr, asi que los rechazaba mi propia validacion de claims a la salida, no el chequeo de algoritmo. CUARTA instancia del patron ya guardado en obs-9e00e3a6413dc7d2 (la capa que contesta primero), pero con un giro NUEVO: las tres anteriores fueron en tests de base de datos y la capa intrusa era del motor; esta fue en Go puro y la capa intrusa fue MI PROPIA defensa en profundidad. Agregar un chequeo redundante puede desarmar en silencio el test del chequeo de abajo. Corregido con forgeablePayload(), un payload COMPLETO Y VALIDO donde lo unico mal es el header. Con eso el mutante muere, y lo mata exactamente el caso de HS512 -- alg none sigue pasando sin el allowlist porque la libreria lo rechaza por su cuenta, lo que confirma que el allowlist es load-bearing SOLO para HS512. Candidato de memoria PENDIENTE DE APROBACION en .engram/queue. PRESUPUESTO: EXCEDE LOS DOS. Implementacion 289 de 250 (39 over); total 935 de 800 (135 over). De las 289 de token.go solo 136 son codigo: 120 son comentario y 33 blancos. Es exactamente lo que anote un PR antes --que el factor 2,36x es un piso y no un centro-- y paso en el PR siguiente. NECESITA size:exception DEL USUARIO. RED y GREEN no se pueden separar sin mergear un arbol que no compila. SIGUE T-02-017: RED de recovery.go, rama feat/pr-02-10-recovery sobre feat/pr-02-09-token. NADA PUSHEADO, CERO PRs ABIERTOS. Judgment Day sigue pendiente antes de mergear PR-02-11 (session.go)."
---

# Estado del proyecto

## Resumen en una línea

**Fase 02 (Auth y multi-tenancy) EN CURSO**: `T-02-001`…`T-02-015` y `T-02-016` cerradas en local (PR-02-01…PR-02-09; **rebanadas (a) y (b) cerradas**, y con ellas el último alcance heredado de la Fase 01), ninguno pusheado ni con PR abierta — eso lo gatea el usuario. `T-02-016` (migración `00014_totp_recovery_codes`) se entregó en `PR-02-04`, adelantada de la rebanada (c) por el invariante de numeración de migraciones — ver la nota de la sección **Entrega** abajo. **40 tareas** `T-02-001`…`040`, entregadas en **24 PRs encadenados** (4.890 líneas, ninguno sobre 400) por decisión del usuario del 2026-09-02. Judgment Day va antes del merge del `PR-02-11` (`session.go`) y los 13 PRs siguientes están detrás de él. Sigue `/sdd-apply` desde `T-02-017` (`PR-02-10`, `recovery.go`). **`PR-02-09` excede LOS DOS presupuestos** (implementación 289 de 250, total 935 de 800) — **`size:exception` aceptada por el usuario el 2026-09-04**, la primera que rompe también el límite de implementación. **Arrancá con `make doctor`** — preflight nuevo del 2026-09-03, paso 0 del ritual. `gentle-ai` **fue reinstalado el 2026-09-03** (v1.49.0) y el dispatcher SDD está sano (`apply: ready`). Queda un solo `WARN`: `goose`, que no bloquea nada del trabajo. **`gentle-ai` queda fijado en 1.49.0 hasta cerrar la Fase 02** (decisión del usuario del 2026-09-03): está tres majors atrás y no tiene `review` ni `sdd-attempt`, pero eso es irrelevante mientras RDD siga apagado, y subir de major a mitad de la cadena arriesga el store del runtime SDD. **No subirlo.** **Fase 01 CERRADA Y ARCHIVADA** (35/35). Fase 00 queda en 21/23 con 2 tareas del usuario que no bloquean nada.

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
