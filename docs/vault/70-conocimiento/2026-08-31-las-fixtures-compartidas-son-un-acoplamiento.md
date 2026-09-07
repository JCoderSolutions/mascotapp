---
type: convention
score: 4
topic_key: mascotapp/convention/testing-shared-fixtures
approved: 2026-08-31 por el usuario
observation_id: obs-3acb038bf64919cf
task: T-01-021
rationale: "El contenedor compartido hace que el orden ALFABETICO de los archivos sea parte del contrato de los tests, y nadie lo escribio nunca. Rompio dos casos que no tenian nada que ver con el archivo nuevo."
---

# Las fixtures compartidas en un contenedor compartido son un acoplamiento

El paquete `rlstest` comparte un unico Postgres. `child_orphan_test.go` reusaba
`env.ShelterA` / `env.ShelterB`, ordena **primero** alfabeticamente en el paquete, y por eso
sembro antes que `TestTenantIsolation` por primera vez. Rompio dos casos ajenos por dos
caminos distintos:

1. **El caso `shelters` tiene `TenantColumn: "id"`** — la fila del tenant A **es** el refugio
   A — asi que ese caso **tiene que ser quien lo crea**. Sembrarlo antes convierte su INSERT
   en un `duplicate key value violates "shelters_pkey"` pelado, que esta lejisimos de la causa.
2. **El caso `pets` sondea un `DELETE FROM pets` sin `WHERE`**, la unica forma que puede cazar
   una politica DELETE permisiva. Cualquier pet de B que quede con historial lo rechaza de
   plano por `ON DELETE RESTRICT`.

## Las dos correcciones, y por que son dos

- **`freshTenant` acuna los refugios propios del archivo.** Las fixtures del runner A/B son
  del runner; un test ajeno no tiene por que estirar la mano. Esto arregla el caso concreto.
- **El caso `shelters` gano un `Fixture` que chequea si el refugio A ya existe y NOMBRA la
  causa.** Esto arregla el proximo caso. Documentar el peligro no sirve: el que lo pisa no
  esta leyendo esa linea, esta leyendo un `duplicate key`.

## La regla

**Un test que necesita tenants se los acuna; no toma prestados los del runner de
conformidad.** Y cuando una fixture compartida SI tiene que ser unica — como el refugio A,
que por diseno solo puede crearlo su propio caso — eso se hace cumplir con un chequeo que
falla por nombre, no con un comentario.

Relacionado:
[[2026-08-30-un-test-no-toma-prestado-un-nombre-que-el-esquema-va-a-reclamar]],
[[2026-08-30-delete-sin-where-esquiva-la-politica-select]].
