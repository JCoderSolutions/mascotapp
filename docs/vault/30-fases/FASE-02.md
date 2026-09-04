# FASE 02 — Auth y multi-tenancy

**Objetivo:** Identidad, sesión y resolución de tenant, con el `shelter_id` viniendo siempre del token y nunca de la URL.

**Entregable verificable:** Registro/login, Argon2id, TOTP, enlace mágico, rotación de refresh con detección de reutilización, middleware de tenant, matriz RBAC testeada.

**RDD:** ✅ recomendado · Judgment Day antes del merge de la rotación de tokens
**Depende de:** 01

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Change SDD activo

`openspec/changes/phase-02-auth-and-multitenancy/`

| Artefacto | Estado | Engram |
|---|---|---|
| [exploration.md](../../../openspec/changes/phase-02-auth-and-multitenancy/exploration.md) | cerrado | `#151` |
| [proposal.md](../../../openspec/changes/phase-02-auth-and-multitenancy/proposal.md) | cerrado, amendado | `#152` |
| [design.md](../../../openspec/changes/phase-02-auth-and-multitenancy/design.md) | cerrado, `P2-D1`..`P2-D11` | `#155` |
| [specs/](../../../openspec/changes/phase-02-auth-and-multitenancy/specs/) | 6 capabilities | `#154` |
| [tasks.md](../../../openspec/changes/phase-02-auth-and-multitenancy/tasks.md) | cerrado, 24 PRs encadenados | `#158` |

> **Este tablero indexa; no duplica.** El detalle de cada tarea — `spec:`, `build:`,
> `tests:`, `dod:`, `est:` — vive en `tasks.md` y tiene un solo dueño. Acá viven el
> estado y la traza.

---

## Decisiones del usuario — 2026-09-01, antes de la propuesta

| Decisión | Elegido | Por qué |
|---|---|---|
| **B1 — escalada dentro del tenant** | **Lo cierra Fase 02** | Es la fase que construye RBAC. Sin grants por columna, un chequeo de dominio sería **lo único** que impide que un refugio se auto-verifique (`shelters.status = 'verified'`, salteando LT-2) — sin red abajo, que es justo lo que ADR-0002 existe para evitar. **Resuelve la contradicción** entre ADR-0009 (lo difería a 02/03) y este tablero: **la dueña es la 02**. |
| **Envío de mail** | **Interfaz + stub** | El flujo de auth queda completo y testeable de punta a punta sin cuenta externa, sin clave y sin red — y todavía no hay dominio registrado. Cablear Resend de verdad es **no-goal** de esta fase. |
| **Rate limiting** | **En memoria, por instancia, y dicho** | La tabla de free tier no nombra ningún backend, y un contador en Postgres cuesta una escritura por intento de login contra una base que factura por CU-hora. **La limitación va escrita, no supuesta:** se reinicia en cada arranque en frío y no coordina entre instancias, así que frena fuerza bruta torpe y no a un atacante paciente. |

> **Consecuencia que NO es una regresión.** Angostar la asignación **rompe a propósito** un test
> puesto justamente para ponerse rojo ese día:
> `TestAssignment_DoesNotYetRequireAnActiveMembership`. **Actualizarlo es parte del mismo cambio.**
>
> **Corregido el 2026-09-02, contra la fuente.** Este párrafo decía **dos** tests, y sumaba
> `TestApplicantPolicy_IsAsWideAsWritingAnApplication`. **Ese no se mueve.** Su mecanismo es un
> `INSERT` de `app_tenant` en `adoption_applications` nombrando un `applicant_user_id` arbitrario, y
> los grants por columna de B1 van sobre `shelters` y `memberships` — **no tocan esa tabla**.
> Cerrarlo pediría revocar `INSERT (applicant_user_id)` en `adoption_applications`, que es
> justamente el camino por el que un adoptante postula: flujo de Fase 05, que todavía no existe.
> **El error estaba en tres lugares a la vez** —la exploración, la propuesta y el comentario del
> propio test— y lo encontró `sdd-design` (P2-D6). En esta fase se mueve **un** test fijado, no dos.

### Cuatro más, en la ronda de preguntas previa a la propuesta

