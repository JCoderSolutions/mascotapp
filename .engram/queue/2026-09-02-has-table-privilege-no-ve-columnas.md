---
type: constraint
score: 4
topic_key: mascotapp/security/column-grants-are-not-table-privileges
task: T-02-003
status: guardado
observation_id: obs-eb0d04da21d94427
rationale: "Toda la Fase 02 se apoya en grants por columna. Un test que los verifica con has_table_privilege reporta 'sin permiso' sobre un grant que funciona, y el diagnostico obvio -- ensanchar el grant a la tabla -- destruye exactamente la proteccion que se estaba construyendo."
---

**Verificado en vivo contra PostgreSQL 17, no asumido:**

```sql
CREATE ROLE r LOGIN;
CREATE TABLE t (a int, b int);
GRANT SELECT (a) ON t TO r;

has_table_privilege('r','t','SELECT')        -> false
has_column_privilege('r','t','a','SELECT')   -> true
has_column_privilege('r','t','b','SELECT')   -> false

GRANT SELECT ON t TO r;                      -- a nivel tabla
has_table_privilege('r','t','SELECT')        -> true
```

**Un grant por columna NO es un privilegio de tabla.** `has_table_privilege` devuelve
`false` aunque el rol pueda leer perfectamente las columnas concedidas.

**Por que importa aca:** P2-D3 concede `users` y `memberships` a `app_auth` **por columna**
a proposito -- ese es el mecanismo que impide que la puerta de auth lea `full_name` o
`phone`. Un inventario de privilegios escrito con `has_table_privilege` reporta esas filas
como "sin permiso" sobre grants que funcionan, y el arreglo que primero se le ocurre a
cualquiera -- ensanchar a `GRANT SELECT ON users` -- **destruye la proteccion entera**.

**La forma correcta de fijarlo, y es mas fuerte que la ingenua:** en el inventario a nivel
tabla se espera **`false`** para toda columna concedida por columna, con el motivo escrito.
Asi el test se pone rojo el dia que alguien ensancha el grant. La cobertura precisa por
columna vive aparte, en las aserciones de comportamiento (`auth_door_test.go`), donde se
lee cada columna concedida y se comprueba que las no concedidas dan `42501`.

`refresh_tokens` es el contraste que confirma la lectura: ahi el grant SI es de tabla
(`GRANT SELECT, INSERT, UPDATE, DELETE`), y sus cuatro filas del inventario siguen en `true`.

**Y el recordatorio de siempre:** el mensaje de la denegacion por columna dice
`permission denied for TABLE users` -- dice TABLE, no column. Se asierta el SQLSTATE `42501`,
nunca el texto. Ver [[mascotapp/convention/recompute-announced-numbers]] para la misma
familia de error: una afirmacion que nadie chequeo.
