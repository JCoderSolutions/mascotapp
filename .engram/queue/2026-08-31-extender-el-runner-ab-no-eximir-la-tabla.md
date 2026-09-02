---
type: convention
score: 3
topic_key: mascotapp/convention/testing-conformance-flags
approved: 2026-08-31 por el usuario
observation_id: obs-64c7b74b30e1800a
task: T-01-020
rationale: "La salida facil -- saltear las sondas de escritura en una tabla append-only -- convierte la regla de completitud en un agujero que crece con cada tabla protegida."
---

# Una tabla mas estricta se cubre con sondas mas estrictas, nunca con una exencion

El runner A/B afirma, tabla por tabla, que el tenant B no lee ni escribe filas de A. En una
tabla append-only las sondas de `UPDATE`/`DELETE` no aplican tal cual: no llegan a cero
filas, **son rechazadas de entrada**.

La salida rapida era saltearlas. Es la mala: una tabla a la que en algun refactor le
restauran los grants en silencio **pasaria por no ser mirada**.

`AppendOnly bool` cambia la sonda por una **mas fuerte**:

- normal: "la sentencia se ejecuto y alcanzo **cero filas**"
- append-only: "la sentencia fue **rechazada**, con `42501`"

Lo mismo con el borrado sin `WHERE`: en vez de comparar conteos, exige el rechazo.

## La regla

**Un flag en un runner de conformidad solo puede endurecer, nunca eximir.** Si el flag
hiciera skip, la regla "toda tabla con `shelter_id` pasa por el runner" seguiria escrita y
ya no significaria nada. Generaliza a `application_events` (T-01-029) y `audit_log`
(T-01-032), que llegan con la misma forma.

Relacionado: [[2026-08-30-delete-sin-where-esquiva-la-politica-select]],
[[2026-08-30-una-tabla-rota-no-alcanza]].
