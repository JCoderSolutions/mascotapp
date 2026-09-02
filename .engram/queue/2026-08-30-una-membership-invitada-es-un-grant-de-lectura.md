---
type: architecture
score: 5
topic_key: mascotapp/security/write-on-the-bridge-is-read-on-the-table
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-217a73f022ead863
task: T-01-016
rationale: "Una política EXISTS convierte el permiso de escritura sobre la tabla puente en permiso de lectura sobre la tabla protegida."
---

# Una política `EXISTS` hereda los **permisos de escritura** de la tabla puente

`users` no tiene `shelter_id`, así que su visibilidad se deriva a través de
`memberships`:

```sql
USING (EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND ...))
```

Y `app_tenant` tiene **INSERT y UPDATE sobre `memberships`**.

Esas dos frases juntas dicen algo que ninguna de las dos dice sola: **el tenant
controla la condición de su propia política de lectura.** Probado contra
PostgreSQL 17:

```
sin membership:              stranger visible = false
inserta una INVITED (como app_tenant): visible = true
la REVOCA:                             visible = true   <-- todavía
```

Un refugio se acuña lectura del perfil de cualquier usuario cuyo id conozca —
mail, teléfono, nombre — y revocar no se la quita.

## La regla general

Cuando una política de la tabla P se deriva por `EXISTS` sobre la tabla puente B:

> **el permiso de ESCRITURA sobre B es permiso de LECTURA sobre P.**

Así que la política tiene que filtrar por el estado que el negocio considera
válido, no por la mera existencia de la fila:

```sql
AND m.status = 'active'
```

El spec ya decía la palabra — *"user U has an **active** membership"* — y la
política no la implementaba. La distancia entre el spec y el DDL era una fuga de
PII.

## Lo que hay que preguntarse la próxima vez

Por cada carve-out `EXISTS`: **¿quién puede escribir filas en la tabla puente, y
qué le habilita eso a leer?** Ver
[[una-politica-exists-hereda-el-rls-de-la-tabla-que-lee]] — es la otra mitad de
la misma idea: esa nota dice que hereda el *filtro* de B, esta dice que hereda
sus *permisos de escritura*.
