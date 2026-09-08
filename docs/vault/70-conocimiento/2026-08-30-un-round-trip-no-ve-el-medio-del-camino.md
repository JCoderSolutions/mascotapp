---
type: convention
score: 4
topic_key: mascotapp/convention/a-round-trip-misses-the-middle
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-84bfb6c00194e6d2
task: T-01-011
rationale: "Toda suite de migraciones se escribe como up/down/up, y esa forma es exactamente la que no ve el estado intermedio."
---

# Un round-trip prueba los extremos, no el camino

`goose up → down-to 0 → up` es la forma canónica de probar migraciones reversibles.
Y con dos migraciones ya es insuficiente.

El test aplica todo, deshace todo, reaplica todo, y compara el final con el
principio. Un `Down` en el medio que dropea una política y deja su tabla, o que
dropea un padre y huerfaniza un hijo, **no aparece**: el `down-to 0` siguiente
borra la evidencia y el `up` la reconstruye.

Es la misma familia que [[2026-08-30-un-round-trip-no-ve-el-medio-del-camino]] de T-01-007,
un nivel más arriba: ahí el ciclo tapaba lo que un `Down` destruía; acá tapa lo
que un `Down` **deja mal**.

## La regla

Bajar **una migración por vez**, y correr los invariantes en cada parada. Las
versiones intermedias son las que un rollback de producción realmente ocupa —
nadie baja doce migraciones de un salto.

El recorrido tiene que estar manejado por el **set embebido**, no por una lista
escrita a mano. Si hay que acordarse de extender el test cada vez que se agrega
una migración, no se va a extender.

## Qué invariante va en cada parada, y cuál no

- **Sí**: lo que ningún `Down` puede destruir (roles, extensiones) y lo que toda
  tabla que *sigue existiendo* debe cumplir (clasificada, con RLS y política).
- **No**: cualquier regla que asuma el esquema completo. A mitad de la cadena una
  tabla declarada está ausente **por una razón legítima**, y afirmarla ahí falla
  por estar bien. En T-01-011 eso fue `CheckPending`.

Distinguir esas dos categorías es la única parte difícil del diseño.
