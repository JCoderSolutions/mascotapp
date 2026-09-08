---
type: convention
score: 4
topic_key: mascotapp/convention/an-unfalsifiable-spec-scenario
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-1e24d237d733ecb6
task: T-01-018
rationale: "Un escenario del spec implementado al pie de la letra puede ser verde permanente y leerse como cobertura."
---

# Un escenario del spec puede ser imposible de fallar

El spec pedía, para los seeds de datos de referencia:

> GIVEN el set fue aplicado, revertido y aplicado de nuevo
> WHEN se cuentan `species` y `breeds`
> THEN cada una tiene exactamente el número sembrado

**Ese escenario no puede fallar.** `down-to 0` **dropea** las dos tablas, así que
el segundo apply siembra sobre pizarra limpia sin importar cómo esté escrito el
`INSERT`. Un seed sin ninguna guarda de conflicto lo pasa igual. Y encima ya
estaba satisfecho por el round-trip, así que implementarlo agregaba una línea de
cobertura y cero información.

## Dónde está la falla de verdad

Reaplicar el seed contra una base **que ya tiene las filas**: un segundo camino
de deploy, una migración corrida a mano, un dump restaurado. Ahí el
`ON CONFLICT DO NOTHING` importa, y solo ahí.

## El test que sí muerde

Leer las sentencias entre marcadores `-- seeds:begin` / `-- seeds:end` **del
archivo de migración** y ejecutarlas una segunda vez contra el esquema vivo.

Dos propiedades que valen:

1. **Ejercita el SQL real**, no una copia en el test que se desincroniza al
   primer seed nuevo.
2. **Falla ruidoso si los marcadores desaparecen.** Un test que silenciosamente
   no encontrara nada que reejecutar sería la misma clase de mentira que el
   escenario que reemplaza.

## La regla

Un escenario del spec es una **entrada**, no una especificación de test.
Preguntá siempre: *¿qué cambio en el código hace fallar esto?* Si la respuesta es
"ninguno", implementalo igual para trazabilidad — y escribí al lado el que sí
muerde, diciendo por qué.

Misma familia que [[2026-08-30-un-suite-verde-sobre-cero-tablas]]: verde sobre un sujeto que
no está.
