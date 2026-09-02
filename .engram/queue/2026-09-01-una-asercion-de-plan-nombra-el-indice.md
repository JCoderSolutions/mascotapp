---
type: bug
score: 4
topic_key: mascotapp/convention/plan-assertions-name-the-index
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-a284e13047458786
task: T-01-025
rationale: "Bajo RLS toda query lleva el predicado de la politica, asi que 'Index Scan' aparece en casi cualquier plan. Una asercion que busca ese string pasa con cero indices utiles."
---

# Una asercion de plan nombra el indice, y se planifica como OWNER

Dos cosas, encontradas en la misma corrida y las dos por el mismo motivo: **la RLS deforma el
plan**.

**1. La asercion nombra el indice.** Bajo `app_tenant`, **toda** query lleva el predicado
`shelter_id = ...` de la politica. El planner lo satisface con el indice de shelter, asi que el
string generico `"Index Scan"` aparece en casi cualquier plan — **incluido el de la consulta
anti-vacuidad**, que es como se detecto. Una asercion contra ese string pasa **sin ningun indice
GIN presente**. Lo que se afirma es el nombre: `form_submissions_answers_idx`.

**2. Se planifica como OWNER.** Con el predicado de la politica encima, el planner satisface el
indice de shelter y aplica `answers @>` como **filtro plano**: el plan llegaba a un indice, pero
**no a este**, y el GIN nunca se consultaba por correcto que fuera. La propiedad bajo prueba es
del **INDICE**, no del camino de tenant, asi que el camino de tenant se saca del medio. Es
introspeccion, como leer el catalogo: no afirma nada sobre aislamiento.

**Y `enable_seqscan = off` no es trampa.** Es una **preferencia** del planner, no un fake:
PostgreSQL sigue negandose a usar un indice que **no puede** servir al operador de la query. Por
eso discrimina exactamente el bug realista — un btree sobre `answers`, que PostgreSQL acepta, se
lee como indice en el catalogo, y **no sirve ningun `@>`** — de un GIN, que si. Sobre una tabla
de tres filas el planner no elegiria ningun indice por costo, asi que sin la preferencia el caso
seria sobre el tamano de la tabla y no sobre el indice.

**Lo que mantiene todo esto honesto es la anti-vacuidad**: la misma preferencia **no** arrastra
al GIN a una query que no puede servir (`answers::text LIKE '%...%'`). Sin ese segundo plan, el
primero podria estar saliendo de la preferencia sola.

`SET LOCAL` deja la preferencia dentro de la transaccion, para que no se filtre a una conexion
del pool y cambie como planifica un test posterior.
