---
type: bug
score: 4
topic_key: mascotapp/convention/testing-layer-attribution
approved: 2026-08-31 por el usuario
observation_id: obs-fedf7a4ef4efc6e3
task: T-01-022
rationale: "El test parecia fijar los grants y no los fijaba. La mutacion lo mostro y hubo que corregir el comentario, no el test: creerle al comentario habria dejado el rol publico con un bit de escritura sin que nada chillara."
---

# Cuando dos capas devuelven el mismo SQLSTATE, ninguna sonda de comportamiento las separa

El test de escrituras publicas afirmaba, en su propio comentario, que el caso sobre transaccion
ordinaria fijaba **los grants** de `app_public`. **No los fija.**

Lo mostro el mutante M1 de T-01-022 — `GRANT INSERT ON pets TO app_public` — que dejo **verdes
las dos** pruebas de comportamiento:

- `public_catalog` es `FOR SELECT`, asi que **no hay politica de INSERT** para `app_public`.
- Con el grant puesto, lo que rechaza es la politica faltante.
- Y una violacion de RLS al escribir reporta **`42501`**: el mismo codigo que la falta de grant.

Dos capas distintas, un solo SQLSTATE. Los mensajes si difieren (`permission denied for table`
vs `new row violates row-level security policy`), pero matchear texto de mensajes es fragil y
no se hizo.

## Que se corrigio, y que no

Se corrigio **el comentario**, no el test: el rechazo end-to-end sigue valiendo la pena
afirmarlo, y el caso sobre transaccion ordinaria sigue probando algo real — que la base rechaza
por su cuenta y no gracias al `AccessMode: pgx.ReadOnly` del helper (verificado con un mutante
que se lo saca).

Lo que **si** fija los grants es una enumeracion sobre el catalogo: todas las tablas x
`INSERT`/`UPDATE`/`DELETE`/`TRUNCATE`. Eso la vuelve **estructural, no redundante** — y ademas
cubre cada tabla que agregue una migracion futura el dia que aterriza, sin que nadie extienda
una lista escrita a mano.

## La regla

**Antes de afirmar en un comentario que un test fija una capa, matar esa capa y mirar.** Un
comentario que atribuye mal una garantia es peor que no tenerlo: describe una red que nadie
tejio, y el proximo que lea el archivo va a confiar en ella.

Relacionado: [[2026-08-30-testear-un-check-no-es-testear-que-corre]],
[[2026-08-31-rechazar-no-alcanza-tienen-que-rechazar-igual]].
