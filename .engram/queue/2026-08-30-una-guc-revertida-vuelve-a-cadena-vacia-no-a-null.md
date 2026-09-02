---
type: bug
score: 5
topic_key: mascotapp/security/a-reverted-guc-is-the-empty-string
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-1a24fe956c61636a
task: T-01-015
rationale: "El fail-closed se cumplía o no según si esa conexión del pool había servido un request antes; ningún test de una sola conexión lo ve."
---

# Una GUC revertida vuelve a **cadena vacía**, no a NULL

`set_config('app.shelter_id', $1, true)` es transaction-local: PostgreSQL la
revierte en `COMMIT` y en `ROLLBACK`. Pero **revertir no es desetear**.

- GUC que **nunca** se seteó → `current_setting('app.shelter_id', true)` da **NULL**.
- GUC seteada y revertida → da **cadena vacía**.

Con la plantilla de política escrita como `shelter_id = current_setting(...)::uuid`,
eso significa:

| conexión | expresión | resultado |
|---|---|---|
| virgen | `shelter_id = NULL` | 0 filas ✅ fail-closed |
| reusada | `shelter_id = ''::uuid` | **22P02** ❌ |

Y después del primer request, **toda conexión de un pool es una conexión
reusada.**

## Por qué importa aunque no filtre

No expuso nada: el fallo es ruidoso, no filtrante. Lo roto es peor de encontrar:
el contrato que el spec escribe — *"MUST see zero rows"* — se cumplía **según si
esa conexión había servido un request antes**. Un contrato con esa forma no es un
contrato, es una moneda.

## El arreglo

```sql
shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid
```

**Las dos mitades son load-bearing**: `missing_ok` cubre el caso nunca-seteado,
`nullif` cubre el revertido-a-vacío. Mutantes distintos matan cada mitad.

## Lo que lo encontró

Un test que **separa los dos estados de conexión** y usa un pool con
`MaxConns = 1`, para que "la conexión que ya llevó un scope" sea una certeza y no
una coincidencia de scheduling. Con el pool por defecto esto pasa de casualidad
la mayoría de las veces, que es peor que no tenerlo.

Ver [[el-scope-no-sobrevive-a-su-transaccion]].
