---
type: bug
score: 5
topic_key: mascotapp/convention/a-substring-is-not-an-assertion
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-e4c1ca6f36c7b65f
task: T-01-033
rationale: "El mismo defecto aparecio DOS VECES en una sola tarea -- en un guard heredado y en un test recien escrito -- y las dos veces el test se leia correcto. Es un patron de escritura de tests, no un descuido."
---

# Un substring encontrado en un archivo no es una asercion sobre lo que el archivo HACE

`strings.Contains(archivo, "algo")` responde *"la palabra aparece"*, no *"el archivo hace eso"*.
Y **un comentario satisface la primera**.

Paso dos veces en T-01-033, con un test de distancia:

1. **El guard heredado.** `strings.Contains(ci, "sqlc")` afirmaba que CI regenera y compara la
   salida. Un workflow que **instala** la herramienta y nunca la corre pasaba. Uno que la corre y
   nunca compara, tambien. Y hasta un `# TODO: sqlc` pasaba. Endurecido a las **dos** condiciones
   concretas: `sqlc generate` y el `git diff --exit-code` sobre el directorio generado.

2. **El test recien escrito.** *"Toda tabla declarada tiene que ser nombrada por al menos una
   query"* buscaba el nombre de la tabla en el corpus de los `.sql`. Sacarle a `pet_media` y a
   `breeds` su **unica** query dejo el test en VERDE — porque cada tabla seguia nombrada en un
   **comentario**, en la prosa que yo mismo habia escrito explicando otra cosa.

**La forma general:** cuando un test busca texto en un archivo, preguntarse *"¿que otra cosa,
ademas del comportamiento que quiero, hace aparecer este texto?"*. Las respuestas de siempre son
comentarios, mensajes de error, nombres de variables y TODOs. Si alguna aplica, hay que
**normalizar antes de buscar** (stripear comentarios) o **buscar la construccion, no la palabra**
(dos substrings especificos en vez de uno generico).

**Y el corolario:** cuando se stripea o se normaliza, ese atajo necesita su propio guard. El
stripper de `--` que resuelve el caso 2 es correcto para todas las queries de este repo y
silenciosamente incorrecto el dia que aparezca un `/* */` o un `--` dentro de un literal. Un test
que **declara ese limite** es lo que impide que una simplificacion deliberada se convierta en un
bug accidental.

Ver [[una-lista-a-mano-en-un-inventario-excluye-en-silencio]] y
[[un-guard-enumerado-tambien-puede-preguntar-de-menos]]: la misma familia de fallas, donde el test
se lee correcto y mide otra cosa.
