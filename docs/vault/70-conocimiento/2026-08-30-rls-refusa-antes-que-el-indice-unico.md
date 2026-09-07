---
type: constraint
score: 4
topic_key: mascotapp/security/with-check-precedes-the-unique-index
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-fab580eb9736dc4c
task: T-01-013
rationale: "Si el orden fuera el otro, un INSERT rechazado sería un oráculo de existencia sobre datos de otro tenant."
---

# PostgreSQL evalúa el `WITH CHECK` de RLS **antes** que el índice único

Verificado contra Postgres 17, no deducido: un `INSERT` que viola a la vez la
política de RLS y una restricción `UNIQUE` devuelve **42501**, no 23505.

## Por qué importa dos veces

**Como propiedad de seguridad.** Si el índice único ganara, el tenant B
aprendería que existe una fila con la clave del tenant A — un oráculo de
existencia sobre datos ajenos, entregado por la sentencia misma que la política
rechazó.

**Como propiedad del harness.** El suite A/B prueba que B no puede forjar una
fila estampada con el `shelter_id` de A. Si esa fila colisiona además en una
clave única y el orden fuera el inverso, el caso seguiría en verde **con la
política borrada**, porque algo igual dijo que no.

## La trampa de la que casi me caigo

El test que afirma este orden es vacuo si la restricción única desaparece: sin
`UNIQUE (user_id, shelter_id)`, el insert forjado igual da 42501 y el test pasa
sin probar nada.

Por eso el test primero **prueba que la colisión es real** — un duplicado desde
el scope del propio tenant A, donde la política no puede ser lo que rechaza — y
recién después afirma el orden. Misma forma que [[un-suite-verde-sobre-cero-tablas]]:
antes de afirmar algo sobre un sujeto, probá que el sujeto está ahí.
