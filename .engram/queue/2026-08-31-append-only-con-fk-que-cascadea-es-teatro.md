---
type: domain
score: 5
topic_key: mascotapp/domain/append-only-tables
approved: 2026-08-31 por el usuario
observation_id: obs-99c2e3298b483ab8
task: T-01-020
rationale: "El design del propio proyecto escribe ON DELETE CASCADE en esta clave. Copiarlo dejaba una tabla inmutable que se vacia entera por la puerta de al lado, y todas las capas la habrian dado por protegida."
---

# Una tabla append-only cuyo padre cascadea no esta protegida

`pet_status_history` es inmutable desde el dia 1 (mitigacion **LT-5**). Se implemento con
cuatro capas — politicas por comando, `REVOKE`, trigger de fila y trigger de `TRUNCATE`.

Con `ON DELETE CASCADE` en la FK a `pets`, como lo escribe el ejemplo de **D5**, todo eso
vale cero:

- `app_tenant` tiene `DELETE` sobre `pets`.
- Borrar el pet borra su historial **en cascada**.
- La cascada no consulta ninguna politica de `pet_status_history`, y el refugio nunca toco
  la tabla protegida: entro por la unica puerta que nadie estaba mirando.

El rastro se borra borrando al animal. Ninguna de las cuatro capas se entera.

## Por que `ON DELETE RESTRICT` no es un workaround

Es lo que el producto ya pedia. Por **LT-5** un animal no se borra: **cambia de estado**
(adoptado, fallecido, no disponible). Un pet con historial registrado es exactamente un pet
que solo puede tener borrado logico. La FK no agrega una restriccion nueva — hace cumplir la
que el dominio ya tenia escrita.

`pet_media` y `pet_health_records` conservan `CASCADE` a proposito: son **registros**, no
**historia**. Borrar el animal borra sus fotos; no borra la prueba de lo que le paso.

## La regla generalizable

**Al declarar una tabla inmutable, la primera pregunta no es que privilegios revocar sino
por donde se borra sin tocarla.** En Postgres esa puerta son las cascadas de las FK
entrantes. Aplica igual a `application_events` (T-01-029) y `audit_log` (T-01-032).

Relacionado: [[2026-08-31-truncate-es-la-escritura-que-ninguna-politica-ve]].
