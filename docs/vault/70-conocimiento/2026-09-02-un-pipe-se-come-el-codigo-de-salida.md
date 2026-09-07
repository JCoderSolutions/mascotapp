---
type: bug
score: 3
topic_key: mascotapp/convention/exit-code-through-a-pipe
task: PR-02-01 (T-02-001/002)
approved: 2026-09-02 (aprobacion explicita del usuario)
observation_id: obs-e5c386557333aa6a
rationale: "Es el camino mas corto para reportar una suite verde que esta roja, y en este proyecto el runner de tests SIEMPRE se lee por pipe."
---

`make test-api-container 2>&1 | tail -60` reporto **exit code 0** mientras la salida contenia
`make: *** [Makefile:184: test-api-container] Error 1` y un `--- FAIL`.

El codigo de salida de un pipeline es el del **ultimo** comando, y `tail` siempre sale 0. El
estado de `make` se pierde salvo que se active `set -o pipefail`, que no esta activo en las
corridas del harness.

**Regla:** en este proyecto la evidencia de que la suite paso son las **lineas de salida**, nunca
el codigo de retorno. Se buscan `--- FAIL`, `^FAIL` y `Error \d` en el texto.

Y hay un segundo filo, del mismo dia: **`ok <paquete>` tampoco prueba que un test corrio.** Go
imprime `ok` para el paquete sin importar cuales tests ejecutaron. Para afirmar que un test
especifico paso hace falta `-run '<patron>' -v` y contar los `--- PASS` por nombre.
