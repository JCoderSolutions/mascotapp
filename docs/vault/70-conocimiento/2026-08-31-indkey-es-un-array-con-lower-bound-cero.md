---
type: bug
score: 3
topic_key: mascotapp/convention/testing-catalog-assertions
approved: 2026-08-31 por el usuario
observation_id: obs-ce88a1825e176a88
task: T-01-020
rationale: "Una asercion sobre el catalogo del sistema que no podia dar verdadero nunca. Se lee correcta y es infalsificable."
---

# `indkey::int2[]` tiene lower bound 0, `array_agg` construye desde 1, y Postgres compara los bounds

`hasPartialUniqueOn` verifica que `pet_media` tenga
`UNIQUE (shelter_id, pet_id) WHERE is_primary`. Lee `pg_index` y no `pg_constraint`, porque
**un indice unico parcial no puede ser una constraint de tabla**. Eso estaba bien. Y aun asi
devolvia `false` contra el indice real.

`pg_index.indkey` es un `int2vector`. Casteado a `int2[]` da un array **con lower bound 0**:

```
indkey::int2[]  ->  [0:1]={3,1}
array_agg(...)  ->  {3,1}            -- lower bound 1
```

PostgreSQL compara **bounds ademas de elementos**, asi que los dos arrays no son iguales por
mas que las columnas coincidan. Se arregla re-agregando para normalizar la base:

```sql
(SELECT array_agg(k ORDER BY ord)
   FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS got(k, ord))
```

## Lo que hay que llevarse

**Una asercion sobre el catalogo del sistema se prueba en los dos sentidos antes de creerle**
— que da verdadero con el objeto correcto, y falso sin el. Esta solo se habia probado en el
sentido negativo, y por eso "funcionaba" mientras el indice todavia no existia.

Relacionado: [[2026-08-30-un-suite-verde-sobre-cero-tablas]].
