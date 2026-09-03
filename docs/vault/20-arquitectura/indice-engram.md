# Índice de memoria — Engram

> **Generado por `make engram-index`. No editar a mano.**
> Fuente: el frontmatter de `.engram/queue/*.md`.

Engram es un índice semántico al que **solo llega Claude Code por MCP**.
Cualquier otro agente —Kiro, OpenCode, una sesión fría sin MCP— ve el
repositorio y nada más. Este archivo proyecta esa capa dentro del vault para
que el repositorio siga siendo la verdad operativa (regla IA-4 del plan maestro).

**El texto completo de cada decisión vive en su archivo de cola**, que está
versionado. El `observation_id` es la misma nota dentro de Engram; sirve para
trazabilidad, no es requisito para leerla.

- Candidatos totales: **79**
- Aprobados y guardados en Engram: **76**
- Pendientes de aprobación explícita del usuario: **1**
- Descartados por el pase de curaduría: **2**

## Pendientes de aprobación

Nada de esto está en Engram todavía. Requiere el sí explícito del usuario
antes de `mem_save` (§6.6 del plan maestro).

| Tarea | Tipo | Score | Archivo | Qué dice |
|---|---|---|---|---|
| `T-02-003` | constraint | 4 | [2026-09-02-has-table-privilege-no-ve-columnas.md](../../../.engram/queue/2026-09-02-has-table-privilege-no-ve-columnas.md) | Verificado en vivo contra PostgreSQL 17, no asumido: |

## Descartados — NO guardar

El pase de curaduría los rechazó. **No tienen `observation_id` porque fueron
descartados, no porque esperen aprobación.** Guardarlos metería en Engram
exactamente lo que la columna de abajo explica que está mal.

| Tarea | Archivo | Por qué se descartó | Reemplazado por |
|---|---|---|---|
| `T-01-002 (spike testcontainers)` | [2026-08-29-windows-appcontrol-blocks-go-test-binaries.md](../../../.engram/queue/2026-08-29-windows-appcontrol-blocks-go-test-binaries.md) | descartado -- su causa raiz se probo FALSA y el que la corrige ya esta guardado | `2026-08-31-smart-app-control-no-es-defender.md` |
| `T-01-015` | [2026-08-30-defender-no-application-control.md](../../../.engram/queue/2026-08-30-defender-no-application-control.md) | descartado -- su causa raiz se probo FALSA y el que la corrige ya esta guardado | `2026-08-31-smart-app-control-no-es-defender.md` |

## Guardadas

### `mascotapp/arch/*` — 6

| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |
|---|---|---|---|---|---|
| `T-00-010` | `…/multitenancy` | 5 | [2026-08-28-rls-multitenancy.md](../../../.engram/queue/2026-08-28-rls-multitenancy.md) | `obs-4ae5918c16167e36` | Multi-tenancy de MascotApp: base compartida + esquema compartido + discriminador shelter_id + Row Level Security de PostgreSQL, c… |
| `T-01-020` | `…/public-catalog` | 4 | [2026-08-31-composicion-en-vez-de-duplicar-el-predicado-publico.md](../../../.engram/queue/2026-08-31-composicion-en-vez-de-duplicar-el-predicado-publico.md) | `obs-db33d8e90bb53b65` | La politica publica de media aterrizo en su tercera planificacion (00003, despues 00005, y por fin 00006 — cuando existe pet_medi… |
| `T-01-025` | `…/d5-child-side-conformance` | 5 | [2026-09-01-la-conformidad-de-d5-se-pregunta-desde-el-hijo.md](../../../.engram/queue/2026-09-01-la-conformidad-de-d5-se-pregunta-desde-el-hijo.md) | `obs-5b59704241bc94ab` | La clave compuesta de D5 tenia dos verificaciones y ninguna de las dos cubria un hijo nuevo: |
| `T-01-027` | `…/cascade-direction-follows-the-child` | 5 | [2026-09-01-la-direccion-de-la-cascada-la-decide-quien-es-el-hijo.md](../../../.engram/queue/2026-09-01-la-direccion-de-la-cascada-la-decide-quien-es-el-hijo.md) | `obs-5711866a288ae91b` | Este esquema puso ON DELETE RESTRICT en cada referencia compuesta, y por buenas razones cada vez: el hijo era la historia y el pa… |
| `T-01-027` | `…/assignment-bounded-by-membership-key` | 5 | [2026-09-01-una-clave-compuesta-a-memberships-encierra-la-asignacion.md](../../../.engram/queue/2026-09-01-una-clave-compuesta-a-memberships-encierra-la-asignacion.md) | `obs-cb3fdedda1fa5e49` | adoption_applications.assigned_to_user_id no es una referencia a users. Es una clave compuesta a memberships (user_id, shelter_id… |
| `T-01-034` | `…/verify-inherited-constraints` | 5 | [2026-09-01-una-restriccion-heredada-se-verifica-antes-de-disenar-contra-ella.md](../../../.engram/queue/2026-09-01-una-restriccion-heredada-se-verifica-antes-de-disenar-contra-ella.md) | `obs-ed6c4139b505a445` | D3 decia: *"no dependemos de extensiones de PostgreSQL, porque el free tier de Neon no las garantiza"*. |

### `mascotapp/convention/*` — 40

| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |
|---|---|---|---|---|---|
| `PR-02-01 (T-02-001/002)` | `…/exit-code-through-a-pipe` | 3 | [2026-09-02-un-pipe-se-come-el-codigo-de-salida.md](../../../.engram/queue/2026-09-02-un-pipe-se-come-el-codigo-de-salida.md) | `obs-e5c386557333aa6a` | make test-api-container 2>&1 / tail -60 reporto exit code 0 mientras la salida contenia make: * [Makefile:184: test-api-container… |
| `PR-02-01 (T-02-001/002)` | `…/makefile-tool-provenance` | 4 | [2026-09-02-un-target-que-corre-en-tu-maquina-no-es-un-target.md](../../../.engram/queue/2026-09-02-un-target-que-corre-en-tu-maquina-no-es-un-target.md) | `obs-011873b42b4352bd` | TestDevcontainerInstallsEveryToolTheMakefileInvokes (apps/api/internal/db/wiring_test.go) exige que toda herramienta que invoca u… |
| `T-00-022` | `…/mutation-testing-finds-vacuous-coverage` | 4 | [2026-08-29-mutation-testing-vacuous-coverage.md](../../../.engram/queue/2026-08-29-mutation-testing-vacuous-coverage.md) | `obs-7472df2f86aeaeb4` | En MascotApp, un test en verde no cuenta como prueba hasta que se muta la implementación y el test falla. No es ceremonia: ya atr… |
| `T-01-002 (spike testcontainers)` | `…/go-get-subpackages` | 3 | [2026-08-29-go-get-no-resuelve-subpaquetes.md](../../../.engram/queue/2026-08-29-go-get-no-resuelve-subpaquetes.md) | `obs-79c957e4c6787855` | go get sobre la raíz de un módulo NO resuelve las dependencias de sus subpaquetes. |
| `T-01-005 / T-01-006 (incidente de truncado)` | `…/hybrid-store-is-recovery` | 4 | [2026-08-29-store-hibrido-salvo-el-artefacto.md](../../../.engram/queue/2026-08-29-store-hibrido-salvo-el-artefacto.md) | `obs-90f889e7090661ac` | El store híbrido de SDD no es papeleo. Es la copia de recuperación. |
| `T-01-008 (role guard)` | `…/guarantees-must-be-observable` | 4 | [2026-08-30-testear-un-check-no-es-testear-que-corre.md](../../../.engram/queue/2026-08-30-testear-un-check-no-es-testear-que-corre.md) | `obs-c12eb1e15c811174` | Testear un check no es testear que el check corre. |
| `T-01-009 (runner A/B)` | `…/one-broken-fixture-per-check` | 4 | [2026-08-30-una-tabla-rota-no-alcanza.md](../../../.engram/queue/2026-08-30-una-tabla-rota-no-alcanza.md) | `obs-d5109c9408820758` | Un fixture roto prueba que ALGUNA verificación funciona, no que cada una funciona. |
| `T-01-010` | `…/a-survivor-may-be-dead-code` | 3 | [2026-08-30-la-mutacion-encuentra-codigo-muerto.md](../../../.engram/queue/2026-08-30-la-mutacion-encuentra-codigo-muerto.md) | `obs-94029dcc990eb3b4` | En T-01-010 sobrevivió el mutante que borraba el helper duplicates() de Validate. El reflejo es escribir un test que lo cubra. |
| `T-01-010` | `…/green-over-nothing-must-say-so` | 4 | [2026-08-30-un-suite-verde-sobre-cero-tablas.md](../../../.engram/queue/2026-08-30-un-suite-verde-sobre-cero-tablas.md) | `obs-b4c7ca784082649e` | El meta-test de catálogo de T-01-010 declara 19 tablas del modelo y afirma que cada una tiene relrowsecurity, relforcerowsecurity… |
| `T-01-011` | `…/one-sentinel-per-rule` | 4 | [2026-08-30-dos-reglas-un-centinela.md](../../../.engram/queue/2026-08-30-dos-reglas-un-centinela.md) | `obs-589b052a73fbade8` | En T-01-011 sobrevivió el mutante que desactivaba la regla de exhaustividad de checkCatalogAt. El test que la cubría seguía en ve… |
| `T-01-011` | `…/a-round-trip-misses-the-middle` | 4 | [2026-08-30-un-round-trip-no-ve-el-medio-del-camino.md](../../../.engram/queue/2026-08-30-un-round-trip-no-ve-el-medio-del-camino.md) | `obs-84bfb6c00194e6d2` | goose up → down-to 0 → up es la forma canónica de probar migraciones reversibles. Y con dos migraciones ya es insuficiente. |
| `T-01-012` | `…/defer-with-a-guard-not-a-stub` | 4 | [2026-08-30-diferir-con-guarda-en-vez-de-stub.md](../../../.engram/queue/2026-08-30-diferir-con-guarda-en-vez-de-stub.md) | `obs-0baa00784ecb7128` | sqlc generate falla con un directorio de queries vacío — verificado contra sqlc v1.31.1: error parsing queries: no queries contai… |
| `T-01-012` | `…/mutate-the-config-not-the-test` | 3 | [2026-08-30-mutar-la-config-no-el-test.md](../../../.engram/queue/2026-08-30-mutar-la-config-no-el-test.md) | `obs-41e556e63b5b8722` | Los tests de T-01-012 no ejercitan código Go: leen sqlc.yaml, el Makefile y postCreate.sh, y afirman propiedades sobre ellos. |
| `T-01-013` | `…/a-loosened-rule-keeps-a-test` | 3 | [2026-08-30-aflojar-una-regla-de-seguridad-cuesta-un-test.md](../../../.engram/queue/2026-08-30-aflojar-una-regla-de-seguridad-cuesta-un-test.md) | `obs-a5ffe14020786d7e` | El guard de D8 ("ninguna contraseña aparece en el texto de una migración") era un match por substring sobre PASSWORD. La migració… |
| `T-01-013` | `…/mutation-kill-requires-a-fail-line` | 4 | [2026-08-30-el-harness-de-mutacion-que-medía-el-host.md](../../../.engram/queue/2026-08-30-el-harness-de-mutacion-que-medía-el-host.md) | `obs-85289f0218a5964f` | El harness era: aplicar el mutante, correr go test, y |
| `T-01-014` | `…/a-visibility-grid-needs-both-directions` | 3 | [2026-08-30-una-grilla-de-visibilidad-necesita-las-dos-direcciones.md](../../../.engram/queue/2026-08-30-una-grilla-de-visibilidad-necesita-las-dos-direcciones.md) | `obs-07442a03c4820e61` | El spec pedía dos escenarios: un miembro del refugio actual es visible; un usuario sin relación con el refugio actual es invisibl… |
| `T-01-015` | `…/a-rotating-survivor-outranks-a-stable-one` | 4 | [2026-08-30-un-sobreviviente-que-rota-vale-mas-que-uno-estable.md](../../../.engram/queue/2026-08-30-un-sobreviviente-que-rota-vale-mas-que-uno-estable.md) | `obs-3b0eca453ae1938e` | Ronda 1: murió S1, sobrevivió S2. Arreglo el hueco que explicaba S2. Ronda 2: murió S2, sobrevivió S1 — que ya había muerto. |
| `T-01-016` | `…/a-blind-judge-findings-are-inferential` | 4 | [2026-08-30-un-juez-ciego-no-puede-ejecutar.md](../../../.engram/queue/2026-08-30-un-juez-ciego-no-puede-ejecutar.md) | `obs-94366e6a3c8b917d` | Judgment Day corre dos jueces ciegos read-only. El contrato dice: se arregla solo lo que confirman los dos. La regla existe para… |
| `T-01-017` | `…/the-mutant-carries-its-target-package` | 4 | [2026-08-30-el-paquete-objetivo-es-parte-del-mutante.md](../../../.engram/queue/2026-08-30-el-paquete-objetivo-es-parte-del-mutante.md) | `obs-003e71fb99b7d894` | Ronda 2 de mutación sobre 00003_media.sql: borrar el DROP CONSTRAINT del Down sobrevivió. Fui a buscar el hueco de cobertura. |
| `T-01-018` | `…/an-unfalsifiable-spec-scenario` | 4 | [2026-08-30-un-escenario-de-spec-que-no-puede-fallar.md](../../../.engram/queue/2026-08-30-un-escenario-de-spec-que-no-puede-fallar.md) | `obs-1e24d237d733ecb6` | El spec pedía, para los seeds de datos de referencia: |
| `T-01-019` | `…/probe-names-come-from-the-ledger` | 3 | [2026-08-30-un-test-no-toma-prestado-un-nombre-que-el-esquema-va-a-reclamar.md](../../../.engram/queue/2026-08-30-un-test-no-toma-prestado-un-nombre-que-el-esquema-va-a-reclamar.md) | `obs-6873e19af4076fac` | Un test de T-01-011 necesitaba una tabla declarada pero todavía no creada — esa forma exacta, para que la regla de exhaustividad… |
| `T-01-020` | `…/testing-aborted-transaction` | 3 | [2026-08-31-el-primer-rechazo-aborta-la-transaccion.md](../../../.engram/queue/2026-08-31-el-primer-rechazo-aborta-la-transaccion.md) | `obs-8ab9e31b8abb632d` | El caso A/B de pet_status_history sondea UPDATE y despues DELETE, ambos esperando ser rechazados. El UPDATE fallaba como correspo… |
| `T-01-020` | `…/testing-conformance-flags` | 3 | [2026-08-31-extender-el-runner-ab-no-eximir-la-tabla.md](../../../.engram/queue/2026-08-31-extender-el-runner-ab-no-eximir-la-tabla.md) | `obs-64c7b74b30e1800a` | El runner A/B afirma, tabla por tabla, que el tenant B no lee ni escribe filas de A. En una tabla append-only las sondas de UPDAT… |
| `T-01-020` | `…/testing-catalog-assertions` | 3 | [2026-08-31-indkey-es-un-array-con-lower-bound-cero.md](../../../.engram/queue/2026-08-31-indkey-es-un-array-con-lower-bound-cero.md) | `obs-ce88a1825e176a88` | hasPartialUniqueOn verifica que pet_media tenga UNIQUE (shelter_id, pet_id) WHERE is_primary. Lee pg_index y no pg_constraint, po… |
| `T-01-021` | `…/testing-shared-fixtures` | 4 | [2026-08-31-las-fixtures-compartidas-son-un-acoplamiento.md](../../../.engram/queue/2026-08-31-las-fixtures-compartidas-son-un-acoplamiento.md) | `obs-3acb038bf64919cf` | El paquete rlstest comparte un unico Postgres. child_orphan_test.go reusaba env.ShelterA / env.ShelterB, ordena primero alfabetic… |
| `T-01-022` | `…/testing-layer-attribution` | 4 | [2026-08-31-dos-capas-un-solo-sqlstate.md](../../../.engram/queue/2026-08-31-dos-capas-un-solo-sqlstate.md) | `obs-fedf7a4ef4efc6e3` | El test de escrituras publicas afirmaba, en su propio comentario, que el caso sobre transaccion ordinaria fijaba los grants de ap… |
| `T-01-023` | `…/plpgsql-triggers` | 4 | [2026-08-31-return-new-en-un-before-delete-cancela-en-silencio.md](../../../.engram/queue/2026-08-31-return-new-en-un-before-delete-cancela-en-silencio.md) | `obs-19d7bb85bca38508` | En un trigger BEFORE ... FOR EACH ROW, devolver NULL cancela la operacion para esa fila: sin error, sin aviso, y RowsAffected() =… |
| `T-01-023` | `…/inventory-from-declaration` | 4 | [2026-08-31-una-lista-a-mano-en-un-inventario-excluye-en-silencio.md](../../../.engram/queue/2026-08-31-una-lista-a-mano-en-un-inventario-excluye-en-silencio.md) | `obs-a2740164931022d6` | El inventario de politicas filtraba con un c.relname IN ('shelters', 'users', ...) hardcodeado. Cuando T-01-023 agrego form_templ… |
| `T-01-024` | `…/the-layer-that-answers-first` | 5 | [2026-09-01-una-capa-que-contesta-primero-vacia-el-test-de-abajo.md](../../../.engram/queue/2026-09-01-una-capa-que-contesta-primero-vacia-el-test-de-abajo.md) | `obs-9e00e3a6413dc7d2` | Un test que espera un rechazo no prueba cual capa lo rechazo. Si una capa mas arriba contesta antes, el test sigue verde y la cap… |
| `T-01-025` | `…/equivalent-mutant-disposition` | 4 | [2026-09-01-un-mutante-equivalente-se-prueba-no-se-acepta.md](../../../.engram/queue/2026-09-01-un-mutante-equivalente-se-prueba-no-se-acepta.md) | `obs-773c9a9bbd23df5a` | Un mutante que sobrevive tiene tres destinos posibles y ninguno es aceptarlo: |
| `T-01-025` | `…/plan-assertions-name-the-index` | 4 | [2026-09-01-una-asercion-de-plan-nombra-el-indice.md](../../../.engram/queue/2026-09-01-una-asercion-de-plan-nombra-el-indice.md) | `obs-a284e13047458786` | Dos cosas, encontradas en la misma corrida y las dos por el mismo motivo: la RLS deforma el plan. |
| `T-01-027` | `…/enumerated-guard-must-quantify` | 4 | [2026-09-01-un-guard-enumerado-tambien-puede-preguntar-de-menos.md](../../../.engram/queue/2026-09-01-un-guard-enumerado-tambien-puede-preguntar-de-menos.md) | `obs-535e166e505a8722` | Enumerar en vez de listar a mano fue la leccion de T-01-022, T-01-023 y T-01-025. No alcanza. El guard enumerado tiene su propio… |
| `T-01-029` | `…/each-layer-dies-in-its-own-test` | 5 | [2026-09-01-cada-capa-tiene-que-morir-en-un-test-distinto.md](../../../.engram/queue/2026-09-01-cada-capa-tiene-que-morir-en-un-test-distinto.md) | `obs-5ea94dcd00c0adb7` | Decir *"esto esta protegido por cuatro capas independientes"* no significa nada hasta que se muestra que sacar cualquiera de las… |
| `T-01-029` | `…/close-a-set-when-the-domain-branches` | 4 | [2026-09-01-un-conjunto-se-cierra-cuando-el-dominio-ramifica.md](../../../.engram/queue/2026-09-01-un-conjunto-se-cierra-cuando-el-dominio-ramifica.md) | `obs-e0fefde5e839d79a` | 00010 creo dos columnas de conjunto y les dio tratamientos opuestos, en la misma migracion. El criterio no es la costumbre; es un… |
| `T-01-033` | `…/a-substring-is-not-an-assertion` | 5 | [2026-09-01-un-substring-en-un-archivo-no-es-una-asercion.md](../../../.engram/queue/2026-09-01-un-substring-en-un-archivo-no-es-una-asercion.md) | `obs-e4c1ca6f36c7b65f` | strings.Contains(archivo, "algo") responde *"la palabra aparece"*, no *"el archivo hace eso"*. Y un comentario satisface la prime… |
| `T-01-033` | `…/queries-never-filter-by-shelter-id` | 5 | [2026-09-01-una-query-no-filtra-por-shelter-id.md](../../../.engram/queue/2026-09-01-una-query-no-filtra-por-shelter-id.md) | `obs-c94143b56078801e` | Regla dura para todo internal/db/query/*.sql: ninguna lectura lleva shelter_id en un WHERE, en un AND ni en una condicion de JOIN. |
| `fase-02 planning` | `…/recompute-announced-numbers` | 4 | [2026-09-02-un-numero-anunciado-se-recomputa.md](../../../.engram/queue/2026-09-02-un-numero-anunciado-se-recomputa.md) | `obs-99a194c5559106b8` | Un agente cerro la cadena de entrega de la Fase 02 con 23 PRs, 4.875 lineas, ninguno sobre 400. Los tres numeros eran del mismo r… |
| `fase-02 planning` | `…/spec-must-carry-the-property` | 5 | [2026-09-02-un-requisito-que-el-verificador-no-puede-leer.md](../../../.engram/queue/2026-09-02-un-requisito-que-el-verificador-no-puede-leer.md) | `obs-51c9db2915bb5533` | El diseno de la Fase 02 prohibe que un tenant escriba shelters.storage_bytes_used — el contador contra el que se chequea la cuota… |
| `handoff multi-agente` | `…/commit-messages` | 3 | [2026-09-02-convencion-de-commits.md](../../../.engram/queue/2026-09-02-convencion-de-commits.md) | `obs-62d33d3023d8afae` | Los commits de MascotApp siguen Conventional Commits, y la convencion esta escrita en dos lugares ejecutables, no en la memoria d… |
| `phase-01-domain-and-data (design D7, revertida por el usuario)` | `…/case-insensitive-email` | 4 | [2026-08-29-citext-vs-lower-index.md](../../../.engram/queue/2026-08-29-citext-vs-lower-index.md) | `obs-1ee4f1b598f5b471` | users.email es citext, no text + índice único sobre lower(email). |

### `mascotapp/domain/*` — 4

| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |
|---|---|---|---|---|---|
| `T-01-013` | `…/a-policy-that-reads-blocks-drop-table` | 4 | [2026-08-30-una-politica-que-lee-otra-tabla-es-una-dependencia.md](../../../.engram/queue/2026-08-30-una-politica-que-lee-otra-tabla-es-una-dependencia.md) | `obs-57ee5e7a04dc4d61` | member_visible_users vive en users pero lee memberships: |
| `T-01-020` | `…/append-only-tables` | 5 | [2026-08-31-append-only-con-fk-que-cascadea-es-teatro.md](../../../.engram/queue/2026-08-31-append-only-con-fk-que-cascadea-es-teatro.md) | `obs-99c2e3298b483ab8` | pet_status_history es inmutable desde el dia 1 (mitigacion LT-5). Se implemento con cuatro capas — politicas por comando, REVOKE… |
| `T-01-022` | `…/shelter-verification` | 5 | [2026-08-31-lt2-no-lo-hace-cumplir-la-base.md](../../../.engram/queue/2026-08-31-lt2-no-lo-hace-cumplir-la-base.md) | `obs-85415c0b8700bec6` | §1.1 del plan pone pending_verification como requisito duro de MVP: *"un refugio no puede publicar hasta ser verificado manualmen… |
| `T-01-024` | `…/historical-readability-from-the-renderer` | 5 | [2026-09-01-la-legibilidad-historica-se-afirma-desde-el-renderer.md](../../../.engram/queue/2026-09-01-la-legibilidad-historica-se-afirma-desde-el-renderer.md) | `obs-069b75a9882b4f07` | §4.4 regla 1 dice que una respuesta enviada siempre se renderiza contra la version con la que se lleno. Hay dos formas de "probar… |

### `mascotapp/ops/*` — 7

| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |
|---|---|---|---|---|---|
| `T-00-017` | `…/agent-supervision` | 5 | [2026-08-28-supervision-agentes-bypass.md](../../../.engram/queue/2026-08-28-supervision-agentes-bypass.md) | `obs-86bec7dbc4378a13` | Qué se hace cumplir de verdad en Claude Code, verificado contra la documentación oficial (agosto 2026): |
| `T-00-022` | `…/line-endings` | 3 | [2026-08-29-line-endings-ci-gate.md](../../../.engram/queue/2026-08-29-line-endings-ci-gate.md) | `obs-7c3386a868ab728a` | El repo se desarrolla en Windows con core.autocrlf=true y CI corre en Linux. Sin .gitattributes eso hace que los finales de línea… |
| `T-01-004 (construccion de pools)` | `…/neon-pool-config-cost` | 5 | [2026-08-29-pool-config-quema-el-free-tier-de-neon.md](../../../.engram/queue/2026-08-29-pool-config-quema-el-free-tier-de-neon.md) | `obs-0a07268799d53b37` | En Neon free, la configuración del pool es un contrato de COSTO, no una preferencia de tuning. Los defaults de pgxpool agotan el… |
| `T-01-013` | `…/testcontainers-windows-provider-race` | 4 | [2026-08-30-el-flake-de-testcontainers-en-windows.md](../../../.engram/queue/2026-08-30-el-flake-de-testcontainers-en-windows.md) | `obs-df8668122e55b55d` | Síntoma: un paquete entero falla a 0.00s, todos sus tests con el mismo mensaje: |
| `T-01-020` | `…/windows-appcontrol-go-tests` | 5 | [2026-08-31-smart-app-control-no-es-defender.md](../../../.engram/queue/2026-08-31-smart-app-control-no-es-defender.md) | `obs-09d0eb05887b634f` | El sintoma nunca cambio: |
| `T-01-032` | `…/bigserial-needs-a-sequence-grant` | 5 | [2026-09-01-bigserial-necesita-un-grant-que-no-es-sobre-una-tabla.md](../../../.engram/queue/2026-09-01-bigserial-necesita-un-grant-que-no-es-sobre-una-tabla.md) | `obs-64400c1e36f5e3ab` | bigserial no es un tipo. Es bigint + una SECUENCIA + un DEFAULT nextval(...). Un grant de tabla no dice nada sobre esa secuencia. |
| `fase-02 planning` | `…/failed-report-is-not-lost-work` | 3 | [2026-09-02-un-agente-que-falla-al-reportar-no-fallo-al-escribir.md](../../../.engram/queue/2026-09-02-un-agente-que-falla-al-reportar-no-fallo-al-escribir.md) | `obs-216cdb370cd7e725` | sdd-design se cayo por limite de sesion de proveedor. Su ultimo texto era *"Now I have the full picture. Writing the design."*, y… |

### `mascotapp/security/*` — 19

| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |
|---|---|---|---|---|---|
| `T-00-011` | `…/transport` | 4 | [2026-08-28-tls-sin-mtls.md](../../../.engram/queue/2026-08-28-tls-sin-mtls.md) | `obs-b24f892dc7c32085` | MascotApp usa TLS 1.3 sin mTLS. Motivo por canal: |
| `T-00-022` | `…/client-ip-resolution` | 5 | [2026-08-29-client-ip-resolution.md](../../../.engram/queue/2026-08-29-client-ip-resolution.md) | `obs-878517463d368981` | La IP de cliente en MascotApp se resuelve con la primera entrada no confiable de X-Forwarded-For, escaneando de derecha a izquier… |
| `T-01-009 (runner A/B)` | `…/unqualified-delete-bypasses-select-policy` | 5 | [2026-08-30-delete-sin-where-esquiva-la-politica-select.md](../../../.engram/queue/2026-08-30-delete-sin-where-esquiva-la-politica-select.md) | `obs-81f7b8b17c3780db` | Un DELETE ... WHERE nunca puede detectar una política DELETE permisiva. Solo lo hace un DELETE FROM tabla sin WHERE. |
| `T-01-013` | `…/a-policy-count-hides-its-role` | 4 | [2026-08-30-contar-politicas-no-dice-para-quien-son.md](../../../.engram/queue/2026-08-30-contar-politicas-no-dice-para-quien-son.md) | `obs-20b9dbca96480d36` | El meta-test del catálogo afirma, para cada tabla: RLS habilitada, RLS forzada, y al menos una política. Suena completo. La mutac… |
| `T-01-013` | `…/with-check-precedes-the-unique-index` | 4 | [2026-08-30-rls-refusa-antes-que-el-indice-unico.md](../../../.engram/queue/2026-08-30-rls-refusa-antes-que-el-indice-unico.md) | `obs-fab580eb9736dc4c` | Verificado contra Postgres 17, no deducido: un INSERT que viola a la vez la política de RLS y una restricción UNIQUE devuelve 425… |
| `T-01-014` | `…/an-exists-subquery-inherits-rls` | 5 | [2026-08-30-una-politica-exists-hereda-el-rls-de-la-tabla-que-lee.md](../../../.engram/queue/2026-08-30-una-politica-exists-hereda-el-rls-de-la-tabla-que-lee.md) | `obs-165de410af49853a` | La política de users (D6, la única excepción sancionada) es: |
| `T-01-015` | `…/a-reverted-guc-is-the-empty-string` | 5 | [2026-08-30-una-guc-revertida-vuelve-a-cadena-vacia-no-a-null.md](../../../.engram/queue/2026-08-30-una-guc-revertida-vuelve-a-cadena-vacia-no-a-null.md) | `obs-1a24fe956c61636a` | set_config('app.shelter_id', $1, true) es transaction-local: PostgreSQL la revierte en COMMIT y en ROLLBACK. Pero revertir no es… |
| `T-01-016` | `…/write-on-the-bridge-is-read-on-the-table` | 5 | [2026-08-30-una-membership-invitada-es-un-grant-de-lectura.md](../../../.engram/queue/2026-08-30-una-membership-invitada-es-un-grant-de-lectura.md) | `obs-217a73f022ead863` | users no tiene shelter_id, así que su visibilidad se deriva a través de memberships: |
| `T-01-017` | `…/a-single-column-fk-does-not-isolate` | 5 | [2026-08-30-las-dos-formas-compilan-y-solo-una-es-correcta.md](../../../.engram/queue/2026-08-30-las-dos-formas-compilan-y-solo-una-es-correcta.md) | `obs-a5ce66db164f9410` | shelters.logo_media_id apunta a media. La forma obvia: |
| `T-01-019` | `…/a-global-unique-is-an-existence-oracle` | 5 | [2026-08-30-la-unicidad-correcta-es-un-oraculo-de-existencia.md](../../../.engram/queue/2026-08-30-la-unicidad-correcta-es-un-oraculo-de-existencia.md) | `obs-33ffdf4eb61c7c3c` | Un número de microchip es único en el mundo. Así que lo obvio es: |
| `T-01-020` | `…/rls-truncate` | 4 | [2026-08-31-truncate-es-la-escritura-que-ninguna-politica-ve.md](../../../.engram/queue/2026-08-31-truncate-es-la-escritura-que-ninguna-politica-ve.md) | `obs-7547cb4172cbc931` | Row Level Security se evalua por fila: USING decide que filas son visibles, WITH CHECK decide que filas pueden quedar escritas. T… |
| `T-01-020` | `…/rls-exists-grant` | 4 | [2026-08-31-un-exists-de-politica-tambien-exige-el-grant.md](../../../.engram/queue/2026-08-31-un-exists-de-politica-tambien-exige-el-grant.md) | `obs-e2fdbf0b785f08df` | Ya estaba registrado que ese subquery hereda la RLS de la tabla referenciada. Faltaba la otra mitad, y la fase la venia asumiendo… |
| `T-01-021` | `…/error-oracles` | 4 | [2026-08-31-rechazar-no-alcanza-tienen-que-rechazar-igual.md](../../../.engram/queue/2026-08-31-rechazar-no-alcanza-tienen-que-rechazar-igual.md) | `obs-f653583f24128395` | El test de huerfano cruzado inserta, como tenant B, un hijo que nombra al pet de tenant A. Que sea rechazado no alcanza: si el pa… |
| `T-01-023` | `…/constraint-check-order` | 5 | [2026-08-31-el-indice-unico-se-chequea-antes-que-la-fk.md](../../../.engram/queue/2026-08-31-el-indice-unico-se-chequea-antes-que-la-fk.md) | `obs-380c510874e6712e` | Probado contra PostgreSQL 17, no supuesto. Los chequeos referenciales corren como AFTER triggers; el insert del indice unico pasa… |
| `T-01-025` | `…/truncate-cascade-bypasses-the-fk` | 4 | [2026-09-01-una-fk-nueva-cambia-quien-contesta-a-truncate.md](../../../.engram/queue/2026-09-01-una-fk-nueva-cambia-quien-contesta-a-truncate.md) | `obs-ee57985f8321d331` | TRUNCATE es la unica escritura que ninguna politica row-level puede ver, y por eso form_template_versions lleva un trigger BEFORE… |
| `T-01-028` | `…/derived-visibility-must-expire` | 5 | [2026-09-01-la-visibilidad-derivada-tiene-que-terminar-cuando-termina-su-causa.md](../../../.engram/queue/2026-09-01-la-visibilidad-derivada-tiene-que-terminar-cuando-termina-su-causa.md) | `obs-3b6ad00b3b71abae` | users es visible por dos motivos, cada uno con su policy permisiva: |
| `T-01-028` | `…/fk-cannot-reference-partial-unique` | 4 | [2026-09-01-una-fk-no-referencia-un-indice-unico-parcial.md](../../../.engram/queue/2026-09-01-una-fk-no-referencia-un-indice-unico-parcial.md) | `obs-28d2d22bf005152e` | Verificado en PostgreSQL 17, no supuesto: |
| `phase-01-domain-and-data (sdd-design, D5)` | `…/fk-checks-bypass-rls` | 5 | [2026-08-29-fk-checks-bypass-rls.md](../../../.engram/queue/2026-08-29-fk-checks-bypass-rls.md) | `obs-f97019216916c800` | Las comprobaciones de integridad referencial de PostgreSQL se saltan RLS. Siempre. |
| `phase-01-domain-and-data (sdd-propose)` | `…/neon-rls-bypass` | 5 | [2026-08-29-neon-bypassrls-defeats-policies.md](../../../.engram/queue/2026-08-29-neon-bypassrls-defeats-policies.md) | `obs-ff9448b3c1505e50` | En Neon, crear el rol de aplicación desde la consola, el CLI o la API desactiva todas las políticas RLS, en silencio. |

