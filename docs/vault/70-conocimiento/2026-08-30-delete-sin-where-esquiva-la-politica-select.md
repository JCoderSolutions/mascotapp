---
type: constraint
score: 5
topic_key: mascotapp/security/unqualified-delete-bypasses-select-policy
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-81f7b8b17c3780db
task: T-01-009 (runner A/B)
rationale: "Una política DELETE completamente rota es INDETECTABLE con un DELETE con WHERE. El test que creías que la cubría no la cubre, y el ataque real usa justo la forma que tu test no prueba."
---

**Un `DELETE ... WHERE` nunca puede detectar una política DELETE permisiva. Solo lo hace un
`DELETE FROM tabla` sin `WHERE`.**

Verificado contra la documentación de PostgreSQL 17 (`sql-createpolicy.html`):

> *"Because DELETE commands often need to read data from columns (such as in a WHERE or
> RETURNING clause), SELECT rights are typically required on the relation. In these cases, the
> appropriate SELECT or ALL policies are applied **in addition to** the DELETE policies."*

Y lo mismo para UPDATE: *"the user must satisfy both SELECT and UPDATE policies to modify a
row."*

## La consecuencia, que es al revés de lo intuitivo

Con la política **SELECT correcta**, una política **DELETE completamente permisiva**
(`USING (true)`) es **inalcanzable** por una sentencia con `WHERE`. La sentencia se frena en la
política SELECT, y la de DELETE nunca se ejercita.

O sea: el test A/B que dice *"el inquilino B no puede borrar las filas de A"*, escrito de la
forma obvia —

```sql
DELETE FROM tabla WHERE id = $1   -- como B
```

— **pasa siempre**, esté la política DELETE bien o catastróficamente mal. Verde permanente,
cero protección.

## El ataque real

```sql
DELETE FROM tabla    -- sin WHERE
```

No lee ninguna columna. La política SELECT **no aplica**. Solo queda la de DELETE. Si está
permisiva, **el inquilino B borra todas las filas de A sin haber podido verlas nunca.**

Es el peor de los casos: destrucción total de datos ajenos por un rol al que ni siquiera se le
mostraron.

## Reglas que quedan

1. El runner A/B lleva `checkTenantBCannotWipeTheTable`: como B, `DELETE FROM tabla` sin
   `WHERE` debe afectar **0 filas**. Es la única aserción del runner que puede atrapar esto.
2. **El probe siempre hace rollback.** Un test que prueba que los datos se pueden destruir no
   los destruye. (Y eso también hay que testearlo: el mutante que hacía comitear el probe
   sobrevivía, porque en una tabla correcta el DELETE no borra nada.)
3. Las aserciones de UPDATE/DELETE **con** `WHERE` se conservan como defensa en profundidad y
   están documentadas como tales. No pueden disparar mientras la política SELECT esté bien, y
   fingir lo contrario sería exactamente el falso confort que esta fase existe para eliminar.
4. Para UPDATE no hay equivalente genérico: un `UPDATE` sin `WHERE` igual lee columnas en el
   `SET`. Queda anotado como límite conocido del runner.

Emparenta con [[2026-08-29-fk-checks-bypass-rls]]: las dos son rutas por las que PostgreSQL
evalúa algo **sin** consultar la política que uno cree que lo cubre. RLS no es un perímetro; es
un conjunto de reglas por comando, y hay que saber cuál aplica a qué.
