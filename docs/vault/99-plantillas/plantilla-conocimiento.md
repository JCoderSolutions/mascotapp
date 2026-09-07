# Plantilla — una pieza de conocimiento en dos capas

Un conocimiento del proyecto vive en **dos lugares con trabajos distintos**.
Duplicarlo entero en los dos es el error que esta plantilla existe para evitar.

| Capa | Qué guarda | Para qué | Tamaño |
|---|---|---|---|
| **Engram** | la regla, la trampa, dónde más aplica | encontrar y **actuar** | ~20–25 líneas |
| **Vault** (`70-conocimiento/`) | el caso, el código, la mutación, las alternativas | entender y **auditar** | lo que haga falta |

## La prueba para saber si la observación quedó corta

> **Si un agente lee solo la observación y actúa, ¿actúa bien?**

Si necesita abrir el documento del vault para no equivocarse, la observación
está incompleta. No moviste el segundo salto: lo encareciste. Leer una
observación es una llamada dentro del mismo sistema; abrir un documento exige
conocer la ruta, que exista y que siga vigente.

## La prueba inversa: qué NO va a Engram

Se va al vault todo lo que sea **narración del descubrimiento**:

- el código equivocado que motivó la regla
- cómo se encontró (la corrida de mutación, el test que lo agarró)
- las alternativas que se descartaron y por qué
- el relato de la sesión

Se queda en Engram **la regla y su trampa**. Si al borrar una línea la regla
sigue siendo aplicable sin ambigüedad, esa línea era narración.

## Cuidado al podar: lo que borrás, no lo podés encontrar

La búsqueda corre sobre el contenido. La sección *"dónde más aplica"* es lo que
hace que una observación **aparezca** cuando alguien trabaje en otra cosa tres
fases más tarde. Sacarla por prolijidad deja la memoria guardada e inalcanzable
— que es lo mismo que no tenerla, con el agravante de que creés que la tenés.

---

## A) La observación en Engram

`title` es **obligatorio**. Sin título, `mem_search` devuelve un volcado de
contenido y la observación es prácticamente imposible de reconocer en una lista
de resultados. Además, una observación sin título bloquea la replicación.

```
title:     <la regla, en una línea, buscable>
type:      architecture | convention | constraint | domain | bugfix | discovery
topic_key: mascotapp/<área>/<slug>
```

Cuerpo:

```markdown
**Qué**: la regla, en una o dos frases. Afirmativa, no narrativa.

**Por qué**: qué se rompe si no se respeta. Concreto, con el mecanismo.

**La trampa**: la versión incorrecta que suena bien. Esto es lo que hace que la
regla valga: si el error fuera obvio, no haría falta guardarlo.

**Dónde más aplica**: los otros lugares del proyecto donde muerde. Esta sección
es la que hace que la observación se encuentre desde una tarea distinta.

**Detalle**: [[<nota del vault>]] · `docs/vault/70-conocimiento/<archivo>.md`
```

## B) La nota del vault (`70-conocimiento/`)

```markdown
---
fecha: YYYY-MM-DD
tarea: T-NN-NNN
tipo: architecture | convention | constraint | domain | bugfix | discovery
engram: obs-xxxxxxxxxxxxxxxx
---

# <el mismo título que la observación>

## El requisito
Qué se estaba resolviendo.

## La regla que suena bien
El error, con el código. Por qué pasa una revisión.

## Por qué está mal acá
El mecanismo, con la evidencia que lo demuestra.

## Lo que sí es correcto
La regla buena, con su orden si lo tiene.

## Cómo se agarró
La mutación, el test, la revisión. Qué lo mató y qué no.

## Dónde más aplica
Enlaces `[[wiki]]` a las notas relacionadas.
```

## Trazabilidad, en las dos direcciones

- la observación apunta a la nota con `**Detalle**`
- la nota apunta a la observación con `engram:` en el frontmatter
- la tarea en `tasks.md` apunta a la observación con `- engram: obs-...`

Si una de las tres falta, el enlace se rompe en silencio y no hay nada que
avise. Se ponen las tres.
