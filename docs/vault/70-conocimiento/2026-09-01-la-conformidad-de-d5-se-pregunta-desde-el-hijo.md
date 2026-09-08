---
type: architecture
score: 5
topic_key: mascotapp/arch/d5-child-side-conformance
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-5b59704241bc94ab
task: T-01-025
rationale: "La clave compuesta se verificaba desde el padre y a mano desde el hijo. Un hijo entero paso sin cobertura y la mutacion lo encontro. La pregunta hay que hacerla desde el lado del hijo y por enumeracion."
---

# La conformidad de D5 se pregunta desde el HIJO, y por enumeracion

La clave compuesta de D5 tenia dos verificaciones y **ninguna de las dos cubria un hijo nuevo**:

- el inventario de claves compuestas mira a los **padres** — que `pets`, `media`,
  `form_templates` y `form_template_versions` declaren su `UNIQUE (id, shelter_id)`;
- los casos huerfanos conductuales enumeraban **a mano** los tres hijos de `pets`.

`form_submissions` llego y no lo cubrio ninguna. La mutacion lo encontro: reducir su referencia
a `REFERENCES form_template_versions (id)` — la forma de una sola columna —
**sobrevivio la suite entera**.

Y no se puede notar leyendo. Los chequeos de integridad referencial **siempre pasan por encima
de row security**: la clave de una sola columna resuelve la version de otro refugio **en nombre
de este tenant**. La submission queda atada a un formulario que su refugio no puede ver,
renderiza las preguntas de A contra las respuestas de B, y es invisible para A porque lleva el
`shelter_id` de B.

**La pregunta correcta se hace desde el hijo:** para cada tabla declarada en
`Schema.TenantChildren` que ya existe, tiene que haber una foreign key cuyo conjunto de
columnas **incluya `shelter_id`** y tenga al menos dos columnas — el `>= 2` es lo que descarta
el `shelter_id uuid REFERENCES shelters (id)` propio, que no prueba nada.

Manejada por la **declaracion**, no por una lista: los hijos que todavia no llegaron
(`adoption_applications`, `application_events`, `application_notes`, `documents`) quedan
chequeados el dia que aparezcan, sin ningun test que alguien tenga que acordarse de escribir.

**El corolario que se paga aparte:** `form_submissions` tampoco estaba **declarado** tenant
child. Un guard manejado por la declaracion no vale nada si la declaracion esta incompleta, asi
que cerrar el hueco fue arreglar las dos mitades — el test y el set, mas el conteo y la tabla
de la spec.

Ver [[2026-08-31-rechazar-no-alcanza-tienen-que-rechazar-igual]] y
[[2026-08-31-una-lista-a-mano-en-un-inventario-excluye-en-silencio]].