| Decisión | Elegido | Por qué |
|---|---|---|
| **Topología de despliegue** | **Orígenes separados** | El dominio llega **después** de la primera prueba del MVP, así que el primer despliegue cae en `*.pages.dev` y `*.run.app` — **sites distintos**. |
| **Códigos de recuperación de TOTP** | **Sí, en esta fase** | TOTP es obligatorio para `owner`/`admin`: sin códigos, un teléfono perdido deja al dueño del refugio afuera **sin camino de vuelta**. Suma una tabla, con su grant, su política y su clasificación en el catálogo. |
| **Registro de refugios** | **Abierto, nace `pending_verification`** | Es la mitigación de LT-2 funcionando, no un hueco: nadie publica hasta que un humano lo habilita. Hasta Fase 06/10, verificar es un `UPDATE` manual. |
| **Magic link** | **Indistinguible** | Misma respuesta y mismo tiempo exista o no la cuenta. Si no, el endpoint es un **oráculo de existencia sobre direcciones de mail** — la misma clase de fuga que la Fase 01 cerró tres veces. |

> **Lo que cuesta la topología, dicho antes de construirlo.** `SameSite=Strict` como lo escribe §5.2
> **no se sostiene** entre sites distintos: el navegador no manda la cookie, el refresh falla, y no
> falla con un error claro sino como **un logout cada 15 minutos que parece un bug de sesión**.
>
> Así que la Fase 02 construye `SameSite=None; Secure`, la allowlist de CORS **con credenciales**
> (o sea nunca `*`), y **protección CSRF explícita** — la defensa que `Strict` regalaba gratis.
> Esas tres piezas **no estaban agendadas para esta fase**; entran por esta decisión.
>
> **Condición de reapertura, escrita para que no se pudra:** cuando la web y la API queden detrás de
> un solo hostname, la cookie puede volver a `Strict` y el token CSRF se puede retirar. Es un cambio
> deliberado y posterior, **y el riesgo de que nunca ocurra está aceptado a sabiendas**.

> **Dato verificado, no supuesto:** la API en Go **no corre gratis en Cloudflare** — Workers ejecuta
> JS y WASM, no un binario Go arbitrario, y por eso el plan la puso en Cloud Run desde el principio.
> Lo que sí entra en el free tier es el **proxy**: 100.000 requests/día compartidos con Workers,
> contra los ~6.700/día que estima el propio documento de límites. **Usaría el 7%.**

> **Dato desconocido, no ausente.** `.env.example` está denegado a los agentes por una regla global,
> así que **no está verificado** si `RESEND_API_KEY` y `JWT_SECRET` ya existen. No se supone en
> ninguna dirección: hay que confirmarlo con el usuario.

## Alcance heredado de Fase 01 — NO se expande, ya está decidido

Arrastrado el **2026-09-01** al cerrar `T-01-035`. Fase 01 dejó estos tres puntos **abiertos a
propósito**: los tres necesitan la capa de dominio o el RBAC que esta fase construye, y ninguno se
podía cerrar en la base sin inventar una regla que ningún endpoint respalda todavía.

### 1. La ruta de acceso de `refresh_tokens`

Hoy es **default-deny**: RLS encendida, **sin política y sin grant**, así que `app_tenant` no la
alcanza en absoluto — y eso es **más estricto** que cualquier política que se pudiera haber escrito
antes de que esta fase eligiera su camino. `TestRefreshTokens_RefuseAppTenantEntirely` lo fija.

**La decisión de esta fase:** un rol `app_auth` dedicado, o una GUC `app.user_id` junto a la del
tenant. Lo que **no** puede pasar es que la tabla gane un grant sin ganar su política — un grant
sin política es el estado más ancho posible, no el más seguro.

### 2. Escalada de privilegios DENTRO del tenant (hallazgo B1 de Judgment Day)

Confirmado en vivo: con grants a nivel **tabla**, `app_tenant` puede, sobre su propia fila,
ponerse `status = 'verified'` (**bypass de LT-2 — el refugio se auto-verifica**), subirse
`storage_quota_bytes`, y ponerse `role = 'owner'` en `memberships`.

El arreglo son **grants a nivel columna**, y qué columnas puede escribir un tenant depende de
endpoints que Fase 03 todavía no escribió — por eso se difirió. Ver `[[ADR-0009]]`, donde está
escrito como la reapertura más probable de esa decisión.

**El marcador se queda donde está.** `TestApplicantPolicy_IsAsWideAsWritingAnApplication` **no** se
pone rojo con estos grants —ver la corrección de arriba— y su comentario, que dice lo contrario,
también estaba equivocado. Sigue marcando una deuda real, pero de Fase 05.

