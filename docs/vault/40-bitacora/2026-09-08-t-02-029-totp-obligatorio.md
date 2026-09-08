---
fecha: 2026-09-08
fase: "02"
agente: claude-opus-5
---

## Tarea cerrada

- **T-02-029** — TOTP obligatorio para `owner`/`admin` y respaldo por código de
  recuperación (`PR-02-16`).

**El criterio E2E de la fase está en verde**: un owner con TOTP enrolado y la
contraseña **correcta** no obtiene sesión, ni cookie, ni token.

## La decisión que el contrato dejaba abierta

El spec dice *"TOTP obligatorio para `owner`/`admin`"*. Pero el login **todavía
no eligió refugio**, y un rol es propiedad de una membresía — así que no hay
*un* rol que consultar.

**La respuesta: se pregunta sobre TODAS las membresías activas, no sobre una.**

Alguien puede ser `owner` de un refugio y `volunteer` en otro. Pedir el segundo
factor por refugio dejaría entrar sin él y **después intercambiar esa sesión por
el refugio privilegiado** — el intercambio verifica que la membresía esté activa,
no *cómo* se autenticó la sesión. La pregunta correcta es *"¿tiene un rol
privilegiado en algún lado?"*, hecha una sola vez, en la puerta.

## Dos preguntas más que el contrato no cerraba

**Un owner que nunca enroló TOTP: se rechaza.** La otra lectura vuelve el
requisito opcional por inacción, y exime **para siempre** a todo owner creado
antes de que existiera el enrolamiento. El flujo de enrolamiento es lo que los
destraba, no una excepción acá.

**El código de recuperación se chequea ANTES que el TOTP.** Quien manda uno
perdió el autenticador; caer al TOTP lo rechazaría por no producir un código que
no puede producir.

## Cinco tests viejos se rompieron, y el código tenía razón

Los tests de `T-02-028` registran un refugio —lo que convierte a esa persona en
`owner`— y después esperan entrar con contraseña sola. **Eso ahora está
prohibido: es el criterio E2E funcionando.**

Así que cambié la **fixture**, no las cinco aserciones. `seedAccount` ahora crea
un usuario **sin membresía** — que es lo que es un adoptante, y lo que esos tests
siempre quisieron decir. La forma privilegiada vive en `login_totp_test.go`,
donde es el sujeto y no el decorado.

> Corregir cinco aserciones habría sido tratar el síntoma. La fixture era la que
> mentía sobre a quién representaba.

## Los dos mutantes que sobrevivieron eran el mismo defecto

Condiciones **escritas en el código, declaradas en su comentario, y afirmadas en
ningún lado**.

### M6 — `admin`

Sacar `admin` del conjunto privilegiado **no cambió nada**: todos mis casos
usaban `owner`. El spec nombra los dos roles en la misma frase.

> Una regla que se hace cumplir para uno de los dos roles que nombra tiene un
> agujero exactamente donde está el segundo nombre.

Agregué el caso de `admin` **y el de `volunteer`** — sin este último, el arreglo
podría haber sido "toda membresía es privilegiada", y pedirle una app de
autenticación a alguien que pasea perros cuesta más adopciones de las que
protege.

### M4 — `active`

Escribí `Status == "active"` y lo expliqué en el comentario, pero **ningún test
tenía una membresía no activa**.

No es teórico: una invitación `invited` que exija segundo factor **deja afuera a
alguien por un rol que todavía no tiene** — una denegación de servicio que
cualquiera dispara mandando una invitación. Una `revoked` es lo mismo desde el
otro extremo.

## Mutación

Siete mutantes, siete muertos (M4 y M6 tras agregar sus tests):

| Mutante | Test que lo mató |
|---|---|
| `owner` no necesita segundo factor | `AnOwnerWithTOTPEnrolledCannotLogin...` |
| No verifica el código TOTP | `AnOwnerWithAValidTOTPCodeGetsASession` |
| `amr` dice `otp` sin segundo factor | `AUserWithNoPrivilegedMembership...` |
| El código de recuperación no se gasta | `ARecoveryCodeIsSpentByUsingIt` |
| Se saltea la rama de TOTP | `AnOwnerWithAValidTOTPCodeGetsASession` |
| Cuenta membresías no activas | `AnUnacceptedInvitation...` ← **agregado** |
| `admin` no necesita segundo factor | `AnAdminIsAsPrivilegedAsAnOwner` ← **agregado** |

Cero residuo, verificado con `diff` y `sha256`.

## Una nota sobre las fixtures

Las dos brechas eran invisibles por la misma razón: **el helper solo sabía
construir owners.** Generalizarlo a `grantMembership(role, status)` es lo que
volvió baratos los dos tests nuevos.

> La fixture es el límite de lo que un test **puede** decir. Un helper que solo
> construye una forma produce una suite que solo prueba esa forma, y el hueco no
> se ve leyendo los tests — se ve mutando el código.

## Presupuesto

| | implementación | total |
|---|---:|---:|
| límite de `PR-02-16` | 250 | 800 |
| `est:` de la tarea | 140 | — |
| **`T-02-029` medido** | **115** | **662** |

Entra en los dos sin excepción. La implementación es chica porque las piezas
—`totp.go`, `recovery.go`, `envelope.go`— ya existían y estaban probadas: esta
tarea es **cableado y decisiones**, no criptografía.

## Verificación

- suite completa en contenedor, **`exit=0` capturado**, siete paquetes verdes
- **18 tests de login** nombrados en la salida `-v`, cero fallos
- `golangci-lint` sobre `internal/httpapi` — 0 issues
- `gofmt -l` — limpio, verificado en el contenedor

## Siguiente

`T-02-030` — handler de magic link (`PR-02-16b`), donde la
**indistinguibilidad** entre una dirección conocida y una desconocida es
requisito explícito del spec.
