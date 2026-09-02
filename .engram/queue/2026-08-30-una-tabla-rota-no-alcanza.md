---
type: convention
score: 4
topic_key: mascotapp/convention/one-broken-fixture-per-check
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-d5109c9408820758
task: T-01-009 (runner A/B)
rationale: "Un test negativo con un solo fixture roto da la sensación de estar cubierto y no lo está. Es el error más fácil de cometer al escribir tests de un verificador."
---

**Un fixture roto prueba que ALGUNA verificación funciona, no que cada una funciona.**

El runner A/B de la Fase 01 hace cuatro comprobaciones sobre cada tabla. Lo probé como parece
correcto: una tabla bien aislada (debe pasar) y una tabla rota (debe fallar).

Verde. Y la mutación mostró que **tres de las cuatro verificaciones se podían borrar sin que
ningún test fallara.**

## Por qué

La tabla rota usaba `USING (true)`, que dispara **varias** comprobaciones a la vez. Y el test
solo pedía `err != nil`. Entonces:

- borrás la comprobación de lectura → la del INSERT forjado igual falla → verde
- borrás la del INSERT forjado → la de lectura igual falla → verde

El test no podía distinguir **cuál** había atrapado el problema, así que no protegía a ninguna
en particular.

## El arreglo

Una tabla rota **por cada verificación**, rota exactamente de la forma que esa verificación
existe para atrapar, y el test exige que el mensaje de error **nombre la comprobación que
disparó**:

| tabla | rota en | el error debe mencionar |
|---|---|---|
| `permissive_policy` | `USING (true)` | `can READ` |
| `deletable_but_not_readable` | DELETE permisivo detrás de un SELECT correcto | `unqualified DELETE` |
| `no_with_check` | `WITH CHECK (true)` | `stamped with tenant A's shelter_id` |
| `hidden_from_its_own_owner` | `SELECT USING (false)` | `can no longer see its own row` |

Ninguna es un hombre de paja. Las cuatro son políticas que alguien escribe camino a la
respuesta correcta.

## La regla

Al testear un verificador con N comprobaciones, hacen falta **N fixtures rotos**, uno por
comprobación, y cada aserción tiene que exigir **qué** falló, no solo **que** falló.

Corolario que salió del mismo ejercicio: si una comprobación no se puede romper de forma
aislada, probablemente **no puede disparar nunca** — y eso es un hallazgo, no un inconveniente.
Así apareció que un `DELETE ... WHERE` no puede detectar una política DELETE rota
([[2026-08-30-delete-sin-where-esquiva-la-politica-select]]).

Tercera aparición del mismo patrón en la fase, junto con
[[2026-08-29-mutation-testing-vacuous-coverage]] y
[[2026-08-30-testear-un-check-no-es-testear-que-corre]]. **Las tres estaban en verde.**
