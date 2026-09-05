---
tipo: judgment-day
tarea: T-02-021
target: f507692b771c937ede385cdba32a3927c639cdc8
fecha: 2026-09-04
ronda: 1
estado: pendiente-de-decision
---

# Judgment Day — PR-02-11 (`session.go` + la ruta de acceso a `refresh_tokens`)

Revisión ciega dual antes del merge, exigida por `FASE-02.md` (*Judgment Day antes del merge de
la rotación de tokens*) y por `T-02-021`.

## Target inmutable

Commit `f507692`, árbol limpio, 1.253 líneas en cinco archivos revisados como **una sola
unidad**:

| Archivo | Líneas |
|---|---:|
| `apps/api/internal/auth/session.go` | 360 |
| `apps/api/internal/auth/session_test.go` | 549 |
| `apps/api/internal/db/auth.go` | 139 |
| `apps/api/internal/db/migrations/00013_auth_role.sql` | 120 |
| `apps/api/internal/db/query/auth.sql` | 85 |

`internal/db/sqlcgen/` entró como contexto, nunca como hallazgo: es código generado.

Dos jueces ciegos (`jd-judge-a`, `jd-judge-b`) en paralelo, mismo alcance, mismos criterios,
sin conocimiento del trabajo del otro ni de las conclusiones del orquestador.

## Resultado — ronda 1

| | |
|---|---:|
| Severos **confirmados por ambos** | **1** |
| Severos reportados por uno solo (*suspect*) | 1 |
| Contradicciones | 0 |
| WARNING / INFO | 2 |

---

## JD-1 · CONFIRMADO POR AMBOS — la revocación de familia no es atómica

**Severidad: SEVERE. Bloquea el merge.**

`session.go:257-268` · `query/auth.sql:37-43`

La revocación de familia es un solo `UPDATE ... WHERE family_id = $1 AND revoked_at IS NULL`,
bajo **READ COMMITTED**, sin serialización de ningún tipo.

**El interleaving concreto** (planteado por el juez A, compatible con el análisis de B):

1. **T1** — el atacante presenta un token **ya rotado**. Dispara la rama de reutilización y
   ejecuta `RevokeRefreshTokenFamily`.
2. **T2** — en paralelo, el usuario legítimo rota `live` (el sucesor vivo y todavía no
   revocado). Inserta `fresh2` en **la misma familia** y recién después marca `live` como
   rotado.
3. Si el `UPDATE` de T1 toma su snapshot de sentencia **antes** de que T2 commitee, `fresh2`
   **no está en su conjunto de filas candidatas**.

Bajo READ COMMITTED, el conjunto de filas de un `UPDATE` queda fijado en el snapshot de la
sentencia. `EvalPlanQual` **re-evalúa** filas que ya estaban en ese conjunto y fueron
modificadas concurrentemente; **nunca agrega** filas que no eran visibles al empezar. Es
semántica documentada de PostgreSQL, no un detalle de implementación.

**Consecuencia:** `fresh2` **sobrevive** a la revocación de su propia familia. Y no hay una
segunda oportunidad: la familia ya fue "revocada", así que nada la vuelve a revocar. Ese token
queda **permanentemente fuera de la contención**.

Eso contradice directamente el requisito del spec (`identity-and-session`, líneas 117-121):
*"MUST revoke every token sharing its `family_id`"*.

### Evidencia verificada de forma independiente por el orquestador

- `apps/api/internal/db/auth.go:53` usa `db.Begin(ctx)`. La interfaz `Beginner`
  (`db/tenant.go:42-44`) **no acepta `pgx.TxOptions`**, así que no existe camino de código por
  el cual `WithAuthUser` pueda pedir un aislamiento más fuerte. Se aplica el default de
  PostgreSQL, READ COMMITTED, incondicionalmente.
- `refresh_tokens` (`00002_tenancy_identity.sql:135-155`) lleva **solo** `token_hash bytea
  UNIQUE`. **No hay** índice único parcial ni constraint de exclusión sobre
  `(family_id) WHERE revoked_at IS NULL`. La migración `00013` tampoco agrega uno.

Los dos hechos que el escenario necesita son reales.

---

## JD-2 · SUSPECT — un solo juez lo llamó severo. NO se corrige en esta ronda.

`session.go:299-310`

Dos presentaciones concurrentes del **mismo token todavía no rotado**: ambas leen
`revoked_at = NULL`, ninguna entra en la rama de reutilización, y el perdedor de la carrera
recibe `ErrRefreshTokenInvalid` sin alarma y sin revocación de familia.

