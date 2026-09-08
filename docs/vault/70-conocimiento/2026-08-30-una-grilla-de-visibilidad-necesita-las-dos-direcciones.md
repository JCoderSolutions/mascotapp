---
type: convention
score: 3
topic_key: mascotapp/convention/a-visibility-grid-needs-both-directions
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-07442a03c4820e61
task: T-01-014
rationale: "Los dos escenarios del spec los pasa una política que esconde todo y también una que muestra todo."
---

# Una aserción de visibilidad necesita las dos direcciones **y** los dos tenants

El spec pedía dos escenarios: un miembro del refugio actual es visible; un
usuario sin relación con el refugio actual es invisible.

Escritos así, contra un solo tenant, **una política rota los pasa igual**:

- `USING (false)` esconde todo → pasa "V es invisible para B".
- `USING (true)` muestra todo → pasa "U es visible para A".

Cada escenario, solo, es mitad de una aserción.

Lo implementado es una grilla de 2 tenants × 3 usuarios. Con eso, los dos
mutantes mueren.

## El tercer usuario

El que el spec **no** nombra y el producto más necesita: un adoptante **sin
membership en ningún lado**. Si `users` fuera legible por cualquier tenant
autenticado, la fila de cada adoptante — nombre, teléfono, mail — sería visible
para todos los refugios de la plataforma.

Un spec enumera escenarios; el caso peligroso suele ser el que no enumera. Ver
[[2026-08-30-una-tabla-rota-no-alcanza]]: misma forma, un nivel más arriba.
