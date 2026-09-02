# FASE 00 — Fundaciones

**Objetivo:** dejar el proyecto en condiciones de que cualquier agente, en una sesión
nueva y en frío, sepa exactamente dónde retomar — y no pueda hacer daño mientras trabaja.

**Entregable verificable:** monorepo, CI, vault de Obsidian, `PROJECT_STATE.md`,
cola de Engram, SDD + CodeGraph inicializados, ADR-0001..0006, barreras de supervisión
probadas, y límites de free tier **verificados** (no asumidos).

**RDD:** no aplica en esta fase.

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Tareas

- [x] **T-00-001** · Estructura del monorepo (`apps/api`, `apps/web`, `docs/vault`)
      - dod: árbol creado · `.gitignore` en su lugar · repo git inicializado (`main`)
      - engram: —

- [x] **T-00-002** · Vault de Obsidian + plantillas (tarea, ADR, bitácora)
      - dod: `docs/vault/` navegable en Obsidian sin plugins
      - engram: —

- [x] **T-00-003** · `PROJECT_STATE.md` + contrato de continuidad + `/sdd-init`
      - dod: frontmatter parseable · rituales de inicio y cierre escritos
      - engram: obs-dae8c6b242d09f68

- [x] **T-00-004** · `.engram/RUBRIC.md` + `.engram/queue/`
      - dod: rúbrica 0–5 · taxonomía de `topic_key` · formato de candidato
      - engram: —

