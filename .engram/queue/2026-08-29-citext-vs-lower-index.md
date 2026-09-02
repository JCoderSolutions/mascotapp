---
type: decision
score: 4
topic_key: mascotapp/convention/case-insensitive-email
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-1ee4f1b598f5b471
task: phase-01-domain-and-data (design D7, revertida por el usuario)
rationale: "El valor no está en qué tipo ganó, sino en cómo se ganó: una decisión de diseño se había tomado sobre una premisa que nadie verificó, y verificarla costó dos minutos y la dio vuelta."
---

**`users.email` es `citext`, no `text` + índice único sobre `lower(email)`.**

## Lo que pasó

El agente de diseño desvió la columna de lo que decía §4 del plan, con este argumento:

> *"D3 rechazó las dependencias de extensiones porque la allow-list de Neon no está
> confirmada. Aplicando el mismo razonamiento: `email text NOT NULL`..."*

**La premisa era falsa, y verificarla costó dos minutos:**

- Neon tiene página dedicada para la extensión: `neon.com/docs/extensions/citext`
- `postgres:17-alpine` — la imagen del compose — trae `citext 1.6` de fábrica, confirmado
  corriendo `CREATE EXTENSION IF NOT EXISTS citext` en un contenedor descartable

Los dos entornos objetivo la tienen. Sin la premisa, la desviación de §4 se quedaba sin
nada que la sostuviera.

## Por qué `citext` gana por mérito, no solo por §4

Las dos formas garantizan **unicidad** insensible a mayúsculas. Difieren en la **búsqueda**:

- Con índice sobre `lower(email)`, `WHERE email = $1` **falla en silencio** con un mail
  escrito con otro casing, salvo que *cada* llamador se acuerde de normalizar — en la ruta
  de escritura y en la de lectura.
- Con `citext` la propiedad vive en el tipo de la columna, donde no se puede olvidar.

Eso es exactamente lo que dice [[ADR-0002]]: *"la base es la última línea de defensa y no
depende de la disciplina de quien escribe el SQL."* El mismo criterio que eligió RLS sobre
`WHERE shelter_id = ?` elige `citext` sobre `lower()`.

Costo honesto: cada comparación baja a minúsculas internamente, así que `citext` es más
lento que `text`. En un login por mail eso es ruido.

Descartado: `text` con collation ICU no determinista — es lo que hoy recomienda la doc de
PostgreSQL y no necesita extensión, pero los operadores de patrón (`LIKE`, `~`) **no
funcionan** sobre una columna así, y eso cierra en silencio un buscador de usuarios por
prefijo en el panel de admin.

## Dos reglas que quedan

1. **`D3` se estrecha, no se rompe.** "No usar extensiones" nunca fue la regla. La regla es
   *no depender de una extensión que no verificaste en el entorno destino*. `pg_uuidv7`
   efectivamente no está en Neon, así que UUIDv7 se sigue generando en Go. `citext` sí está.
2. **El `Down` de la migración NUNCA dropea la extensión.** `DROP EXTENSION citext`
   **cascadea a `users.email`** y destruye la columna que `goose down-to` debería dejar
   recuperable. Misma asimetría que los roles en D1b, y por la misma razón: puede ser
   compartida por objetos que esa migración no creó.

## La lección transferible

Una decisión de arquitectura heredó "la allow-list no está confirmada" de otra decisión y
la trató como hecho. Nadie la verificó porque venía envuelta en un razonamiento correcto.
**Cuando una decisión se apoya en la premisa de otra, verificá la premisa otra vez en el
contexto nuevo** — `pg_uuidv7` y `citext` no son la misma pregunta aunque las dos empiecen
con `CREATE EXTENSION`.