**Lo que sí queda pinchado en esta fase** es el residuo que los grants por columna **no** cierran, y
va con test propio (`TestMembershipInsert_CanStillMintAnOwner`): revocar `UPDATE (role)` mata la
auto-promoción, pero `INSERT` **sigue llevando `role`**, así que un miembro del refugio A puede
insertar una membresía `owner` para un cómplice **dentro de A**. No se cierra con un grant por
columna, porque cerrarlo exige que la base sepa **quién actúa**, y bajo `app_tenant` no hay GUC de
usuario a propósito. Lo acotan tres cosas: el `UNIQUE (user_id, shelter_id)`, la política de tenant,
y el RBAC de dominio. El endpoint de invitaciones de Fase 03 es donde la regla se vuelve expresable.

### 3. Asignación a un miembro que el refugio no puede leer

`member_visible_users` filtra por `status = 'active'`; la clave compuesta de
`assigned_to_user_id` solo puede chequear que el par de `memberships` **exista**. Y eso es
PostgreSQL, verificado en 17: **una foreign key no puede referenciar un índice único parcial**, así
que `UNIQUE (user_id, shelter_id) WHERE status = 'active'` no está disponible como clave
referenciada.

Queda `asignable = la fila existe` contra `legible = la fila existe Y está activa`. **No es una
fuga entre tenants** —todos pertenecen al mismo refugio— así que la respuesta de la base es
**incompleta, no equivocada**. Cerrarlo necesita un trigger o la capa de dominio, y las reglas de
asignación viven con el RBAC de esta fase.

`TestAssignment_DoesNotYetRequireAnActiveMembership` deja la conducta actual por escrito y se pone
rojo el día que se angoste.

## Entrega — 24 PRs encadenados

**Decisión del usuario, 2026-09-02: PRs encadenados, no `size:exception`.** El presupuesto de
revisión son **400 líneas por unidad** y tres de las cinco rebanadas lo reventaban — la (c), la
superficie HTTP de auth, por **7,85×**. Nadie revisa 3.140 líneas de código de autenticación con
atención real, y esta es la fase donde un error no se recupera.

Estrategia **feature-branch-chain**, igual que la Fase 01: el `PR-02-n` se basa en el `PR-02-(n-1)`
y la cadena entera mergea a `main` junta. **4.890 líneas en 24 PRs, ninguno sobre 400** — la suma
por PR se verificó contra los `est:` de cada tarea, no contra la tabla.

**Judgment Day va antes del merge del `PR-02-11`** (`session.go` — rotación y detección de
reutilización), revisado junto con la ruta de acceso a `refresh_tokens` que ya mergeó en el
`PR-02-02`. Es el cuello de botella real de la fase: por la cadena, **los 13 PRs siguientes están
detrás de él** en el orden de merge, dependan o no de su código.

**Una desviación de la numeración del diseño, forzada por un test.** El diseño numera
`00013`(a) → `00014`(c) → `00015`/`00016`(b), pero entrega en orden (a) → (b) → (c). El invariante
de `T-01-005` —*versiones estrictamente ascendentes, sin huecos*, en `migrate_test.go:91`— corre en
**la rama de cada PR**, no solo en el árbol final. Meter `00015`/`00016` antes de que exista
`00014` dejaría esa rama con `{00013, 00015, 00016}` y el CI del propio PR en rojo. Por eso la
`00014` se adelanta al `PR-02-04`. Misma clase de movida que el `T-01-007` de la Fase 01.

## Tareas

Columna **PR**: en qué PR de la cadena entra. Columna **est**: líneas autoradas estimadas.

