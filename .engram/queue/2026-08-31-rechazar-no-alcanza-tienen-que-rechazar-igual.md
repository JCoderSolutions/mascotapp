---
type: security
score: 4
topic_key: mascotapp/security/error-oracles
approved: 2026-08-31 por el usuario
observation_id: obs-f653583f24128395
task: T-01-021
rationale: "Un test que solo pide 'que falle' deja pasar la fuga entera. Es la tercera vez en esta fase que la forma del error, y no su existencia, es la propiedad de seguridad."
---

# Rechazar no es la propiedad. Rechazar **igual** si

El test de huerfano cruzado inserta, como tenant B, un hijo que nombra al pet de tenant A. Que
sea rechazado no alcanza: si el **padre ajeno** contestara distinto del **padre inexistente**,
B enumera los pets de A **de a un id por vez**, leyendo la diferencia en el error de una
sentencia que la base ya rechazo.

La asercion compara **SQLSTATE y nombre de constraint** entre los dos casos. Solo esa
igualdad cierra el oraculo.

## Es la misma fuga tres veces en esta fase

- `pets.microchip_id` (T-01-019): `UNIQUE (microchip_id)` global es el modelado "correcto" —
  un numero de chip es unico en el mundo — y convierte cada INSERT en un oraculo. Se scopeo a
  `(shelter_id, microchip_id)`.
- La foto de portada de `pet_media` (T-01-020): `UNIQUE (pet_id) WHERE is_primary` deja
  distinguir `23505` (ese pet ya tiene portada) de `23503` (no existe). Se scopeo a
  `(shelter_id, pet_id)`.
- Aca (T-01-021): la clave compuesta hace que las dos preguntas colapsen en **una sola
  respuesta**.

## Y el corolario que decide el diseno del test

La fila lleva el `shelter_id` **de B**, no el de A. Estampar A es el otro ataque, el que la
suite A/B ya cubre y que la politica rechaza con `42501`. Estampar B **satisface la politica
para que se corra al costado** y conteste la clave foranea.

Por eso un `42501` en ese test es un **fallo**, no un pase: significa que la politica llego
primero y la clave compuesta podria borrarse manana sin que ningun test se entere. Va como
rama propia con mensaje propio, no como un `!=` generico.

Relacionado: [[2026-08-30-la-unicidad-correcta-es-un-oraculo-de-existencia]],
[[2026-08-29-fk-checks-bypass-rls]].
