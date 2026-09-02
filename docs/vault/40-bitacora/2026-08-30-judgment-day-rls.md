---
fecha: 2026-08-30
fase: "01"
agente: claude-opus-5
tarea: T-01-016
tipo: judgment-day
target_identity: c5d3cf1ba42dccd8cf76075c5dd2ad59e5d92698fd0644d782e82d04c76e7604
fix_delta: a217d24b74344b504fc914b1162cce85ad453721329ca7e18b0ef062c8684ed2
---

# Judgment Day — la plantilla RLS de la Fase 01

Revisión ciega dual antes de mergear las políticas RLS. Es uno de los **tres**
merges de todo el proyecto que la tienen, y está acá y no más adelante porque las
cuatro slices siguientes replican esta plantilla **verbatim**: un defecto acá se
multiplica por doce migraciones.

## Target congelado

`c5d3cf1b…`, 1199 líneas en 6 archivos:

| archivo | líneas | qué aporta al target |
|---|---|---|
| `migrations/00001_extensions_roles_and_grants.sql` | 88 | extensión, roles de aplicación, grants de esquema |
| `migrations/00002_tenancy_identity.sql` | 284 | **la plantilla de política** + las 4 primeras tablas |
| `db/tenant.go` | 153 | `WithTenant`, `WithPublic` |
| `db/dbtest/roles.go` | 122 | el guard de rol (`rolsuper` / `rolbypassrls` / owner) |
| `db/rlstest/catalog.go` | 416 | el meta-test del catálogo |
| `db/bootstrap.go` | 136 | bootstrap de contraseñas fuera del set de migraciones |

Dos jueces ciegos en paralelo, mismo target, mismos criterios. **Ninguno reportó
un CRITICAL, y no coincidieron en ningún hallazgo.**

## Los cuatro hallazgos, y qué pasó con cada uno

El contrato dice: se arregla solo lo que confirman **los dos** jueces. Acá no
coincidió nada — pero verifiqué los cuatro contra la base real, y eso cambió la
naturaleza de dos.

### A1 · CONFIRMADO y ARREGLADO — un refugio se auto-otorgaba lectura de PII

El juez lo reportó como WARNING inferencial: `member_visible_users` no filtraba
por `memberships.status`, así que una membership `invited` o `revoked` seguía
haciendo visible al usuario.

La sonda contra PostgreSQL 17 mostró que era peor:

```
sin membership:                     stranger visible = false
tras insertar una INVITED:          stranger visible = true
tras REVOCARLA:                     stranger visible = true
```

**El `INSERT` lo hace `app_tenant`**, que tiene INSERT y UPDATE sobre
`memberships`. O sea: cualquier refugio se acuña lectura del perfil de cualquier
usuario cuyo id conozca — mail, teléfono, nombre — y **revocar no se la quita**.

No es una fuga entre tenants: es un **grant de lectura self-service sobre PII**.

El spec del propio proyecto ya decía la palabra que faltaba: *"user U has an
**active** membership in shelter A"*. El arreglo es un predicado.

```sql
AND m.status = 'active'
```

Reverificado con el mismo ataque: `invited` → invisible, `active` → visible,
`revoked` → invisible.

### B1 · CONFIRMADO y DIFERIDO — escalada de privilegios dentro del tenant

Los grants son a nivel tabla, así que con RLS filtrando solo por `shelter_id`,
`app_tenant` puede, sobre **su propia** fila:

| probado | consecuencia |
|---|---|
| `UPDATE shelters SET status = 'verified'` | **bypass de LT-2** — el refugio se auto-verifica |
| `UPDATE shelters SET storage_quota_bytes = …` | se sube la cuota solo |
| `UPDATE memberships SET role = 'owner'` | auto-promoción a dueño |

Los tres confirmados en vivo. **Diferido por decisión explícita del usuario**: el
arreglo son grants a nivel columna, y qué columnas puede escribir un tenant
depende de endpoints que la Fase 03 todavía no escribió. Congelar eso hoy sería
decidir a ciegas.

→ **Fase 02 (RBAC) y Fase 03 (CRUD de refugio)**, con la prueba ejecutable
registrada.

### B2 · CONFIRMADO y DIFERIDO — el meta-test no ve para quién es una política