| Estado / ID | Tarea | PR | est |
|---|---|---|---|
| `[x]` `T-02-001` | RED — `WithAuthUser` / `WithAuthLookup` unit tests against a recording `pgx.Tx` stub | PR 1 | 130 |
| `[x]` `T-02-002` | GREEN — `internal/db/auth.go`: `WithAuthUser`, `WithAuthLookup`, `ErrNoAuthUser`, `NewAuthPool` | PR 1 | 120 |
| `[x]` `T-02-003` | Migration `00013_auth_role.sql` + `dbtest` third-pool wiring + reachability/isolation proof | PR 2 | ~~260~~ **579** ⚠ |
| `[x]` `T-02-004` | Catalog classification + `query/auth.sql` + `TestPolicies_DoNotCrossGUCs` | PR 3 | 150 |
| `[x]` `T-02-005` | Pin the PostgreSQL column-privilege semantics slice (b) depends on | PR 5 | 55 → 221 |
| `[x]` `T-02-006` | Migration `00015_column_grants.sql` — B1 on `shelters` and `memberships` | PR 5 | 215 → 615 `size:exception` |
| `[x]` `T-02-007` | Migration `00016_assignee_active_membership.sql` — the one pinned test move, same commit | PR 6 | 140 → 291 |
| `[x]` `T-02-008` | RED — `password.go` (Argon2id) unit tests | PR 7 | 90 → 317 |
| `[x]` `T-02-009` | GREEN — `password.go` | PR 7 | 70 → 221 |
| `[x]` `T-02-010` | RED — `envelope.go` (AES-256-GCM) unit tests | PR 8a | 110 → 382 |
| `[x]` `T-02-011` | GREEN — `envelope.go` | PR 8a | 90 → 183 |
| `[x]` `T-02-012` | RED — `totp.go` unit tests | PR 8b | 100 |
| `[x]` `T-02-013` | GREEN — `totp.go` | PR 8b | 90 |
| `[x]` `T-02-014` | RED — `token.go` (JWT) unit tests | PR 9 | ~~120~~ **643** |
| `[x]` `T-02-015` | GREEN — `token.go` | PR 9 | ~~100~~ **289** `size:exception` (impl 289/250 · total 935/800, aceptada 2026-09-04) |
| `[x]` `T-02-016` | Migration `00014_totp_recovery_codes.sql` + catalog/query + recovery-code scenarios | PR 4 | ~~160~~ **280** |
| `[~]` `T-02-017` | RED — `recovery.go` tests | PR 10 | 100 |
| `[ ]` `T-02-018` | GREEN — `recovery.go` | PR 10 | 90 |
| `[ ]` `T-02-019` | RED — `session.go` (refresh rotation + reuse detection) tests | PR 11 | 170 |
| `[ ]` `T-02-020` | GREEN — `session.go` | PR 11 | 150 |
| `[ ]` `T-02-021` | 🔴 **Judgment Day** — adversarial review before the refresh-token rotation logic merges | gate PR 11 | 0 |
| `[ ]` `T-02-022` | Email port + `LogSender` stub — sequenced here, see the Ordering note above | PR 16 | 70 |
| `[ ]` `T-02-023` | RED — `middleware_auth.go` tests (the F02 success criterion) | PR 12 | 150 |
| `[ ]` `T-02-024` | GREEN — `middleware_auth.go` | PR 12 | 140 |
| `[ ]` `T-02-025` | `cors.go` + `csrf.go` — allowlist with credentials, `Origin`/`Sec-Fetch-Site` verification | PR 13 | 160 |
| `[ ]` `T-02-026` | `config.go` additions — auth DSN, `JWT_SECRET`, `AUTH_KEK`, `WEB_ORIGINS`, `API_PUBLIC_ORIGIN`, same-site boot refusal | PR 13 | 130 |
| `[ ]` `T-02-027` | Registration handler | PR 14 | 140 |
| `[ ]` `T-02-028` | Login handler — password path, token + cookie issuance | PR 15 | 150 |
| `[ ]` `T-02-029` | Login handler — TOTP-mandatory and recovery-code fallback | PR 15 | 140 |
| `[ ]` `T-02-030` | Magic-link handler | PR 16 | 120 |
| `[ ]` `T-02-031` | Refresh + logout handler | PR 17 | 160 |
| `[ ]` `T-02-032` | TOTP enrol/verify handler | PR 18 | 140 |
| `[ ]` `T-02-033` | Session/shelter-exchange handler — `POST /auth/session/shelter` | PR 19 | 100 |
| `[ ]` `T-02-034` | `api/openapi.yaml` — auth surface + regenerate `openapi.gen.go` | PR 20 | 90 |
| `[!]` `T-02-035` | `.env.example` — add `DATABASE_URL_AUTH`, `AUTH_KEK`, `JWT_SECRET`, `WEB_ORIGINS`, `API_PUBLIC_ORIGIN`, the `app_auth` bootstrap password variable | PR 24 | 10 |
| `[ ]` `T-02-036` | RED — `authz/matrix.go` table-driven exhaustive tests | PR 21 | 130 |
| `[ ]` `T-02-037` | GREEN — `authz/matrix.go` | PR 21 | 100 |
| `[ ]` `T-02-038` | RED — `ratelimit.go` tests | PR 22 | 160 |
| `[ ]` `T-02-039` | GREEN — `ratelimit.go` | PR 22 | 140 |
| `[ ]` `T-02-040` | Final wiring — `router.go` | PR 23 | 150 |

---

## Salida de fase

1. Todas las tareas en `[x]`.
2. Verificación de fase del plan maestro ejecutada y registrada.
3. Cola de Engram vacía o aprobada.
4. `/sdd-verify` en verde → `/sdd-archive`.

Siguiente: [[FASE-03]]