- **Juez B:** CRITICAL.
- **Juez A:** WARNING.

**El orquestador coincide con A, y con una razón que ninguno de los dos escribió:** el sistema
**se recupera solo en el intento siguiente**. El perdedor hace rollback (su sucesor desaparece)
y el token original queda marcado rotado. Cuando el usuario legítimo reintenta con ese token,
ahora sí encuentra `revoked_at` seteado, **dispara la detección de reutilización y la familia
muere**. Es exactamente la garantía documentada en `session.go:29-32`: *el robo no sobrevive al
próximo refresh del usuario legítimo*.

Lo que sí queda en pie de B, y por eso no se descarta: **la alarma llega tarde**, y el
comentario de `session.go:300-307` afirma *"el otro ganador es el mismo usuario legítimo"*, que
es una aseveración que el código **no puede verificar**. Eso es una corrección de comentario,
no de lógica.

---

## JD-3 · WARNING (ambos) — cero cobertura de concurrencia

`session_test.go` — las once funciones corren estrictamente en secuencia. Sin goroutines, sin
`WaitGroup`, sin transacciones concurrentes. La garantía *"revoca la familia ENTERA"* está
probada **solo** para el orden no concurrente, y calla justo sobre el interleaving que la
feature existe para defender.

---

## Lo que resistió la revisión adversarial

Ambos jueces lo verificaron por separado y coincidieron:

- El scoping por RLS (`auth_own_sessions`) hace lo que dice.
- El no-alarmar ante un prefijo `user_id` mangleado es correcto y está bien documentado como
  limitación en P2-D2.
- La expiración rechaza sin quemar la familia.
- `app_tenant` **no tiene absolutamente ningún grant** sobre `refresh_tokens`, verificado
  contra la migración `00002`.

---

## Ronda 1 de corrección — **FALLÓ**, y el error de análisis fue del orquestador

**Aprobado por el usuario:** índice único parcial `UNIQUE (family_id) WHERE revoked_at IS NULL`
(migración `00017`) más reordenar `RotateRefreshToken` a revocar → insertar → back-pointer.

**Presentado por el orquestador como "cierra JD-1 y JD-2 juntos". Esa afirmación era falsa.**

Cierra **JD-2**. **No cierra JD-1**, y la razón es la misma que hace severo a JD-1: el índice es
un invariante **dentro de una transacción**. Impide que *una* rotación tenga dos filas vivas a
la vez. **No** hace que la sentencia `UPDATE ... WHERE family_id` de otra transacción
**descubra** una fila creada después de su propio snapshot. El conjunto de filas candidatas de
esa sentencia sigue fijándose al empezar, sin importar en qué orden corran los statements de la
transacción vecina.

### Evidencia — ahora determinística, no inferencial

El actor de corrección escribió
`TestRotateRefreshToken_ConcurrentReuseAndRotationLeaveNoLiveTokenInTheFamily`: 25 trials por
corrida, dos goroutines reales con barrera de arranque — una re-presenta un token ya rotado
(dispara la revocación de familia), la otra rota el sucesor vivo. Aserta que después **no queda
ninguna fila viva** en esa familia.

| | Resultado |
|---|---|
| Con el fix aplicado, 4 corridas | **3 fallaron**, en el trial 1 o 2 de 25 |
| Sin el fix (baseline), 3 corridas | 1 falló |
| Verificación independiente del orquestador, 1 corrida | **falló** — `trial 1: 1 live token(s) remain` |

**Esto es una mejora real aunque el arreglo no sirviera:** `JD-1` entró a esta revisión como
hallazgo `inferential` de dos jueces razonando sobre semántica de PostgreSQL. Ahora hay un test
que lo reproduce.

### Lo que del trabajo de la ronda 1 SÍ se conserva

- La migración `00017` y el reordenamiento: cierran `JD-2` y hacen imposible por esquema que
  el camino normal tenga dos tokens vivos por familia. No se revierten.
- El test de concurrencia: es el que va a probar que la ronda 2 funcionó.

### Lo que falta, y por qué es un rediseño

Cerrar `JD-1` necesita **serializar dos transacciones distintas que tocan la misma familia**.
El actor de corrección se detuvo en vez de sustituir el diseño aprobado, que es exactamente lo
que se le pidió. Las opciones quedan para la decisión de la ronda 2.

## Estado

**Ronda 1 de corrección agotada. Queda una sola ronda** antes de que el veredicto sea
`ESCALATED`. Pendiente de la decisión del usuario sobre el enfoque.
