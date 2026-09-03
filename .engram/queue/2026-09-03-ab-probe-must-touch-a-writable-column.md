---
type: architecture
score: 5
topic_key: mascotapp/arch/isolation-probe-must-use-a-writable-column
task: T-02-006
rationale: "Un harness de aislamiento que se pone verde por el motivo equivocado es peor que no tenerlo: reporta PASS sobre una propiedad que dejó de medir. Este falló en la dirección silenciosa —el rechazo por privilegio se ve idéntico a un rechazo por policy desde afuera— y le pasó a las dos tablas centrales de tenancy, cuyo aislamiento el plan marca como bloqueante. El patrón es general: cualquier sonda negativa tiene que garantizar que la capa que quiere medir sea la que efectivamente responde."
---

# Una sonda de aislamiento tiene que escribir una columna que el rol PUEDA escribir

El runner A/B de este proyecto prueba que el tenant B no alcanza la fila del tenant A con un
no-op:

```sql
UPDATE <tabla> SET <columna_de_tenant> = <columna_de_tenant> WHERE <clave de A>
```

La columna de tenant se eligió porque una auto-asignación es un no-op garantizado. Funcionó
durante toda la Fase 01 — y **solo funciona mientras el rol tenga el privilegio sobre esa
columna**.

`00015_column_grants` (B1) pasó `shelters` y `memberships` de grants de tabla entera a grants
por columna. `shelters.id` y `memberships.shelter_id` quedaron como INSERT-only, que es lo
correcto: un `shelters.id` actualizable **re-tenantea un refugio** y un
`memberships.shelter_id` actualizable mueve a un miembro al refugio de otro.

La sonda empezó a devolver `42501`.

## Por qué eso es un fallo silencioso y no un fallo ruidoso

Desde afuera, un rechazo por privilegio y un rechazo por policy se ven igual: el statement no
tocó la fila. Pero **la policy nunca se consultó**. El aislamiento dejó de estar probado, en
las dos tablas centrales de tenancy, y la "solución" que primero se le ocurre a cualquiera es
declararlas como tablas de solo-rechazo — con lo cual la sonda deja de ejercitar la policy
para siempre y el test pasa por no mirar.

La otra "solución" obvia, otorgar UPDATE sobre esas columnas para que la sonda vuelva a
correr, destruye la propiedad de seguridad que la migración acababa de instalar.

## El arreglo: mover la sonda, no el grant

`TenantTable.TouchColumn` — la columna que la sonda se auto-asigna, con la de tenant como
default. `display_name` para `shelters`, `status` para `memberships`: columnas que el tenant
tiene permitido escribir, así que **el grant se hace a un lado y la policy responde**.

Queda **estrictamente más fuerte que antes**: la sonda ya no puede satisfacerse con un
rechazo de privilegio que no dice nada sobre aislamiento.

`TenantTable.NoDeleteGrant` es el complemento: la mitad angosta de `AppendOnly`, para una
tabla actualizable y no borrable — un estado que el flag existente no podía describir, porque
mezclaba "sin UPDATE" y "sin DELETE" en un solo booleano.

## La regla transferible

**Una sonda negativa tiene que garantizar que la capa que pretende medir sea la que responde.**
Si otra capa puede rechazar primero, la sonda mide esa otra capa y reporta el resultado como
si fuera el de la suya.

Dos instancias de la misma familia aparecieron en la misma tarea:

- El caso de `DELETE FROM shelters` volvía `23503`: la **foreign key** desde `memberships`
  dispara antes que el privilegio. Habría sido rojo antes de la migración y verde después sin
  haber testeado el grant ni una vez. Se arregló actuando sobre un refugio sin membresías.
- `TestAnAssignedMembership_CannotBeDeleted` medía el grant en vez de la constraint: su sonda
  corría como `app_tenant`, que ya no llega a un DELETE, así que `ON DELETE RESTRICT` nunca
  se consultaba. Se movió al rol que sí tiene el privilegio, y el hecho nuevo y más fuerte
  —el tenant rechazado de entrada— se asevera aparte.

Al agregar o cambiar un grant, preguntar: **¿qué sondas existentes dependían de ese privilegio
para llegar a la capa que están midiendo?** Ver también
[[has-table-privilege-no-ve-los-grants-por-columna]] y
[[policy-lands-at-version-aware-exemption]], que son la misma familia: una declaración con más
de un consumidor, y un consumidor que dejó de medir lo que creía.
