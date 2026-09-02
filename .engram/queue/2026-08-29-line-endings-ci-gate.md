---
type: constraint
score: 3
topic_key: mascotapp/ops/line-endings
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-7c3386a868ab728a
task: T-00-022
rationale: "Un CI que falla por bytes idénticos con contenido idéntico hace perder horas si no se sabe de antemano de dónde viene."
---

El repo se desarrolla en Windows con `core.autocrlf=true` y CI corre en Linux.
Sin `.gitattributes` eso hace que los finales de línea dependan de la máquina de
cada quien, y **dos puertas de CI comparan bytes**:

- `gofmt -l .` en el job `api`
- las verificaciones de drift de código generado (`oapi-codegen` y
  `openapi-typescript`, que regeneran y hacen `git diff --exit-code`)

El síntoma es desconcertante: `gofmt -d` muestra el archivo entero como
modificado, con las líneas `-` y `+` idénticas a simple vista. Eso es la firma
de una diferencia CRLF/LF, no de un problema de formato.

Se fijó con `.gitattributes` en la raíz: `* text=auto eol=lf`, más entradas
`binary` explícitas para imágenes, PDF y fuentes, para que nada de eso se
transforme.
