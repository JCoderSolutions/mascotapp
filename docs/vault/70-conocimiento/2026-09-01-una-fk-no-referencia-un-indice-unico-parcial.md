---
type: constraint
score: 4
topic_key: mascotapp/security/fk-cannot-reference-partial-unique
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-28d2d22bf005152e
task: T-01-028
rationale: "Explica por que 'el asignado tiene que ser un miembro ACTIVO' no se puede hacer cumplir con la clave compuesta, y evita que alguien lo intente de nuevo en Fase 02."
---

# Una foreign key no puede referenciar un indice unico PARCIAL

Verificado en PostgreSQL 17, no supuesto:

```sql
CREATE UNIQUE INDEX m_active ON m (user_id, shelter_id) WHERE status = 'active';
ALTER TABLE a ADD FOREIGN KEY (assignee, shelter_id) REFERENCES m (user_id, shelter_id);
-- ERROR:  there is no unique constraint matching given keys for referenced table "m"
```

La clave referenciada tiene que ser una unique constraint (o un indice unico) **total**. Un
indice parcial no califica, por mas que cubra exactamente las columnas.

**La consecuencia concreta en este esquema.**
`adoption_applications.assigned_to_user_id` es una clave compuesta a `memberships (user_id,
shelter_id)`, que encierra la asignacion dentro del refugio. Lo que **no** puede hacer es exigir
que la membresia este **activa**, porque eso necesitaria exactamente el indice parcial de arriba.

Y eso deja dos capas en desacuerdo **por construccion**:

```
asignable = la fila de membresia existe
legible   = la fila existe Y status = 'active'   (member_visible_users, T-01-016)
```

O sea que un refugio puede asignarle un caso a alguien que **no puede leer**. No es una fuga
entre tenants — todos pertenecen a ese refugio —, asi que la respuesta de la base es
**incompleta**, no equivocada.

**Lo que NO hay que hacer:** volver a intentar la FK parcial en Fase 02. No existe. Las salidas
reales son un trigger, o la regla en la capa de dominio junto con RBAC. Se eligio dominio, y
`TestAssignment_DoesNotYetRequireAnActiveMembership` deja la conducta actual por escrito para que
angostarla sea un cambio visible.

**La forma general:** una clave compuesta puede acotar una relacion (*"pertenece a este
refugio"*), pero **no puede acotar un ESTADO de esa relacion** (*"y sigue vigente"*). Cuando la
regla incluye un estado, la clave llega hasta la mitad y hay que decir en voz alta donde vive la
otra mitad.

Ver [[2026-09-01-una-clave-compuesta-a-memberships-encierra-la-asignacion]].
