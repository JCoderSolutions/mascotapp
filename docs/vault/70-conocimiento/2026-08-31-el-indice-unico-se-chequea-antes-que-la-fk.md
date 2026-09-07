---
type: constraint
score: 5
topic_key: mascotapp/security/constraint-check-order
approved: 2026-08-31 por el usuario
observation_id: obs-380c510874e6712e
task: T-01-023
rationale: "Es el hecho que decide si una clave unica 'correcta' es una fuga entre tenants. Se probo contra PG 17, y ya cambio el diseno de tres tablas."
---

# El indice unico se chequea ANTES que la foreign key

Probado contra PostgreSQL 17, no supuesto. Los chequeos referenciales corren como **AFTER
triggers**; el insert del indice unico pasa durante la escritura al heap. Entonces:

```sql
-- fila que es duplicada Y ADEMAS nombra un padre ajeno
ERROR: duplicate key value violates unique constraint   -- 23505

-- fila que solo nombra un padre ajeno
ERROR: insert or update ... violates foreign key constraint  -- 23503
```

## Por que decide un diseno

Una clave unica **global** sobre una columna que el tenant elige convierte cada INSERT en un
**oraculo de existencia**, incluso cuando hay una FK compuesta que deberia frenarlo: la FK
nunca llega a opinar. El atacante estampa **su propio** `shelter_id` para que la politica se
corra al costado, nombra el objeto del otro tenant, y lee la respuesta en el SQLSTATE.

Ya cambio tres claves en la fase 01:

- `pets.microchip_id` (T-01-019) -> `UNIQUE (shelter_id, microchip_id)`
- la foto de portada de `pet_media` (T-01-020) -> `UNIQUE (shelter_id, pet_id) WHERE is_primary`
- `form_template_versions` (T-01-023) -> `UNIQUE (shelter_id, template_id, version)`, desviando
  del literal `UNIQUE (template_id, version)` que escribe §4.4

## La regla

**Agregar `shelter_id` al frente de una clave unica no debilita nada y colapsa el oraculo.** El
objeto hijo pertenece a un unico refugio, asi que la unicidad por refugio ES la unicidad que se
queria; lo unico que cambia es lo que otro tenant puede aprender.

Corolario para los tests: la propiedad no es "fue rechazado", es **"las dos preguntas reciben
LA MISMA respuesta"**.

Relacionado: [[2026-08-31-rechazar-no-alcanza-tienen-que-rechazar-igual]],
[[2026-08-30-la-unicidad-correcta-es-un-oraculo-de-existencia]],
[[2026-08-30-rls-refusa-antes-que-el-indice-unico]].
