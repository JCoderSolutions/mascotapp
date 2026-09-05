---
type: architecture
score: 5
topic_key: mascotapp/arch/refusals-with-side-effects
task: T-02-020
status: pendiente-de-aprobacion
relacionado: obs-9e00e3a6413dc7d2
rationale: "Es una regla de diseño, no un bug de una función: el idioma de Go 'devolvé error para rechazar' y el idioma transaccional 'rollback ante error' son correctos por separado y se destruyen entre sí en el único caso donde un rechazo tiene que ESCRIBIR. Acá el costo era la contención de un token robado — el ladrón era rechazado y se quedaba con la sesión viva, o sea el resultado exacto que la revocación de familia existe para impedir. Y no lo encontró la mutación ni una revisión: lo encontró el primer GREEN, porque el RED había escrito el escenario completo del robo en vez de solo 'devuelve error'. Dentro de tres meses, cualquier flujo que audite o marque algo mientras rechaza pisa exactamente esta mina."
---

# Un rechazo que escribe no puede viajar como error de un callback transaccional

(T-02-020, MascotApp Fase 02. Relacionado con `obs-9e00e3a6413dc7d2`.)

## Dos idiomas correctos que se destruyen entre sí

1. **Go:** un rechazo se comunica devolviendo un `error`.
2. **Transacciones:** `db.WithAuthUser` hace **rollback** cuando su callback devuelve error
   (`apps/api/internal/db/auth.go:68`, `:77`). Es lo correcto: un fallo no debe dejar
   escrituras a medias.

Los dos son buenos. Juntos rompen el único caso donde **un rechazo tiene que escribir**.

## El caso concreto

La detección de reutilización de refresh tokens: si el token presentado ya fue rotado, se
**revoca toda su familia** y se rechaza la petición.

```go
// La primera versión, y estaba mal:
if presented.RevokedAt.Valid {
    queries.RevokeRefreshTokenFamily(ctx, presented.FamilyID)  // escribe
    return ErrRefreshTokenReused                               // ...y el rollback la borra
}
```

El error sube al callback, `WithAuthUser` hace rollback, y **la revocación se deshace**. El
ladrón recibe un 401 y **conserva su sesión viva** — que es exactamente el resultado que la
revocación de familia existe para impedir.

No falla nada. No hay excepción, no hay log, la función "funciona": rechaza. Solo que la
contención nunca ocurrió.

## La regla

**Si un rechazo tiene un efecto persistente, el rechazo no puede ser el error de la
transacción.** Hay que separar dos cosas que Go normalmente mezcla:

- **fallo de infraestructura** → `error` → rollback, correcto
- **resultado del negocio, incluido el rechazo** → valor de retorno → commit

```go
type RotationOutcome struct {
    Cookie  string  // vacío salvo éxito
    Refusal error   // por qué es 401, o nil
}

func RotateRefreshToken(...) (RotationOutcome, error)  // el error es SOLO infraestructura
```

El llamador devuelve al callback **únicamente** el error de infraestructura, así que el
rechazo **commitea** con su revocación adentro.

Y hay que decir cuál rechazo es la excepción: perder una carrera de rotación concurrente sí
usa el `error`, porque ahí ya se insertó la fila sucesora y **hay** que deshacerla. La regla no
es "los rechazos nunca son errores", es **"un rechazo commitea si y solo si dejó algo escrito
que tiene que sobrevivir"**.

## Lo que hay que llevarse, más allá de este archivo

Cualquier flujo que **audite, marque o contenga mientras rechaza** tiene esta forma:

- registrar un intento de login fallido,
- marcar una cuenta como bloqueada al enésimo fallo,
- escribir una entrada de auditoría del acceso denegado,
- consumir un intento de un rate limiter.

Todos escriben en el camino del "no". Todos se pierden si el "no" viaja como error de la
transacción.

## Cómo apareció, que importa tanto como el qué

**Lo encontró el primer GREEN, no la mutación y no una revisión.** Y solo porque el test RED
había escrito el **escenario completo del robo** —rotar, presentar la copia vieja, y después
verificar que el token que el usuario legítimo tenía en la mano quedó revocado— en vez de
limitarse a "devuelve `ErrRefreshTokenReused`".

Un test que solo aserta el error habría pasado con el bug puesto. La aserción que lo mató es
la del **estado después**, y estaba ahí porque el spec la pedía como escenario propio.

## La otra cara de `obs-9e00e3a6413dc7d2`

Un PR antes, en `recovery.go`, **el mismo rollback volvió equivalente a un mutante**: mover
una guarda debajo de un `DELETE` no cambiaba nada, porque el rollback preservaba las filas.
Ahí el rollback nos salvaba.

Acá nos destruye. **Es la misma capa contestando, y no es ni amiga ni enemiga** — es una capa
que hay que tener en el modelo mental cuando se razona sobre qué queda escrito. Las dos veces
la pregunta útil fue la misma: *¿qué sobrevive al final de esta transacción, y quién lo
decidió?*
