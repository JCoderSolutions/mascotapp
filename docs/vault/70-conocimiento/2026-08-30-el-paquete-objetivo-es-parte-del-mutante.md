---
type: convention
score: 4
topic_key: mascotapp/convention/the-mutant-carries-its-target-package
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-003e71fb99b7d894
task: T-01-017
rationale: "Un mutante corrido contra el paquete que no lo observa se reporta como sobreviviente y manda a buscar un hueco que no existe."
---

# El paquete objetivo es parte del mutante, no una configuración del harness

Ronda 2 de mutación sobre `00003_media.sql`: borrar el `DROP CONSTRAINT` del
`Down` **sobrevivió**. Fui a buscar el hueco de cobertura.

No había hueco. El round-trip escalonado vive en `./internal/db/`, y mi harness
corría siempre `./internal/db/rlstest/`. Contra el paquete correcto el mutante
muere ruidoso, en **tres** tests:

```
2BP01: cannot drop table media because other objects depend on it
```

## Tercera vez que el instrumento es el problema

1. **T-01-013** — el harness contaba cualquier exit no-cero como kill, así que un
   bloqueo de Windows Defender (que ni corre el test) se acreditaba como mutante
   cazado. Ver [[2026-08-30-el-harness-de-mutacion-que-medía-el-host]].
2. **T-01-015** — un pool de una sola conexión compartido en un loop por tabla
   hacía que solo la primera tabla tuviera conexión virgen.
3. **T-01-017** — el paquete objetivo no observaba al mutante.

Los tres producen la misma clase de mentira: **un número que no significa lo que
parece.** Dos inflan, uno desinfla.

## La regla

Antes de correr un mutante, preguntá **qué test observa esta línea**, y apuntá
ahí. En un repo con las aserciones repartidas en varios paquetes, el objetivo es
una propiedad **del mutante**, no un ajuste global del runner.

Y el corolario que ya vale tres veces: **un sobreviviente inesperado se
investiga; no se acepta ni se explica.** Las tres veces la investigación encontró
el defecto en el instrumento, no en la red.
