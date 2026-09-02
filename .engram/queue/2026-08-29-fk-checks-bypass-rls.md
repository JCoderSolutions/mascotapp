---
type: constraint
score: 5
topic_key: mascotapp/security/fk-checks-bypass-rls
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-f97019216916c800
task: phase-01-domain-and-data (sdd-design, D5)
rationale: "Invierte la conclusión obvia: denormalizar shelter_id en las tablas hijas es MENOS seguro que EXISTS si no lleva FK compuesta. Nadie lo deduce leyendo la política, y el agujero no lo atrapa ningún test de lectura."
---

**Las comprobaciones de integridad referencial de PostgreSQL se saltan RLS. Siempre.**

Verificado textualmente contra la documentación oficial de PostgreSQL 17
(`ddl-rowsecurity.html`) el 2026-08-29:

> *"Referential integrity checks, such as unique or primary key constraints and foreign
> key references, always bypass row security to ensure data integrity is maintained. Care
> must be taken when developing schemas and row-level policies to avoid covert channel
> leaks of information through these checks."*

Y `CREATE POLICY > Notes` agrega que esto permite **inferir la existencia de filas
ocultas**, por ejemplo a través de errores de clave duplicada.

## Por qué importa acá

En un esquema multi-tenant con `shelter_id` denormalizado en las tablas hijas, una FK de
una sola columna deja un agujero de escritura entre inquilinos:

1. El inquilino B inserta en `pet_media` una fila con **su propio** `shelter_id` y el
   `pet_id` **de A**.
2. La comprobación de la FK `pet_id REFERENCES pets(id)` **se saltea RLS**, así que ve el
   pet de A y **pasa**.
3. El `WITH CHECK` de la política pasa, porque el `shelter_id` de la fila es el de B.
4. B acaba de colgar una fila del animal de otro refugio. Sin error. Sin fila roja.

Una política `EXISTS`-sobre-el-padre **habría rechazado** eso. O sea: la denormalización
"directa e indexable" es, por sí sola, **peor** que la alternativa que descarta.

## La regla que se deriva

La FK **compuesta** cierra el agujero, y por eso es **obligatoria, no una optimización**:

```sql
ALTER TABLE pets ADD CONSTRAINT pets_id_shelter_key UNIQUE (id, shelter_id);

CONSTRAINT pet_media_pet_fk
  FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id) ON DELETE CASCADE
```

`(pet_id_de_A, shelter_id_de_B)` no es una fila de `pets`, así que la restricción falla.

Beneficio secundario: **cierra el oráculo de existencia entre inquilinos**. Con FK de una
columna, un id ajeno da éxito y un id inexistente da violación — esa diferencia *es* el
oráculo. Con FK compuesta, ambos casos devuelven la misma violación.

Regla general para el esquema: **denormalizar donde una fila pertenece a exactamente un
refugio; `EXISTS` solo donde pertenece a varios** (el único caso legítimo es `users`, que
pertenece a N refugios vía `memberships`).

Requisito de test que sale de esto: por cada tabla hija, como B, insertar una fila que
referencie al padre de A y **exigir violación de FK (`23503`)**. Sin esa aserción, la FK
compuesta se puede caer en un refactor y ningún test de lectura se entera.

Complementa [[ADR-0002]] y la trampa de Neon en
[[2026-08-29-neon-bypassrls-defeats-policies]]: aquella era "cómo creaste el rol", esta es
"RLS no cubre la ruta que usa el planner para validar constraints".
