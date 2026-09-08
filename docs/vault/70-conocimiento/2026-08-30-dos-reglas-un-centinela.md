---
type: bug
score: 4
topic_key: mascotapp/convention/one-sentinel-per-rule
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-589b052a73fbade8
task: T-01-011
rationale: "Es la tercera vez en la Fase 01 que el mismo error aparece con otra cara, y la cara nueva es la más difícil de ver."
---

# Dos reglas que devuelven el mismo centinela: `errors.Is` no dice cuál disparó

En T-01-011 sobrevivió el mutante que desactivaba la regla de exhaustividad de
`checkCatalogAt`. El test que la cubría seguía en verde.

La causa: **`ClassifyAll` y `CheckProtection` devuelven ambas
`ErrUnclassifiedRelation`** ante una relación sin clasificar. Con la primera
desactivada, la segunda atrapaba la misma tabla, devolvía el mismo centinela, y
el `errors.Is` del test pasaba. La regla estaba muerta y el test la declaraba
viva.

## Por qué esta variante es la peor de las tres

Es la tercera aparición del mismo error en esta fase:

1. **T-01-009**: una tabla rota dispara varias verificaciones a la vez → el test
   solo pedía `err != nil`. Ver [[2026-08-30-una-tabla-rota-no-alcanza]].
2. **T-01-010**: dos ramas del mismo `Validate` producían errores distintos → el
   test solo pedía `err != nil`.
3. **T-01-011**: dos reglas distintas producen el **mismo centinela** → el test
   pedía `errors.Is` correctamente **y aun así no distinguía**.

Las dos primeras se arreglan endureciendo el test de `err != nil` a
`errors.Is`. La tercera **sobrevive a ese endurecimiento**, y por eso es la que
hay que tener presente: usar centinelas tipados se siente como rigor suficiente,
y no lo es cuando dos caminos comparten uno.

## La regla

Cuando dos reglas distintas pueden devolver el mismo centinela, el test tiene que
afirmar **un fragmento del mensaje** que solo una de ellas produce. El centinela
dice *qué clase de problema*; el mensaje dice *quién lo encontró*, y eso último
es lo que la mutación pone a prueba.

Corolario práctico: si al escribir la assertion no se te ocurre un fragmento que
distinga, probablemente las dos reglas deberían tener centinelas distintos.
