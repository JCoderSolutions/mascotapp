---
type: convention
score: 3
topic_key: mascotapp/convention/mutate-the-config-not-the-test
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-41e556e63b5b8722
task: T-01-012
rationale: "La mutación aplicada al archivo equivocado no encuentra nada, y da la falsa sensación de haber verificado."
---

# Para un test que lee configuración, el mutante honesto rompe la configuración

Los tests de T-01-012 no ejercitan código Go: leen `sqlc.yaml`, el `Makefile` y
`postCreate.sh`, y afirman propiedades sobre ellos.

Mutar **el test** ahí no prueba nada útil — borrar una assertion de un test hoja
siempre lo deja en verde, sin importar qué tan buena era. Es la limitación
irreducible de todo test hoja, y ya la registré como sobreviviente aceptado en
[[un-round-trip-no-ve-el-medio-del-camino]].

El sujeto real de estos tests es **el archivo de configuración**. Así que la
ronda de mutación cambia `sql_package: pgx/v5` por `database/sql`, borra el
override de `uuid`, le pone un default a `DATABASE_URL`, saca los targets de
`.PHONY`, hace que `db-reset` borre un volumen de Docker.

De 20 mutantes así, 19 murieron. Y el que sobrevivió — borrar el install de
`sqlc` del devcontainer — **encontró un hueco real**: nada exigía que el
devcontainer instalara las herramientas que el `Makefile` corre, y esa misma
tarea acababa de agregar tres targets que invocan `goose`. Un target que
funciona en la máquina donde se escribió y falla en cada contenedor nuevo.

## La regla

Antes de armar la ronda, preguntá **cuál es el sujeto**. Si el test lee un
archivo, el sujeto es ese archivo. Mutar el test en ese caso mide la disciplina
del que escribe tests, no la calidad de la red.

Corolario: la regla que salió de ahí se deriva **del Makefile**, no de una lista
escrita a mano — un target agregado después trae su propio requisito. Una lista
a mano habría sido la próxima cosa que se desincroniza.