- [x] **T-00-017** · `.claude/settings.json` del repo: listas `deny` + `ask` · `Makefile` con `clean`
      - spec: [[analisis-y-plan#73-supervisión-del-modo-bypass]]
      - dod: `rm` denegado por completo · borrado legítimo solo vía `make clean`
      - engram: obs-86bec7dbc4378a13

- [x] **T-00-005** · Scaffold del API en Go
      - stack: `chi` v5, `log/slog` JSON, `/healthz`, `/readyz`, config por entorno
      - tests: 13 tests en 2 paquetes · rojo→verde documentado por archivo
      - dod: `go build` OK · `go vet` limpio · `gofmt` limpio · `go test` verde
      - nota: paquete renombrado a `httpapi` (evita sombrear `net/http`)
      - **`-race` NO corre en esta máquina**: exige cgo y no hay compilador C.
        Se mueve a CI en Linux (`make test-race`). Ver T-00-008
      - engram: —

- [x] **T-00-006** · Scaffold del frontend
      - stack: Vite 8 · React 19.2 · TS 6 strict · Tailwind **v4.3** · TanStack Router+Query
        · RHF + Zod 4 · Vitest 4 + Testing Library · oxlint
      - TDD: 6 tests en 2 archivos, rojo→verde documentado (`i18n`, `EmptyState`)
      - i18n desde el dia uno: `src/i18n/` con clave tipada. **Cero copy hardcodeado**
      - screaming architecture: `src/features/pets/`, alias `@/`
      - conectado a las puertas: job `web` en `ci.yml` · `lint-web`/`test-web` en el Makefile
      - dod: typecheck limpio · lint limpio · 6 tests verdes · build OK (CSS 8.37 kB = Tailwind activo)
      - nota: Tailwind v4 es **CSS-first** (`@tailwindcss/vite` + `@import`/`@theme`),
        sin `tailwind.config.js` ni PostCSS. Verificado en doc oficial via context7
      - nota: `baseUrl` esta deprecado en TS 6; se quito en vez de silenciarlo
      - **shadcn/ui NO instalado** — se difiere a la primera pantalla real que lo necesite
      - engram: —

- [x] **T-00-023** · Vaciar `.trash/` y la carpeta `mascotapp/`
      - los huerfanos de Vite (`App.tsx`, `App.css`, `assets/`) se movieron a
        `.trash/` (gitignored) porque el guardian de i18n los marcaba, y
        excluirlos del scan habria sido silenciar en vez de arreglar
      - tambien sobra la carpeta vacia `mascotapp/` (era una vault paralela)
      - destrabada el 2026-08-29 al aplicar el usuario
        `20-arquitectura/settings-propuesto.json`: el deny general `Bash(rm *)`
        se reemplazo por denies de fuga (raiz, `~`, `$HOME`, `../`, unidad `C:`),
        que implementa la politica declarada: borrar dentro del proyecto, nunca fuera
      - `.trash/` eliminado (5 archivos, 45K). `mascotapp/` ya no existia
      - **hallazgo**: los archivos de settings **recargan en caliente**. El `rm`
        paso sin reiniciar la sesion, cosa que con el deny anterior habria sido
        bloqueada. Un cambio de barreras no necesita reinicio para tener efecto
      - engram: —

- [x] **T-00-007** · Docker Compose local (Postgres + MinIO como sustituto de R2)
      - `postgres:17-alpine` (misma major que Neon) + `minio` (API S3, misma que R2)
      - healthchecks en ambos · volumenes nombrados · `.env.example` versionado
      - verificado de verdad: `make dev` -> ambos `healthy` · `select version()` responde
        PostgreSQL 17.10 · MinIO `/minio/health/live` HTTP 200
      - API contra el stack: `/healthz` 200 · `/readyz` 200 · ruta inexistente 404
        · log JSON estructurado con `request_id` correlacionado
      - dod: cumplida
      - engram: —

- [x] **T-00-008** · CI con puertas de calidad
      - instalado: `golangci-lint` v2.13.2 · `govulncheck` v1.7.0 · GNU Make 4.4.1 (winget)
      - `.golangci.yml` v2 · `.github/workflows/ci.yml` (jobs `api` + `secrets`, YAML validado)
      - **`go test -race` corre SOLO en CI** (ubuntu-latest tiene gcc; este host no)
      - `make lint` y `make test` en verde; targets divididos api/web para T-00-006
      - **3 hallazgos del linter, los 3 corregidos en código** (ver nota abajo)
      - dod ajustada: **no hay remote git**, así que no se puede probar el workflow en un PR.
        Verificado: YAML parsea · todos los comandos corren localmente en verde
      - engram: obs pendiente
      - ⚠️ `make` quedó instalado pero **no en PATH hasta reiniciar la shell**.
        Está en `AppData/Local/Microsoft/WinGet/Packages/ezwinports.make_*/bin`

- [x] **T-00-022** · Resolución de IP de cliente detrás de Cloud Run
      - motivo: se quitó `middleware.RealIP` por spoofeable (GHSA-3fxj-6jh8-hvhx).
        Sin reemplazo, `r.RemoteAddr` es el proxy, no el cliente
      - impacto: rate limiting por IP y `ip_hash` del audit log dependen de esto
      - `internal/httpapi/clientip.go`: `ClientIPResolver` con algoritmo
        **"primera entrada no confiable desde la derecha"**. Se apoya en que cada
        salto **agrega** a la derecha: lo que el cliente falsifica queda a la
        izquierda de lo que agregó nuestra infraestructura, y el escaneo se
        detiene antes de llegar ahí
      - si el *peer* TCP no es un proxy confiable, el header se **ignora entero**:
        es input del cliente sin validar
      - falla cerrada: header ausente, vacío, todo confiable o con una entrada
        malformada → devuelve el peer, que nunca es falsificable
      - `X-Real-IP` y `True-Client-IP` se ignoran a propósito: nada en el destino
        de despliegue los escribe, y aceptarlos solo ampliaría la superficie
      - `TRUSTED_PROXIES` (nuevo, en `config`) por defecto **vacío** = confiar en
        nadie = usar siempre el peer. Un despliegue sin configurar obtiene una
        dirección menos precisa, nunca una controlada por el atacante
      - dod: parseo de `X-Forwarded-For` contra un set de proxies confiables ✔ ·
        tests de spoofing ✔ (19 casos) · **4 mutaciones, 4 muertas** ✔
      - **hallazgo de la mutación**: la 4.ª (`continue` en vez de `return` ante una
        entrada malformada) **sobrevivió** a la primera batería. Los tests de
        entrada malformada tenían un solo elemento en la cadena, así que no
        distinguían "detenerse" de "saltar". Se agregó el caso
        `1.1.1.1, not-an-ip, 10.0.0.5` y la mutación murió
      - abre [[FASE-11]] → T-11-XXX: fijar los rangos reales del front-end en
        `TRUSTED_PROXIES` al desplegar, y **verificarlo contra tráfico real**
      - engram: candidato en cola (`mascotapp/security/client-ip-resolution`)

- [x] **T-00-021** · Timeout y concurrencia en `NewReadinessHandler`
      - firma nueva: `NewReadinessHandler(timeout, checks...)`
      - los checks corren **en paralelo** bajo un deadline compartido. En serie se
        pagaba la latencia de cada dependencia lenta una tras otra; sin deadline,
        un check colgado colgaba `/readyz` entero — la sonda **se volvia** la caida
        que deberia reportar
      - salida ordenada (`sort.Strings`) para que el body sea estable en logs y asserts
      - TDD: rojo por firma faltante -> verde. **Test de mutacion**: al quitar el
        timeout de la implementacion el test **colgo hasta matarlo a los 60s**.
        No pasa por casualidad
      - **bonus, causa raiz de un bug de entorno**: Windows Application Control
        bloqueaba `http.test.exe`. Go nombra el binario por el **directorio**, no por
        el paquete — por eso el rename previo de paquete no alcanzo. Se renombro
        `internal/http` -> `internal/httpapi`. Ademas ya no sombrea `net/http`
      - dod: 5 tests de readiness verdes · lint 0 issues · codegraph resincronizado
      - engram: —

- [x] **T-00-009** · Contrato OpenAPI 3.1 + generación de tipos
      - `api/openapi.yaml` documenta **solo lo que existe**: `/healthz` y `/readyz`
      - Go: `oapi-codegen` v2.8.0 -> `apps/api/internal/api/openapi.gen.go` (424 lineas)
      - TS: `openapi-typescript` 7.13.0 -> `apps/web/src/api/schema.gen.ts` (147 lineas)
      - `make generate` regenera ambos lados desde el único archivo
      - **CI falla si hay drift**: regenera y compara con `git diff --exit-code`
      - verificado: regenerar sin cambios da hash idéntico (determinismo);
        agregar un campo al schema aparece en **ambos** lados; revertir vuelve al hash original
      - nota: `embedded-spec` desactivado a propósito (arrastra `kin-openapi`;
        binario más chico = arranque en frío más rápido, ADR-0003)
      - nota: `openapi-typescript` **fuera de devDependencies**: pide `typescript@^5.x`
        y el proyecto usa TS 6. Es herramienta de build, su salida se commitea.
        Se invoca con versión fija por `npx` — sin `--legacy-peer-deps`
      - engram: —

- [x] **T-00-010** · ADR-0001 stack · ADR-0002 multi-tenancy · ADR-0003 costo cero
      - dod: tres ADRs con alternativas descartadas y motivo
      - engram: obs-4ae5918c16167e36

- [x] **T-00-011** · ADR-0004 TLS sin mTLS + condiciones de reapertura
      - spec: [[analisis-y-plan#58-mtls-vs-tls--análisis-y-veredicto]]
      - dod: las cuatro condiciones de reapertura escritas explícitamente
      - engram: obs-b24f892dc7c32085

- [x] **T-00-012** · VERIFICAR límites vigentes de free tier
      - alcance: Cloud Run · Neon · R2 · Cloudflare Pages · Resend · Sentry
      - dod: tabla con límites observados y fecha, en `20-arquitectura/free-tier-limits.md`
      - nota: **verificar, no asumir.** Los límites del plan son de agosto 2026.
      - engram: candidato probable (`mascotapp/ops/free-tier`)

- [x] **T-00-013** · Andamiaje de i18n (`es-MX`) y estructura de claves
      - `src/i18n/es-MX.ts` con `TranslationKey` derivado de las claves reales:
        una clave inexistente es **error de tipo**, no fallo en runtime
      - `t(key, params)` deja el placeholder visible si falta el parametro,
        en vez de imprimir `undefined` — el bug se ve, no se disfraza
      - **guardian automatizado**: `noHardcodedCopy.test.ts` escanea `src/**/*.tsx`
        y falla si hay texto JSX o props (title/placeholder/aria-label/alt)
        fuera de `t()`. **Sin exclusiones.** La DoD dejo de ser aspiracional
      - probado que el guardian detecta de verdad: atrapo los huerfanos de Vite
      - agregar un locale = archivo tipado `Record<TranslationKey, string>`;
        el compilador exige paridad de claves
      - dod: 10 tests verdes · lint limpio · typecheck limpio · build OK
      - engram: —

- [x] **T-00-014** · `gentle-ai codegraph init` + verificar `.codegraph/`
      - indice creado: **27 archivos · 225 nodos · 367 aristas** · 0.63 MB
      - backend `node:sqlite` con WAL · `.codegraph/` ya estaba en `.gitignore`
        (cada checkout necesita su propio indice; nunca se copia ni comparte)
      - verificado que **consulta**, no solo que existe: `codegraph query "readiness handler"`
        ubica `NewReadinessHandler` en `health.go:26`, y `codegraph callers` devuelve
        su unico llamador `NewRouter` en `router.go:28`
      - ⚠️ **el servidor MCP de codegraph se desconecto en esta sesion.** El CLI funciona
        igual; el MCP es comodidad (un round-trip en vez de varios), no requisito.
        Se reconecta reiniciando la sesion
      - nota: `--cwd .` falla con "unsafe CodeGraph root". Requiere ruta absoluta
      - nota: hay CodeGraph v1.6.0 disponible (corriendo 1.5.0). `codegraph upgrade`
        lo corre el usuario, no el agente
      - engram: —

- [x] **T-00-015** · `skill-registry` (.atl/skill-registry.md, 41 skills) + ciclo SDD
      - dod: ciclo SDD escrito en `20-arquitectura/sdd-workflow.md`
      - engram: obs-e2ec087777f054b1

- [x] **T-00-016** · `opencode.json` con doble proveedor + permisos por ruta · ADR-0005
      - OpenCode 1.18.23 instalado · JSON validado
      - **28 reglas `bash`** y **23 reglas `edit`**: la lista blanca/negra de §7.2
        dejo de ser prosa y paso a ser permisos que el runtime aplica
      - denegado a OpenCode: dominio, migraciones, auth, middleware, `api/openapi.yaml`,
        todo lo generado (`*.gen.*`), y **todo el estado del proyecto**
        (`PROJECT_STATE.md`, `.engram/`, `openspec/`, tableros, ADRs)
      - permitido: i18n, stories, tests de funciones ya especificadas, seeds, bitacora
      - [[ADR-0005-opencode-piloto]] con criterios de continuacion **y de muerte**
        escritos **antes** de empezar
      - claves solo por entorno; `.env.example` documenta cuales
      - nota: **el piloto NO se ejecuta hasta Fase 06.** Esto solo deja la config
      - engram: —

- [!] **T-00-018** · Probar empíricamente si el hook `PreToolUse` dispara en bypass
      - **BLOQUEADA — esperando al usuario.** El script ya está escrito y probado:
        `scripts/hook-canary.ps1` (parseo OK, exit 0 en comando benigno, exit 2 en la
        sonda). Lo único que falta es **instalarlo**, y eso lo tiene que hacer el
        usuario: `.claude/settings.json` está en `deny` a propósito, para que un
        agente no pueda reescribir sus propias barandas
      - instalar: el bloque `hooks` está en la cabecera de `scripts/hook-canary.ps1`
      - probar: `echo GENTLE_CANARY_PROBE`
        - bloqueado → el hook dispara **y** `exit 2` deniega en este modo
        - se ejecuta pero hay línea `FIRED` en el log → observa pero no puede denegar
        - se ejecuta y no hay línea → la capa no existe en este modo: se tacha
      - dod: resultado observado registrado en `20-arquitectura/supervision-checks.md`
      - nota: si no dispara, se descarta la capa y **no se cuenta como protección**
      - engram: candidato probable (`mascotapp/ops/agent-supervision`)

- [ ] **T-00-020** · Cuenta de facturación GCP + alerta de presupuesto en USD 0
      - motivo: Cloud Run **exige tarjeta** para verificar identidad (hallazgo T-00-012)
      - dod: billing account creada · presupuesto con alerta al 100% de USD 0 · captura en 20-arquitectura/
      - nota: el uso dentro del free tier no se cobra, pero sin la alerta nadie se entera si se sale
      - engram: —

- [x] **T-00-019** · Devcontainer aislado + ADR-0006 modo de permisos
      - `.devcontainer/` con compose propio: workspace + postgres + minio en red interna
      - **aislamiento probado, no declarado**: se corrio un contenedor con el mismo
        bind mount y se confirmo que `/workspace` tiene el repo y que
        `C:\Users\Jose` **no existe** desde adentro
      - `postCreate.sh` instala golangci-lint, govulncheck, oapi-codegen y goose
      - el contenedor **si** tiene compilador C, asi que `make test-race` corre ahi
        (en el host Windows no)
      - [[ADR-0006-modo-permisos]] con la tabla de lo verificado y el limite honesto:
        las reglas comparan **cadenas**, no efectos -> son barandas, no sandbox
      - dod: JSON valido · compose valido · aislamiento verificado
      - engram: —

---

## Salida de fase

La Fase 00 se cierra cuando:

1. Todas las tareas están en `[x]`.
2. La batería de verificación de supervisión corre y queda registrada.
3. `make dev`, `make test` y `make lint` funcionan de punta a punta.
4. La cola de Engram está vacía o aprobada.

Siguiente: [[FASE-01]] — Dominio y datos.
