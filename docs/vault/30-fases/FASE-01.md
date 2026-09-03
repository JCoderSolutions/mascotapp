# FASE 01 — Dominio y datos

**Objetivo:** Modelar el dominio en la base y hacer el aislamiento entre refugios verificable, no confiable.

**Entregable verificable:** Migraciones `goose`, políticas RLS, `sqlc` generado, seeds, y **un test A/B de aislamiento por cada tabla con `shelter_id`**.

**RDD:** ✅ recomendado · Judgment Day antes del merge de las políticas RLS
**Depende de:** 00

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Change SDD activo

`openspec/changes/phase-01-domain-and-data/`

| Artefacto | Estado | Engram |
|---|---|---|
| [exploration.md](../../../openspec/changes/phase-01-domain-and-data/exploration.md) | cerrado | `#35` |
| [proposal.md](../../../openspec/changes/phase-01-domain-and-data/proposal.md) | cerrado, amendado | `#37` |
| [design.md](../../../openspec/changes/phase-01-domain-and-data/design.md) | cerrado, D7 revertido | `#39` |
| [specs/](../../../openspec/changes/phase-01-domain-and-data/specs/) | 7 capabilities | `#40` |
| [tasks.md](../../../openspec/changes/phase-01-domain-and-data/tasks.md) | cerrado | — |

