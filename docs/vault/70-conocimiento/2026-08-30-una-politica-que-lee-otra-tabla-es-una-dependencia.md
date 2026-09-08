---
type: constraint
score: 4
topic_key: mascotapp/domain/a-policy-that-reads-blocks-drop-table
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-57ee5e7a04dc4d61
task: T-01-013
rationale: "El orden del Down se deriva de las FK, y una política EXISTS agrega una arista que las FK no muestran."
---

# Una política que **lee** otra tabla bloquea el `DROP TABLE` de esa otra tabla

`member_visible_users` vive **en** `users` pero **lee** `memberships`:

```sql
CREATE POLICY member_visible_users ON users FOR SELECT TO app_tenant
  USING (EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND ...));
```

PostgreSQL registra eso como una dependencia. El `Down` de la migración —
escrito en orden inverso de creación, que es lo que las claves foráneas piden —
falló con **2BP01: cannot drop table memberships because other objects depend on it**.

El orden del `Down` se deriva de las FK, y esta arista **no es una FK**. No
aparece en `information_schema.table_constraints` ni en un diagrama de
relaciones. Solo aparece cuando el rollback se ejecuta de verdad.

## El arreglo, y el que se descartó

```sql
DROP POLICY member_visible_users ON users;   -- primero, y por nombre
DROP TABLE refresh_tokens;
DROP TABLE memberships;
...
```

**Descartado: `DROP TABLE ... CASCADE`.** Habría resuelto esto y todo lo que
llegue a depender de estas tablas en el futuro, en silencio. Un rollback cuyo
radio de daño es "lo que el esquema contenga ese día" no es un rollback que
alguien pueda revisar.

## Lo que lo encontró

El round-trip escalonado de T-01-011 — el que baja **un paso a la vez** en vez de
ir directo a 0. Ver [[2026-08-30-un-round-trip-no-ve-el-medio-del-camino]]. Ningún review
humano de este SQL iba a ver la arista.
