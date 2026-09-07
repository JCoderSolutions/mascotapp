---
type: convention
score: 4
topic_key: mascotapp/convention/equivalent-mutant-disposition
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-773c9a9bbd23df5a
task: T-01-025
rationale: "Un sobreviviente de mutacion tiene exactamente tres destinos, y 'lo dejamos pasar' no es ninguno. Sin esta regla escrita, el primer sobreviviente incomodo se convierte en precedente."
---

# Un mutante que sobrevive se cierra: se mata, se prueba equivalente, o se reemplaza

Un mutante que sobrevive tiene **tres destinos posibles y ninguno es aceptarlo**:

1. **Falta un test** → se escribe. El mutante muere.
2. **Es equivalente** → se **prueba** que ningun test sobre la base puede distinguirlo, y la
   prueba se escribe al lado del mutante. Ademas se cierra la **clase** a la que apuntaba.
3. **El mutante estaba mal apuntado** → se reemplaza por uno que si cambia el estado observable.

T-01-025 tuvo uno de cada uno de los dos ultimos, en la misma ronda.

`REVOKE TRUNCATE ON form_submissions FROM app_tenant, PUBLIC` borrado **sobrevivio**. La
tentacion es anotarlo como "defensa en profundidad, no testeable". La disposicion correcta fue
demostrar por que:

- ninguna sentencia del esquema otorga `TRUNCATE` jamas — todos los `GRANT` deletrean
  `SELECT/INSERT/UPDATE/DELETE`;
- `00001` declara que no hay `ON ALL TABLES` ni `ALTER DEFAULT PRIVILEGES` en ningun lado;
- PostgreSQL no le da `TRUNCATE` a `PUBLIC` por defecto.

Entonces el `REVOKE` **llega al mismo estado de privilegios que su propia ausencia**. Ningun
test sobre la **base** puede separarlos, y uno que pudiera tendria que afirmar el **texto** de
la migracion — que es el tipo de test equivocado.

La sentencia **se queda**: es el guard que dispara el dia que el esquema adquiera default
privileges. Lo que se reemplazo fue el **mutante**, por `GRANT ..., TRUNCATE`, que si ensancha
el estado y si muere.

**Y la clase se cerro igual.** El inventario tenia tres filas `TRUNCATE, false` escritas a
mano, que es proteccion para tres tablas. Se agrego un test que enumera el catalogo: ningun rol
de aplicacion tiene `TRUNCATE` sobre ninguna tabla, nunca. El mutante equivalente no revelaba
un bug, pero revelaba **donde la proteccion dependia de que alguien se acordara**.

Ver [[inventario-por-enumeracion-no-por-lista]] y
[[truncate-es-la-escritura-que-ninguna-politica-ve]].
