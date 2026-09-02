---
type: convention
score: 4
topic_key: mascotapp/security/a-policy-count-hides-its-role
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-20b9dbca96480d36
task: T-01-013
rationale: "Un meta-test de esquema puede parecer exhaustivo y ser ciego al eje que más importa: el rol."
---

# Un meta-test que cuenta políticas no ve **para quién** son

El meta-test del catálogo afirma, para cada tabla: RLS habilitada, RLS forzada,
y al menos una política. Suena completo. La mutación mostró qué le falta.

Mutante M3: cambiar `member_visible_users` de `TO app_tenant` a `TO app_public`.
**No rompió nada.** `pg_policy` seguía teniendo una fila, `relrowsecurity` seguía
en `true`, y la migración seguía leyéndose correcta. En ese estado `users` no era
legible por nadie — `app_public` no tiene grant sobre la tabla — y el suite
entero quedaba en verde.

El mismo test ciego dejó pasar dos mutantes más: ensanchar el grant de `users` de
`SELECT` a escritura completa, y darle a `refresh_tokens` un grant que su
default-deny no contempla.

## La regla

Un chequeo de esquema tiene **tres ejes**, no uno: *existe*, *para qué comando*,
y **para qué rol**. Contar filas de `pg_policy` cubre el primero y ninguno de los
otros dos.

El cierre fue un test angosto — `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` —
que fija nombre, `polcmd` y `polroles` de cada política de esa migración, más
`has_table_privilege` por rol/tabla/privilegio. **Un solo test mató tres
mutantes**, dos de los cuales yo ya había descartado como "problema de una tarea
posterior".

Corolario: la ausencia de política también es contrato. `refresh_tokens` aparece
en el test por lo que **no** tiene, porque RLS encendida sin política es lo que
niega a todo el mundo. Ver [[dos-reglas-un-centinela]].