Determinista, y es el mismo hueco que la mutación de T-01-013 encontró:
`CheckProtection` cuenta filas de `pg_policy` sin mirar `polroles` ni si el
`USING` es tautológico. Una migración futura que ate una política al rol
equivocado, o escriba `USING (true)`, **pasa como "protegida"**.

T-01-013 lo cerró para las cuatro tablas de `00002` con
`TestTenancyPolicies_ApplyToTheRightRoleAndCommand`, que es angosto a propósito.
El hueco estructural sigue abierto para `00003`…`00012`.

→ **T-01-017**, la primera migración que lo va a atravesar.

### A2 · REFUTADO — el guard de rol sí está cableado

El juez sospechó que `guardRole` no estaba conectado al harness real, y dijo
explícitamente que no podía confirmarlo desde su slice. Correcto, y la evidencia
de afuera lo refuta: se llama en `newEnv` (`dbtest/container.go:308`) y
`roles_test.go:169` lo observa vía `Env.GuardedRoles()`.

## Lo que esta revisión enseñó sobre el propio método

**Un juez ciego con alcance acotado produce hallazgos "inferenciales" que el
orquestador puede convertir en hechos, y esa conversión es el trabajo.** Los dos
jueces marcaron A1 y B1 como WARNING inferencial *porque no podían ejecutar
nada*. Una sonda de treinta líneas contra la base los convirtió en confirmados —
y a A1 lo mostró más grave de lo que su propio reporte decía.

Corolario incómodo: **si me hubiera quedado con la regla mecánica del contrato**
— "solo se arregla lo que confirman los dos jueces" — **el hueco de PII se
mergeaba**. La regla existe para no actuar sobre especulación de un solo juez. La
prueba ejecutable es más fuerte que un segundo juez, y satisface el espíritu de
la regla aunque no su letra.

## Desviación del diseño que este gate tenía que revisar

T-01-015 introdujo `nullif(current_setting('app.shelter_id', true), '')::uuid` en
las cinco expresiones de política. **El design escribe el `current_setting`
pelado**, y cada migración posterior copia esa plantilla. Ninguno de los dos
jueces lo objetó; los dos lo verificaron como correcto y consistente.

→ La nota del design queda **desactualizada** para `00003` en adelante. Hay que
corregirla ahí, no en un changelog.

## Veredicto

- **confirmados por los dos jueces:** ninguno
- **suspects elevados a confirmados por prueba del orquestador:** A1 (arreglado),
  B1 (diferido), B2 (diferido)
- **contradicciones:** ninguna
- **refutados:** A2
- **rondas de fix:** 1 de 2 (queda una sin usar)
- **re-judgment acotada:** los dos jueces, **limpios**, cero hallazgos causados por
  el fix

La re-judgment verificó lo que hacía falta verificar y no solo que el suite
seguía verde: que `AND m.status = 'active'` cierra **las dos mitades** del hueco
(acuñar con `invited`, y que revocar no revocaba); que los cuatro call sites de
`seedMembership` siguen recibiendo `'active'` tras el split del helper; que los
dos casos nuevos **no son vacuos** — las dos memberships apuntan a `ShelterA`, así
que sacar el predicado los daría vuelta a `visible = true` y el test fallaría; que
`invited` y `revoked` están dentro del `CHECK` de `memberships.status`; y que
ningún test de snapshot fija el texto SQL de la migración.

Verificación final independiente: `go vet` limpio · `golangci-lint` **0 issues** ·
`govulncheck` 0 vulnerabilidades llamadas · `make test-api` verde.

```
JUDGMENT: APPROVED
```

`skill_resolution: mixto` — `jd-fix-a1` y `jd-b-round2` recibieron las rutas
inyectadas; `jd-a-round2` reportó `fallback-path` y no invocó la herramienta de
skills, revisando por inspección directa. Se registra como pasó, no como debía
pasar.

## Lo que este veredicto NO es

Un `APPROVED` de Judgment Day **no emite receipt y no autoriza entrega**: no
satisface ninguna puerta de commit, push, PR ni release. Es una revisión
adversarial, y nada más.

## Candidatos a memoria

- `.engram/queue/2026-08-30-un-juez-ciego-no-puede-ejecutar.md` — score 4
- `.engram/queue/2026-08-30-una-membership-invitada-es-un-grant-de-lectura.md` — score 5
