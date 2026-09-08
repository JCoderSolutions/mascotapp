---
type: convention
score: 3
topic_key: mascotapp/convention/a-survivor-may-be-dead-code
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-94029dcc990eb3b4
task: T-01-010
rationale: "Un mutante que sobrevive no siempre significa 'falta un test'; a veces significa 'sobra código', y confundirlos agrega tests que no prueban nada."
---

# Un mutante que sobrevive puede significar código muerto, no un test faltante

En T-01-010 sobrevivió el mutante que borraba el helper `duplicates()` de
`Validate`. El reflejo es escribir un test que lo cubra.

Era la conclusión equivocada. El mutante sobrevivió porque el chequeo
**anterior** — el mapa `seen` que detecta una tabla clasificada en dos sets — ya
atrapaba el duplicado dentro de un mismo set. `duplicates()` era inalcanzable
para los tres sets que ese mapa recorre. **Código muerto disfrazado de
verificación.**

Escribir un test para cubrirlo habría producido un test que pasa por el camino
equivocado, y habría dejado el código muerto ahí, ahora con la bendición de una
línea verde.

## La regla

Ante un mutante que sobrevive, la primera pregunta no es *"¿qué test falta?"*
sino **"¿por qué nada se rompió?"**. Las tres respuestas posibles:

1. **Falta un test** → escribilo.
2. **El código es inalcanzable** → borralo, no lo cubras.
3. **El código es alcanzable pero el efecto es correcto** → dejalo y documentá
   por qué, como pasó con la verificación de UPDATE en
   [[2026-08-30-delete-sin-where-esquiva-la-politica-select]].

La corrección acá fue colapsar los dos chequeos en uno, con un mensaje que
distingue las dos formas ("listado dos veces en Tenant" vs "declarado en Tenant
y NonTenantModel") en vez del ilegible "declarado en Tenant y Tenant".
`duplicates()` quedó cubriendo solo `TenantChildren`, que el mapa `seen` no
visita — ahí sí es alcanzable, y ahí sí lleva su caso.

Ver [[2026-08-30-testear-un-check-no-es-testear-que-corre]].
