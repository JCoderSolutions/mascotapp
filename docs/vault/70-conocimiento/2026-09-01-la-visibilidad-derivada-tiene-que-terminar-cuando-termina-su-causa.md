---
type: domain
score: 5
topic_key: mascotapp/security/derived-visibility-must-expire
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-3b6ad00b3b71abae
task: T-01-028
rationale: "La misma pregunta se hizo dos veces sobre users y las dos veces la respuesta estaba en si la CAUSA sigue viva. Es la forma que hay que chequear en toda policy derivada, no un caso particular de memberships."
---

# La visibilidad derivada tiene que terminar cuando termina su causa

`users` es visible por dos motivos, cada uno con su policy permisiva:

- **membresia** — `member_visible_users`, via `memberships`
- **aplicacion** — `applicant_visible_users`, via `adoption_applications`

Las dos derivan la visibilidad de una fila en otra tabla, y las dos tienen que contestar la misma
pregunta: **¿que pasa cuando esa fila deja de significar lo que significaba?**

**La rama de membresia ya lo aprendio por las malas** (T-01-016, Judgment Day): sin
`m.status = 'active'`, revocar una membresia **no se llevaba la visibilidad**. Peor: `app_tenant`
tiene INSERT sobre `memberships`, asi que un tenant se fabricaba lectura sobre la PII de
cualquiera insertando una membresia `invited`.

**La rama de aplicante tiene el mismo esqueleto** y su equivalente de revocacion es que la
aplicacion **desaparezca**: un retiro, una purga de retencion, un pedido de borrado del titular.
§5.4 le da al titular ese derecho — y si la visibilidad sobreviviera a la aplicacion,
*"borrame los datos"* dejaria al refugio pudiendo leer a la persona igual. **Ese es exactamente
el resultado que el pedido estaba tratando de evitar.**

Acá funciona porque la policy consulta un `EXISTS` sobre la fila viva y la fila se borra de
verdad. Dejaria de funcionar el dia que `adoption_applications` gane un `deleted_at`: el borrado
logico convierte *"la causa termino"* en *"la causa sigue ahi con un flag"*, y toda policy
derivada que no lo mire empieza a mostrar de mas **sin cambiar una linea**.

**La pregunta a hacerle a TODA policy derivada, antes de escribir el test:**

1. ¿Cual es la fila que causa la visibilidad?
2. ¿Que la termina — un borrado, un cambio de estado, un vencimiento?
3. ¿La policy MIRA eso?
4. ¿Quien puede ESCRIBIR esa fila? (ver [[2026-09-01-una-capa-que-contesta-primero-vacia-el-test-de-abajo]]
   y la regla de D6: permiso de escritura sobre el puente es permiso de lectura sobre la tabla)

Las cuatro se contestan con un test. La 3 es la que nadie escribe, porque la policy se lee
correcta mirandola.