> **Este tablero indexa; no duplica.** El detalle de cada tarea — `spec:`, `build:`,
> `tests:`, `dod:`, `est:`, `open-q:` — vive en `tasks.md` y tiene un solo dueño.
> Acá viven el estado y la traza. (Plan §7.1: *"FASE-NN.md enlaza al change SDD activo;
> no duplica su contenido."*)

## Decisiones del usuario

| Fecha | Decisión |
|---|---|
| 2026-08-29 | **Troceo: 15 PRs encadenados** sobre `feature/fase-01`; PR *n* parte de PR *n-1*. Mediana ~250 líneas autoradas; PR 4 (517) y PR 5 (424) quedan sobre presupuesto a sabiendas — y son justo los que llevan roles, grants, el guard de `pg_roles` y las primeras políticas RLS. Máxima atención ahí. |
| 2026-08-29 | **`T-01-007` movido de PR 4 a PR 3** (decidido por restricción técnica, no por preferencia): `//go:embed migrations/*.sql` no compila sin ningún `.sql`, así que `migrate.go` y la primera migración no pueden vivir en PRs separados. Efecto lateral bueno: PR 3 pasa a 380 líneas (bajo presupuesto) y PR 4 baja de 517 a 455. |
| 2026-08-29 | **`users.email` es `citext`**, como decía §4. Revierte D7, cuya premisa ("no se puede confirmar `citext` en Neon") se verificó **falsa**: Neon lo documenta y `postgres:17-alpine` trae `citext 1.6` de fábrica. |

## Preguntas abiertas — NO responder en silencio

- [x] **RESUELTO POR EL USUARIO 2026-09-01 — LT-2 va a FASE 06.** **LT-2 no lo hace cumplir la base, y la suite venia fijando lo contrario (hallazgo de T-01-022).** §1.1 pone `pending_verification` como requisito duro de MVP: *"un refugio no puede publicar hasta ser verificado manualmente"*. La columna existe y su default es `pending_verification`; **nadie la lee**. `public_catalog` sobre `pets` filtra por `status`, `published_at` y `deleted_at` y por nada mas, asi que hoy **un refugio sin verificar publica y su animal aparece en el catalogo publico** — que es exactamente el vector de estafa que LT-2 nombra. Peor: cada test de catalogo de este paquete publica desde un refugio sin verificar y afirma que el pet SI se ve, o sea que el verde se leia como "la verificacion anda".
  **Por que no se arreglo aca:** es una migracion, no un test. La condicion necesita `EXISTS (SELECT 1 FROM shelters ...)`, y el subquery de una politica exige que el rol tenga `SELECT` sobre la tabla referenciada (T-01-020, mutante M13). `app_public` no tiene ni grant ni politica sobre `shelters`, asi que agregar la condicion sola **no angosta el catalogo: lo rompe** con un error de permisos en toda lectura publica (confirmado por el mutante M6, que se lleva puestos diez casos). Se ata con la pregunta abierta de `app_public` sobre `shelters`, que sigue abajo.
  **Mientras tanto:** `TestPublicCatalog_DoesNotYetEnforceShelterVerification` es un test de caracterizacion — deja el estado actual por escrito, falla el dia que la condicion aterrice con instrucciones para invertirlo, y afirma que las dos mitades solo pueden aterrizar JUNTAS. **RESUELTO: entra en FASE 06**, junto con la política de `app_public` sobre `shelters`, que la condición NECESITA — las dos mitades tienen que aterrizar juntas o el catálogo público se rompe con un error de permisos.
  **Y el motivo por el que NO va a Fase 10, que el tablero no decía:** Fase 10 es **post-MVP** (el corte está al final de la Fase 08). Diferirla ahí significaba **lanzar el MVP con el vector de estafa abierto**, que es exactamente lo que LT-2 existe para evitar. Fase 06 es antes del corte.
  **Mientras tanto**, la verificación de un refugio es un `UPDATE` manual del usuario; el flujo con interfaz sigue siendo Fase 10. Anotado en `[[FASE-06]]`.

- [x] **NO BLOQUEA LA FASE (T-01-035) — es una decisión de entorno, tuya y permanente.** **Smart App Control bloquea TODOS los binarios de test en este host (identificado 2026-08-31 en T-01-020).** No es Defender: `Get-MpComputerStatus` reporta `AMRunningMode: Passive Mode` con protección en tiempo real **apagada**, mientras que `HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy\VerifiedAndReputablePolicyState` vale `1` (enforcement). **La exclusión de Defender que este proyecto venía recomendando no habría hecho nada.** Smart App Control bloquea ejecutables sin firma ni reputación y, a diferencia de Defender, **no tiene lista de exclusiones**: está prendido o apagado, y apagarlo **no se puede deshacer sin reinstalar Windows**. Cada `go test` linkea un binario nuevo sin firma, así que no hay arreglo posible dentro del repo.
  **Mitigado, no bloqueante:** `make test-api-container` corre la misma suite en Linux y pasa entera. La decisión de apagar Smart App Control es tuya y es irreversible; **el proyecto no la necesita** — toda la Fase 01 se verificó en contenedor. Se saca de las preguntas de la fase porque no lo es: no hay nada que Fase 01 pueda decidir al respecto.

- [x] **RESUELTO POR EL USUARIO 2026-09-01 — movido a Fase 07. Tres escenarios del delta de `dynamic-forms-data` no son esquema, son VALIDACIÓN, y nada en Fase 01 los entrega (hallazgo del pase de verificación de T-01-026).** La capability declara *"Duplicate field identifiers are rejected"*, *"An unknown field type is rejected"* y *"An out-of-range span is rejected"*, los tres redactados como **`WHEN it is validated`** — no como una restricción de base. Y hoy no existe nada que valide: no hay `packages/form-schema/`, no hay validador en Go, no hay `CHECK` sobre la forma de `definition` (la migración solo restringe `key`, `purpose` y `version >= 1`), y **ninguna tarea del tablero los nombra**. O sea que `/sdd-verify` sobre este change va a fallar en tres escenarios que Fase 01 nunca tuvo cómo cumplir.
  **Por qué no se resolvió acá:** el plan pone la unión cerrada compartida Go↔TS (`packages/form-schema/`, §9) y el motor de formularios en **Fase 07**, y esto es código de dominio, no una migración. Meterlo en Fase 01 no es una línea: es un paquete nuevo, su suite, y el contrato compartido con TS.
  **Nota sobre por qué no alcanza un `CHECK`:** se podría poner un `CHECK` de jsonb sobre `definition` y taparía los tres escenarios en la base. Sería la capa equivocada como ÚNICA capa — el mensaje de error de un `CHECK` no puede *"nombrar el identificador duplicado"* ni *"nombrar el tipo no soportado"*, que es literalmente lo que los escenarios piden. La base puede ser la última línea de defensa acá, no la primera.
  **Resuelto: se mueven a Fase 07**, que es donde el plan pone el motor de formularios y la unión cerrada compartida Go↔TS. El movimiento fue **quirúrgico, no en bloque**: del requisito *Field identifiers are stable* se movió solo la mitad de validación —la cláusula `SHALL be unique within its template` y su escenario— y **se quedó** el escenario *Answers survive a field removal*, que es el que la base sí hace cumplir y que T-01-024 afirma de punta a punta. El requisito *Field types are a closed union* se movió entero: su texto es validación de arriba a abajo. Los requisitos quedaron **textuales** en `[[FASE-07]]` bajo "Alcance heredado de Fase 01", para que `/sdd-new fase-07` los levante y no dependan de que alguien se acuerde.
  **La restricción que viaja con ellos:** la validación tiene que aterrizar **antes o junto** con el primer camino de escritura de plantillas. `form_template_versions` congela las filas publicadas con un trigger, así que una definición inválida publicada **no se puede arreglar** — solo se puede publicar otra al lado. Hoy la ventana está cerrada por orden de fases (ninguna fase entre esta y la 07 escribe plantillas) y es la 07 la que la abre: el constructor y el validador son la misma tarea, no dos.

- [x] **CERRADA EN FASE 02 EL 2026-09-03 (T-02-007).** La cierra la migración `00016_assignee_active_membership` (P2-D7): un trigger `BEFORE INSERT OR UPDATE OF assigned_to_user_id`, guardado por `WHEN (NEW.assigned_to_user_id IS NOT NULL)`, que levanta `23514` si el asignado no tiene una membresía `active` en ese refugio. **Un trigger y no una policy** por el argumento de ADR-0010: un trigger **no** lo saltea un rol `BYPASSRLS`. `TestAssignment_DoesNotYetRequireAnActiveMembership` fue **borrado** y reemplazado por su inverso `TestAssignment_RequiresAnActiveMembership`, en el mismo commit que el trigger. El texto de abajo queda como el registro de lo que se encontró. ↓
  **ARRASTRADA A FASE 02 (T-01-035).** **Un refugio puede asignar un caso a un miembro que NO PUEDE LEER (hallazgo de T-01-028).** `member_visible_users` filtra por `status = 'active'` (arreglo de T-01-016 tras Judgment Day), pero la clave compuesta de `assigned_to_user_id` solo puede chequear que el par de `memberships` **exista**. Y eso es PostgreSQL, verificado en 17 y no supuesto: **una foreign key no puede referenciar un índice único parcial** (`there is no unique constraint matching given keys`), así que `UNIQUE (user_id, shelter_id) WHERE status = 'active'` no está disponible como clave referenciada. Queda `asignable = la fila existe` contra `legible = la fila existe Y está activa`, así que un caso puede terminar en la cola de alguien cuyo nombre ese refugio ya no puede renderizar.
  **No es una fuga entre tenants** — todos los involucrados pertenecen a este refugio —, así que la respuesta de la base es **incompleta**, no equivocada. Cerrarlo necesita un trigger o la capa de dominio, y las reglas de asignación viven con RBAC en **Fase 02**. `TestAssignment_DoesNotYetRequireAnActiveMembership` lo deja por escrito y se pone rojo el día que se angoste.

- [x] **RESUELTO POR EL USUARIO 2026-09-01 — la purga borra las respuestas y conserva el caso.** **Qué sobrevive a una purga de retención (hallazgo de T-01-031).** Con `application_events`, `application_notes` y `documents` los tres con `ON DELETE RESTRICT`, **una solicitud que tenga cualquier historia no se puede borrar en duro.** Y eso está bien: el rastro es lo que hace que una auditoría signifique algo. Pero implica que la purga de §5.4 **no es un `DELETE` de la solicitud**, sino una **minimización de datos** sobre una fila que se queda.
  **Lo que queda sin resolver:** el comentario de T-01-027 que justifica la `CASCADE` de `form_submissions.application_id` con esa purga está **incompleto** — con historia presente, esa cascada no se dispara nunca. Y `adoption_applications.applicant_user_id` es `NOT NULL`, así que la fila tampoco se puede anonimizar en el lugar; queda apuntando a la persona.
  **RESUELTO: la purga borra `form_submissions` y la solicitud queda como registro.** La `CASCADE` de `00009` ya lo hace, así que no cuesta ninguna migración. **El costo, dicho y aceptado:** `applicant_user_id` es `NOT NULL`, así que el **vínculo al titular sobrevive** — la solicitud sigue apuntando a la persona. **No es un borrado completo del sujeto**, y si alguna vez hace falta que lo sea, las dos salidas quedan escritas acá: volver `applicant_user_id` nullable para poder cortarlo, o un estado `purged` explícito. **Se arrastra a Fase 08** (flujo de adopción), que es donde la retención se implementa de verdad.
  **Y queda corregido el comentario de T-01-027** que justificaba la `CASCADE` de `form_submissions.application_id` con esta purga: con historia presente esa cascada **no se dispara nunca**, porque la solicitud no se borra en duro. La cascada sigue siendo correcta para el caso sin historia; la purga real es el `DELETE` directo sobre `form_submissions`.

- [x] **ARRASTRADA A FASE 02 (T-01-035).** `refresh_tokens`: ruta de acceso. Default-deny esta fase; Fase 02 elige entre un rol `app_auth` dedicado o una GUC `app.user_id` junto a la del tenant. **No es una decisión pendiente, es alcance de otra fase:** default-deny es *más* estricto que cualquier política que se pudiera escribir antes de que Fase 02 elija su camino de acceso, así que Fase 01 cierra en el estado seguro. `TestRefreshTokens_RefuseAppTenantEntirely` lo fija.
- [x] Quién puede **escribir** una fila de `users`. **Resuelto en T-01-013 tal como estaba escrito:** la política es `FOR SELECT` y el grant es `SELECT`, y `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` fija las dos capas. El alta pertenece al registro de Fase 02.
- [x] **ARRASTRADA A FASE 06 (T-01-035).** **`app_public` no tiene política sobre `shelters` y ninguna tarea la crea.** El diseño lista `shelters` entre las tablas públicas, pero el tablero asigna políticas públicas solo a `pets` y `media` (T-01-019), con la suite en T-01-022. T-01-013 decidió **no inventar** un acceso de lectura que ningún test cubre. La Fase 06 necesita datos del refugio para el catálogo público: el hueco es real y pertenece a quien abra esa fase.
  **Por qué se arrastra y no se resuelve:** inventar hoy una política de lectura pública que ningún test cubre es exactamente lo que T-01-013 decidió no hacer, y el grant que la acompañaría sería la forma más ancha posible sin nadie que la angoste. Además se ata con LT-2: cuando esa condición aterrice, `app_public` va a necesitar `SELECT` sobre `shelters` de todas formas, así que las dos deciden juntas. Anotado en `[[FASE-06]]`.
- [x] **ARRASTRADA A FASE 02/03 (T-01-035).** **Escalada de privilegios DENTRO del tenant (Judgment Day T-01-016, hallazgo B1, confirmado en vivo).** Con grants a nivel tabla, `app_tenant` puede sobre su propia fila: ponerse `status = 'verified'` (**bypass de LT-2 — el refugio se auto-verifica**), subirse `storage_quota_bytes`, y ponerse `role = 'owner'` en `memberships`. El arreglo son grants a nivel columna; qué columnas puede escribir un tenant depende de endpoints que la Fase 03 todavía no escribió. Diferido por decisión del usuario a **Fase 02 (RBAC) y Fase 03**.
  **Confirmado arrastrado en T-01-035**, y con marcador: `TestApplicantPolicy_IsAsWideAsWritingAnApplication` se pone rojo el día que los grants a nivel columna aterricen, así que el cambio va a ser visible y no silencioso. Está escrito en `[[ADR-0009]]` como la reapertura más probable de esa decisión.
- [x] **El meta-test no ve para quién es una política (T-01-016, hallazgo B2, determinista).** **Descargado en T-01-017**: `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` ensanchado a `media` (5 tablas, 11 aserciones de privilegio). Cada migración de acá en más agrega sus filas a esas dos tablas. Original: `CheckProtection` cuenta filas de `pg_policy` sin mirar `polroles` ni si el `USING` es tautológico. Una migración futura atada al rol equivocado pasa como protegida. Diferido a **T-01-017**, la primera que lo atraviesa.
- [x] **ARRASTRADA A FASE 06 (T-01-035).** **`pets.breed_id` permite una raza de otra especie (hallazgo de T-01-019).** La FK es de una columna a `breeds (id)`, asi que `species_id = gato` con `breed_id = Labrador` es representable. Por el principio de que la base es la ultima linea de defensa deberia ser FK compuesta a `breeds (id, species_id)`, lo que exige `UNIQUE (id, species_id)` en `breeds`, declarada en `00004`. No es frontera de seguridad: corrompe el filtro del adoptante (AD-3), no el aislamiento. Reabrir una migracion cerrada quedaba fuera del build list de T-01-019.
  **Por qué a Fase 06 y no antes:** el daño es al filtro tipado del adoptante (AD-3), y ese filtro se construye ahí. El arreglo está escrito y es barato — `UNIQUE (id, species_id)` en `breeds` más FK compuesta desde `pets` —, pero es una migración nueva que corrige una cerrada, y conviene que aterrice junto al código que la necesita. Anotado en `[[FASE-06]]`.
- [x] **Si `pet_status_history` debe ser append-only. RESUELTO POR EL USUARIO en T-01-020: sí, append-only, y con la FK que lo hace real.** El tablero decía "§4 no lo dice", lo cual es cierto de §4 y falso del plan: la mitigación de **LT-5** en §1.1 dice literalmente *"Estados de `pets` + `pet_status_history` **inmutable desde el día 1**"*, y `FASE-05.md` ya lo arrastraba. **El hallazgo que convirtió esto en una decisión de verdad:** el design escribe esta FK con `ON DELETE CASCADE` y `app_tenant` tiene `DELETE` sobre `pets`, así que append-only con cascada es **teatro** — el refugio borra el rastro borrando el animal, sin tocar la tabla protegida. Va con `ON DELETE RESTRICT`.
- [x] **`pet_media` sí tiene columna `id`? RESUELTO en T-01-020: no, su PK es el par `(pet_id, media_id)`.** Una fila de join pura no necesita identidad propia, y `RowKey` del runner A/B se diseñó como mapa de columnas justamente para esto.


---

## Tareas

**35 tareas · ~4.112 líneas autoradas · 15 PRs.** Detalle en `tasks.md`.

| ID | Tarea | PR | est. |
|---|---|---|---|
| `[x]` `T-01-001` | Batch every Go dependency in one `go get` | PR 1 | 30 | 
| `[x]` `T-01-002` | Spike: prove `testcontainers-go` works under `CGO_ENABLED=0` | PR 1 | 145 |
| `[x]` `T-01-003` | RED — `WithTenant` unit tests against a recording `pgx.Tx` stub | PR 2 | 160 |
| `[x]` `T-01-004` | GREEN — pool construction and the `WithTenant` contract | PR 2 | 145 |
| `[x]` `T-01-005` | RED — embedded migration set invariants, no database | PR 3 | 90 |
| `[x]` `T-01-006` | GREEN — migration embedding and password bootstrap | PR 3 | 85 |
| `[x]` `T-01-007` | Migration `00001_extensions_roles_and_grants.sql` | PR 3 | 62 |
| `[x]` `T-01-008` | **Role guard** — the connecting role cannot bypass RLS | PR 4 | 55 |
| `[x]` `T-01-009` | A/B isolation harness and tenant fixtures | PR 4 | 220 |
| `[x]` `T-01-010` | **Catalog meta-test** — no table escapes classification | PR 4 | 110 |
| `[x]` `T-01-011` | Migration round-trip: up → down-to 0 → up | PR 4 | 70 |
| `[x]` `T-01-012` | Build wiring: sqlc, Makefile, CI, devcontainer, env | PR 3 | 88 |
| `[x]` `T-01-013` | Migration `00002_tenancy_identity.sql` — `shelters`, `users`, `memberships`, `refresh_tokens` | PR 5 | 214 |
| `[x]` `T-01-014` | `users` visibility and email uniqueness assertions | PR 5 | 110 |
| `[x]` `T-01-015` | Transaction-local scope and default-deny assertions | PR 5 | 100 |
| `[x]` `T-01-016` | 🔴 **Judgment Day** — adversarial review before the RLS policies merge | PR 5 | 0 |
| `[x]` `T-01-017` | Migration `00003_media.sql` | PR 6 | 72 |
| `[x]` `T-01-018` | Migration `00004_reference_data.sql` — `species`, `breeds` + seeds | PR 6 | 170 |
| `[x]` `T-01-019` | Migration `00005_pets.sql` — the catalog core | PR 7 | 239 |
| `[x]` `T-01-020` | Migration `00006_pet_children.sql` — `pet_media`, `pet_health_records`, `pet_status_history` | PR 8 | 171 |
| `[x]` `T-01-021` | **Child-table orphan test** — cross-tenant attachment fails with `23503` | PR 8 | 105 |
| `[x]` `T-01-022` | `app_public` is read-only and shows only published pets | PR 8 | 120 |
| `[x]` `T-01-023` | Migration `00007_form_templates.sql` + published-version immutability trigger | PR 9 | 154 |
| `[x]` `T-01-024` | Form immutability and versioning assertions + escenario end-to-end de legibilidad histórica | PR 9 | 110 |
| `[x]` `T-01-025` | Migration `00008_form_submissions.sql` + orphan case and enumerated child guard | PR 10 | 72 |
| `[x]` `T-01-026` | Forms orphan case and GIN index assertion — **pase de verificación**: 2 enmiendas a la spec + 1 decisión abierta | PR 10 | 45 |
| `[x]` `T-01-027` | Migration `00009_adoption_applications.sql` + the applicant `users` policy | PR 11 | 117 |
| `[x]` `T-01-028` | Adoption-flow domain assertions | PR 11 | 90 |
| `[x]` `T-01-029` | Migration `00010_application_children.sql` — `application_events`, `application_notes` | PR 12 | 174 |
| `[x]` `T-01-030` | Append-only enforcement suite — **enumerada** sobre `Schema.AppendOnly` | PR 12 | 85 |
| `[x]` `T-01-031` | Migration `00011_documents.sql` | PR 13 | 77 |
| `[x]` `T-01-032` | Migration `00012_audit_log.sql` + append-only coverage extended | PR 13 | 162 |
| `[x]` `T-01-033` | sqlc query inputs and committed generated output | PR 14 | 200 |
| `[x]` `T-01-034` | Promote five decisions to ADRs | PR 15 | 225 |
| `[x]` `T-01-035` | Backfill the phase board and project state | PR 15 | 40 |

**Bloqueantes conocidos:** ninguno. Docker Desktop v29.7.2 verificado corriendo
(`server=29.7.2 api=1.55 linux/amd64`) el 2026-08-29 — la suite de aislamiento lo necesita.

---

## Salida de fase

1. ✅ Todas las tareas en `[x]` — 35 de 35.
2. ✅ Verificación de fase del plan maestro ejecutada y registrada (bitácora `2026-09-01`).
3. ✅ Cola de Engram vacía — 52 guardados con `observation_id`, 2 descartados.
4. ✅ **`/sdd-verify` en verde → `/sdd-archive`, 2026-09-01.** Verify: **PASS**, 0 críticos / 0
   warnings / 0 sugerencias, verificado **contra el código** (corrió `make test-api-container`,
   leyó `catalog.go` y contó `abCases()`). Archive: los siete deltas fusionados en
   `openspec/specs/` — **30 requisitos, 72 escenarios** — y el change movido a
   `openspec/changes/archive/2026-09-01-phase-01-domain-and-data/`.
   **Corrección aplicada tras el archive:** la fusión había dejado los specs principales
   byte-idénticos al delta, con título `# Delta for X` y encabezado `## ADDED Requirements`.
   Eso es una sección **de delta**, no de spec principal, y habría vuelto ambigua la próxima
   fusión con `## MODIFIED Requirements`. Corregido con conteo antes/después para probar que no
   se perdió contenido.

Siguiente: [[FASE-02]]
