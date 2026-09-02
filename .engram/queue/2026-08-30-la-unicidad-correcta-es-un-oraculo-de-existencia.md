---
type: architecture
score: 5
topic_key: mascotapp/security/a-global-unique-is-an-existence-oracle
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-33ffdf4eb61c7c3c
task: T-01-019
rationale: "El modelado 'correcto' de una clave natural global filtra la existencia de filas de otros tenants, y ninguna política lo impide."
---

# Una restricción `UNIQUE` global es un oráculo de existencia entre tenants

Un número de microchip **es** único en el mundo. Así que lo obvio es:

```sql
microchip_id text UNIQUE
```

Eso es el modelado correcto del dominio y **una fuga entre tenants**.

La unicidad se chequea **antes que cualquier política** — lo mismo que ya se
probó para el insert forjado en [[rls-refusa-antes-que-el-indice-unico]]. Así que
con una clave global, cada `INSERT` se vuelve un oráculo:

> El refugio B escribe un número de chip, recibe **23505**, y acaba de aprender
> que **algún otro refugio tiene ese animal**.

Sin leer una fila. Sin permiso sobre nada. Con la política funcionando
perfectamente.

## La forma correcta

```sql
UNIQUE (shelter_id, microchip_id)
```

La constraint sigue sirviendo — un refugio no puede registrar el mismo chip dos
veces — y el oráculo se cierra. La detección de duplicados **entre** refugios es
trabajo de una vista de moderación bajo el rol owner, no de una constraint que
cualquier tenant puede sondear.

## La regla general

En un esquema multi-tenant, **toda clave única global sobre un valor que el
tenant puede elegir es un canal de información.** Antes de escribir `UNIQUE`,
preguntá: *¿el valor lo controla el tenant, y le importa si otro lo tiene?* Si
las dos son sí, scopeala.

Casos a mirar cuando lleguen: emails de adoptante, `slug` de refugio (ya es
global y es deliberado — el slug es público por diseño), y cualquier
identificador externo tipo folio o número de expediente.

## Y hay que afirmarlo

La mutación mostró que mover esta clave de per-shelter a global **no rompía
nada**: la propiedad de seguridad estaba argumentada en el comentario de la
migración y en ningún test. Un comentario no es un control.
