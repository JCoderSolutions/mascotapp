---
type: convention
score: 4
topic_key: mascotapp/convention/green-over-nothing-must-say-so
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-b4c7ca784082649e
task: T-01-010
rationale: "Toda fase que construye assertions antes que el esquema tiene este problema, y el modo de falla es que nadie lo note nunca."
---

# Un suite en verde sobre cero tablas miente, y hay que hacerlo decirlo

El meta-test de catálogo de T-01-010 declara 19 tablas del modelo y afirma que
cada una tiene `relrowsecurity`, `relforcerowsecurity` y al menos una política.
Ninguna de las 19 existe todavía: llegan entre T-01-013 y T-01-032.

Escrito de la forma obvia, el test pasa. Recorre las tablas existentes, no
encuentra ninguna de las declaradas, y reporta verde. Alguien que lee el suite
concluye que 19 tablas están verificadas. Cero lo están.

**Es la misma familia de fallo que la mutación encontró en T-01-007 y T-01-008:
un check correcto que no corre.** Acá el check corre — sobre el conjunto vacío.

## La regla

Cuando un assert recorre un conjunto que todavía se está llenando, el conjunto
ausente tiene que ser **declarado explícitamente**, y el test tiene que fallar
cuando la declaración deja de coincidir con la realidad — **en las dos
direcciones**:

- una entrada pendiente cuya tabla ya aterrizó → falla, hay que borrar la línea,
  y borrarla es lo que **enciende** los asserts de esa tabla;
- una tabla declarada que no está y que nadie listó como pendiente → falla.

Y el test **reporta la cobertura en voz alta**: `verified 0 of 19 declared model
tables; 19 still pending`.

## Por qué el ledger y no solo "afirmá sobre lo que existe"

"Afirmá sobre lo que existe" ya atrapa la tabla que aterriza sin protección —
esa parte está cubierta. Lo que el ledger agrega es **impedir que la vacuidad se
lea como prueba**, y forzar que cada tarea de migración toque el archivo del
meta-test. El costo es una línea por tabla. El beneficio es que nadie confunde
"no encontré nada mal" con "verifiqué que todo está bien".

Ver [[testear-un-check-no-es-testear-que-corre]] y
[[una-tabla-rota-no-alcanza]].
