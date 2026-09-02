# ADR-0011 — Política de extensiones: se verifica una por una, no se generaliza

- **Fecha:** 2026-09-01
- **Estado:** aceptado
- **Fase:** 01

## Contexto

El esquema quería dos cosas que en PostgreSQL se resuelven con extensiones:

- **UUIDv7** como primary key — ordenable en el tiempo, que es lo que hace que un índice B-tree
  sobre la clave no se fragmente. La extensión es `pg_uuidv7`.
- **Comparación de email sin distinguir mayúsculas** — la extensión es `citext`.

Y arrastraba una regla heredada, **D3**, que decía: *"no dependemos de extensiones, porque el free
tier de Neon no las garantiza"*.

Esa regla es un ejemplo exacto de una generalización cómoda. Se formó a partir de **un** caso
verdadero —`pg_uuidv7` genuinamente **no** está en Neon— y se aplicó a **todos**. Bajo esa regla,
`users.email` iba a ser `text` con un índice único sobre `lower(email)`, desviándose de lo que §4
escribe.

**La premisa se verificó falsa.** `citext` no es una apuesta: Neon lo documenta en una página
dedicada, y `postgres:17-alpine` —la imagen del compose— trae `citext 1.6` de fábrica, confirmado
corriendo `CREATE EXTENSION` en un contenedor descartable. **Los dos entornos objetivo lo tienen.**

## Decisión

**Cada extensión se decide por separado, contra evidencia de los DOS entornos. No hay una política
de "sí a las extensiones" ni de "no a las extensiones".**

Bajo esa regla:

- **`citext` — SÍ.** `users.email` es `citext NOT NULL UNIQUE`, exactamente como §4 lo escribe.
  `00001` corre `CREATE EXTENSION IF NOT EXISTS citext` como owner, antes de que exista ninguna
  tabla.
- **`pg_uuidv7` — NO.** No está disponible en Neon. **UUIDv7 se genera en Go**, y ninguna primary
  key de este esquema lleva default de base: el identificador lo provee siempre quien inserta.

**El alcance de D3 queda ANGOSTADO, no roto.** D3 rechazaba `pg_uuidv7`, y eso sigue en pie. Lo
que no se sostiene es el salto de ahí a *"ninguna extensión"*.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| `text` + índice único sobre `lower(email)` | Ambas formas garantizan **unicidad** sin distinguir mayúsculas. Difieren en la **búsqueda**: con el índice sobre `lower()`, un `WHERE email = $1` **falla en silencio** contra una dirección escrita distinto, salvo que **cada** call site se acuerde de normalizar — al escribir y al leer. Es corrección apoyada en disciplina, que es el modo de falla que [[ADR-0002]] existe para eliminar. |
| `text` con una collation ICU no determinística | La respuesta moderna de PostgreSQL y sin extensiones. Pero los operadores de patrón (`LIKE`, `~`) **no funcionan** sobre una collation no determinística, lo que cerraría en silencio una búsqueda de usuarios por prefijo en el panel de administración. |
| Generar UUIDv7 con una función plpgsql propia | Código criptográfico y de precisión temporal escrito a mano, sin la revisión que tiene una extensión, para ahorrar una línea en Go. |
| Aceptar UUIDv4 y dejar de lado el orden temporal | Fragmenta el índice de la primary key en cada tabla. `audit_log` además necesita orden **monótono real**, que UUIDv7 no da: su resolución es de milisegundos, y este esquema escribe varias filas dentro del mismo. Por eso `audit_log` usa `bigserial` y no un uuid. |

## Consecuencias

**A favor**

- `users.email` se comporta como el producto espera **por el tipo de la columna**, donde no se
  puede olvidar.
- Ninguna primary key depende de la base para existir, así que el identificador se conoce **antes**
  del `INSERT` — que es lo que permite construir un grafo de objetos y escribirlo en una sola
  transacción.
- Una migración que necesite una extensión nueva tiene un procedimiento: verificarla en los dos
  entornos y anotar la evidencia. No una prohibición que hay que discutir de nuevo.

**En contra, y se acepta**

- **`citext` es medible más lento que `text`**: cada comparación baja a minúsculas internamente.
  Sobre un login por email es ruido.
- **El proyecto ahora depende de una extensión.** Si Neon la sacara, hay que migrar la columna. La
  mitigación es que el cambio está acotado a una columna y a una migración.
- **La verificación tiene fecha.** *"Verificado en agosto de 2026"* no es *"verificado"*. Vuelve a
  chequearse cuando cambie el proveedor o la versión mayor de PostgreSQL.

**La lección que sobrevive a las dos extensiones concretas:** una restricción heredada se verifica
antes de diseñar contra ella. D3 sobrevivió como regla general hasta que alguien la probó, y en el
medio ya había desviado el esquema de lo que la propuesta decía. **Un "no se puede" sin evidencia
cuesta más que un "probémoslo".**

## Condiciones de reapertura

1. Que `pg_uuidv7` aparezca en Neon. Es un cambio de default, no de arquitectura: los ids ya son
   UUIDv7.
2. Que un perfilado muestre que `citext` importa en una ruta caliente real. Hoy la única es el
   login.
3. Que aparezca una tercera extensión candidata — `pg_trgm` para búsqueda difusa en el catálogo
   público de la Fase 06 es la más probable. Se decide por separado, con evidencia de los dos
   entornos, que es justamente lo que este ADR establece.
