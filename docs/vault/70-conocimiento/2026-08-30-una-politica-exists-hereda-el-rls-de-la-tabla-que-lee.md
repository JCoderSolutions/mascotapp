---
type: architecture
score: 5
topic_key: mascotapp/security/an-exists-subquery-inherits-rls
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-165de410af49853a
task: T-01-014
rationale: "La cláusula que parece redundante y la que es load-bearing se ven idénticas; borrar la equivocada deja una tabla sin aislar."
---

# Una política `EXISTS` hereda el RLS de la tabla que lee

La política de `users` (D6, la única excepción sancionada) es:

```sql
USING (EXISTS (SELECT 1 FROM memberships m
               WHERE m.user_id = users.id
                 AND m.shelter_id = current_setting('app.shelter_id', true)::uuid))
```

**Ese `AND` es redundante.** Probado por mutación: borrarlo no cambia **nada**
observable. Una expresión de política se evalúa como el rol que consulta, así que
la subconsulta sobre `memberships` está ella misma filtrada por la política de
`memberships`. El scope se aplica dos veces.

Es el mismo mecanismo detrás del error famoso *"infinite recursion detected in
policy for relation"*: las políticas de la tabla referenciada **sí** se aplican.

## Confirmado, no inferido

Con el mutante solo, el suite queda verde. Con el mutante **más** una política
`SELECT` permisiva sobre `memberships`, la grilla falla exactamente donde debe:
*un miembro de otro refugio es invisible* → `visible = true, want false`.

## Lo que hay que hacer con esto

La cláusula **se queda**, y el porqué quedó escrito en la migración y en el test.
Sin eso, la próxima persona nota la redundancia, la limpia — un cambio que ningún
test rechaza — y deja el aislamiento de `users` apoyado enteramente en una regla
que no está cerca de ese archivo.

## La regla general

Con `EXISTS` a través de otra tabla, **el aislamiento de la tabla es una
conjunción, y la mitad vive en otra tabla.** El test de esta tabla no puede
probarla entera: la otra mitad la prueba el caso A/B de la tabla referenciada.
Ver [[2026-08-30-contar-politicas-no-dice-para-quien-son]] — el mismo tipo de ceguera: la
aserción parece local y no lo es.
