---
type: architecture
score: 5
topic_key: mascotapp/arch/intra-vs-inter-transaction-invariants
task: T-02-021
status: pendiente-de-aprobacion
relacionado: obs-a88eecda20bb1bc4
rationale: "Un índice único parcial y un lock consultivo se ven como la misma clase de herramienta —'poné una restricción en la base y listo'— y no lo son. Uno hace cumplir un invariante DENTRO de una transacción; el otro serializa DOS. Confundirlos me costó una de las dos rondas de corrección de Judgment Day, sobre la unidad más crítica de seguridad de la fase, y el error no lo detectó ninguna revisión: lo detectó un test de concurrencia que el actor de corrección escribió porque se lo pedí. Dentro de tres meses cualquiera que lea 'agregamos un índice único para la carrera' va a repetirlo, porque la frase suena bien."
---

# Un índice único parcial no cierra una carrera entre transacciones

(T-02-021, Judgment Day de MascotApp Fase 02. Relacionado con `obs-a88eecda20bb1bc4`.)

## El defecto

`RevokeRefreshTokenFamily` revoca toda una familia de refresh tokens con una sentencia:

```sql
UPDATE refresh_tokens SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;
```

Bajo **READ COMMITTED**, el conjunto de filas candidatas de un `UPDATE` **queda fijado en el
snapshot que toma esa sentencia**. Una fila que otra transacción inserta después **nunca entra
en ese conjunto**.

`EvalPlanQual` —la re-evaluación que Postgres hace cuando el `UPDATE` se bloquea contra una
fila lockeada— **solo re-examina filas que ya estaban** en el escaneo original. Nunca descubre
filas nuevas.

Consecuencia: durante una revocación de familia disparada por detección de reutilización, un
sucesor insertado por una rotación concurrente **sobrevive**. Y sobrevive **para siempre**,
porque nada revoca dos veces una familia ya revocada. Ese token queda fuera de la contención.

## El arreglo que NO funcionó, y por qué sonaba bien

```sql
CREATE UNIQUE INDEX one_live_token_per_family
  ON refresh_tokens (family_id) WHERE revoked_at IS NULL;
```

"Como máximo un token vivo por familia" — parece exactamente la garantía que falta. **No lo
es.**

Un índice único hace cumplir un invariante **sobre el estado committeado**. Impide que *una*
transacción deje dos filas vivas. **No hace que la sentencia de revocación de otra transacción
descubra una fila creada después de su propio snapshot.** El defecto no era "existen dos filas
vivas"; era "una sentencia no ve una fila". Son problemas distintos.

Medido, no razonado: con el índice puesto, el test de concurrencia **seguía fallando 3 de 4
corridas**, en el trial 1 o 2 de 25.

## La distinción que hay que llevarse

| Herramienta | Qué hace cumplir |
|---|---|
| `UNIQUE`, `CHECK`, FK, índice parcial | Un invariante sobre el estado **committeado**. Intra-transacción. |
| Lock consultivo, `SELECT ... FOR UPDATE`, SERIALIZABLE | **Orden entre dos transacciones**. Inter-transacción. |

**Si el problema es "quién ve qué y cuándo", una restricción declarativa no lo resuelve, por
más que su enunciado se parezca al síntoma.**

## Lo que sí funcionó

```sql
SELECT pg_advisory_xact_lock(hashtext($1::text)::bigint);
```

Tomado sobre el `family_id` después de leer la fila presentada y **antes de decidir nada**.
Mientras se sostiene, ninguna otra transacción empieza a decidir sobre esa familia: o la
revocación ya commiteó (y entonces el guard `revoked_at IS NULL` de la rotación da cero filas y
la rechaza) o no puede arrancar hasta que el sucesor esté commiteado y por lo tanto visible.

`xact` libera en COMMIT o ROLLBACK, así que ningún camino lo filtra. Una colisión de hash entre
dos familias cuesta concurrencia, nunca corrección.

Resultado: **125 trials verdes**. Quitando el lock, falla en las 3 corridas.

## El índice se quedó igual, y por qué

Cierra un problema **distinto y real**: que una rotación ordinaria tenga transitoriamente dos
filas vivas. Y obligó a reordenar la rotación a revocar → insertar → back-pointer, que es más
correcto que el orden anterior. **No se revierte un arreglo por haber resuelto otra cosa que la
que uno creía.** Solo hay que dejar escrito cuál cosa resolvió.

## Cómo se detectó el error, que es la parte reusable

No lo detectó una revisión. Lo detectó **el test de concurrencia**, que existía porque al
delegar la corrección se pidió explícitamente *"un test que falle antes del arreglo y pase
después"*.

Ese pedido es lo que convirtió un hallazgo `inferential` —dos jueces razonando sobre semántica
MVCC— en un defecto **reproducible**, y es lo único que impidió declarar cerrada una carrera
que seguía abierta. Sin ese test, el índice se mergeaba con la convicción de haber arreglado
algo.

**Pedir el test de reproducción antes que el arreglo no es ceremonia: es lo que distingue
"arreglado" de "creo que arreglado".**
