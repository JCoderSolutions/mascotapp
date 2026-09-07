---
type: convention
score: 3
topic_key: mascotapp/convention/a-loosened-rule-keeps-a-test
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-a5ffe14020786d7e
task: T-01-013
rationale: "Una regla que se afloja sin prueba se vuelve a aflojar la próxima vez, hasta que no atrapa nada."
---

# Cuando una regla de seguridad se afloja, debe quedar probada la parte que sigue atrapando

El guard de D8 ("ninguna contraseña aparece en el texto de una migración") era un
match por substring sobre `PASSWORD`. La migración `00002` introdujo
`users.password_hash` y el guard falló contra un **nombre de columna legítimo**.

La salida cómoda es ensanchar la excepción. Ensanchar una regla de seguridad
hasta que deja de molestar es exactamente cómo una regla termina borrada.

Lo que se hizo:

1. **Estrechar con precisión**, no con una lista de excepciones: `\bpassword\b`.
   `_` es carácter de palabra, así que matchea `ALTER ROLE ... PASSWORD 'x'` y
   **no** `password_hash`. Esa es justo la distinción de la que habla D8.
2. **Extraer el predicado a una función pura** y darle su propio test
   table-driven, con las cuatro formas reales en que el secreto termina en un
   archivo commiteado, y las cuatro que deben pasar.

## La regla

Aflojar un control cuesta un test que demuestre qué sigue atrapando. Sin eso, la
próxima persona que se tope con el mismo falso positivo no tiene forma de saber
qué puede tocar sin romper la garantía — y lo va a aflojar de nuevo.
