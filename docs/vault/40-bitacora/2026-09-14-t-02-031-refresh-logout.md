---
fecha: 2026-09-14
fase: "02"
agente: claude-opus-5
---

## Tarea cerrada

- **T-02-031** — handlers de refresh y logout (`PR-02-17`). Cablean la rotación
  de `session.go`, la que pasó por Judgment Day en `T-02-021`.

## Una decisión tuya: el `amr` de un refresh es `[pwd]`

`refresh_tokens` no guarda **cómo** se autenticó la sesión. Un owner entra con
contraseña + TOTP, y a los 15 minutos el refresh emite un token sin saber que hubo
TOTP. El spec no dice nada.

Preservar el `otp` pedía una migración — y además hacía que **una cookie de
refresh robada se lleve el segundo factor puesto** durante toda la vida de la
familia. Con `[pwd]`, un refresh nunca afirma un factor que no verificó; el ladrón
queda con un token que toda ruta que exija `otp` rechaza.

Lo elegiste el 2026-09-14. Hay un test que lo fija, y su cuenta es a propósito un
**owner que entró con un código TOTP válido**: el caso donde preservarlo es
tentador.

## El orden del chequeo se prueba sin espía

Refresh y logout son las únicas dos rutas a las que viaja la cookie, y viaja con
`SameSite=None`. El chequeo de origen envuelve cada handler **entero**, y el orden
es observable:

- origen rechazado + cookie malformada → **403**, no 401. Si diera 401, la cookie
  se habría parseado antes que el origen.
- origen rechazado + cookie **válida** → la cookie sigue sirviendo después. Si la
  rotación hubiera corrido, estaría gastada.

## Logout revoca la familia, no la fila

Salió pensando el GREEN, y fue a test **antes** de ir a código.

La rotación deja una sola fila viva por familia, y casi nunca es la que apunta una
cookie vieja. Revocar solo la fila presentada no hace nada con una cookie vieja
—y el sucesor, que es lo que abre la sesión, sobrevive al logout. El mismo hueco
aparece **sin ninguna cookie vieja** si un refresh está en vuelo mientras el
usuario cierra sesión.

Revocar la familia **bajo el lock de familia** cierra los dos. El lock no es
decorado: Judgment Day mostró que un revoke de familia sin lock fija sus filas en
su propio snapshot y deja vivo a un sucesor insertado después.

## Dos correcciones a trabajo anterior

**`serverError` decía "registration failed" para todo el mundo.** Los 500 de
**login** vienen respondiendo y logueando eso desde `T-02-028`, y refresh y logout
lo iban a heredar. Verifiqué que ningún test dependiera del texto antes de
cambiarlo: ahora es genérico y el log lleva el path.

**Los atributos de la cookie vivían solo en login.** Ahora están en
`setRefreshCookie`, que usan login y refresh. Las dos rutas que setean esta cookie
ya no pueden desincronizarse.

## El mutante combinado que escondía una cláusula sin probar

La rama que convierte dos errores en 401 —prefijo de uuid cero, y rotación
concurrente perdida— se mutó como un solo mutante, y sobrevivió.

El arreglo obvio era el test del uuid cero, porque es **determinístico**:
dieciséis bytes en cero son un uuid válido y `ParseRefreshCookie` lo acepta sin
chistar. Ese test **habría matado al mutante combinado** — y dejado la cláusula de
la carrera tan sin probar como antes, debajo de un reporte verde.

> Un mutante que saca dos cláusulas a la vez pregunta *"¿alguna tiene test?"*. La
> pregunta útil era *"¿cada una tiene test?"*.

Lo partí en M7a y M7b, cada uno con su test. El de la carrera espeja el test
concurrente de `auth`: pruebas repetidas, seis requests retenidos en una línea de
largada y soltados juntos. La aserción no es quién gana: es que **ninguna
respuesta sea 500** y que **exactamente una** prospere.

Y como esa muerte depende de que la carrera ocurra, **M7b se corrió dos veces**.

## Mutación

Nueve mutantes, nueve muertos:

| Mutante | Test que lo mató |
|---|---|
| Refresh sin chequeo de origen | `ChecksTheOriginBeforeTheCookie` |
| El rechazo viaja como error (rollback de la revocación) | `ReusingASpentCookieRevokesTheWholeFamily` |
| Refresh preserva el `otp` | `NeverClaimsASecondFactor` |
| Logout revoca solo la fila presentada | `EndsTheWholeSessionEvenFromAStaleCookie` |
| Logout sin chequeo de origen | `Logout_ChecksTheOriginFirst` |
| El rechazo por reuso dice por qué | `EveryRefusalLooksTheSame` |
| Refresh: uuid cero responde 500 (M7a) | `AZeroUserPrefixIsRefused...` ← **agregado** |
| Refresh: carrera perdida responde 500 (M7b, ×2) | `ConcurrentUsesOfOneCookieNeverAnswer500` ← **agregado** |
| Logout: uuid cero responde 500 (M8) | `AZeroUserPrefixIsRefused...` |

Cero residuo, verificado con `diff` contra copia pristina. El clasificador ya
chequea el build primero, así que ninguna de estas muertes es un falso muerto.

## Presupuesto

| | implementación | total |
|---|---:|---:|
| límite de `PR-02-17` | 250 | 800 |
| `est:` de la tarea | 160 | — |
| **`T-02-031` medido** | **226** | **698** |

Entra en los dos sin excepción. La implementación incluye las dos correcciones a
`login.go` y `auth_handlers.go`.

## Verificación

- suite completa en contenedor, **`exit=0` capturado**, siete paquetes verdes
- los **doce** tests de refresh y logout, y todos los de login con el helper de
  cookie compartido, nombrados en la salida `-v` con cero fallos
- `golangci-lint` sobre `internal/httpapi` — 0 issues
- `gofmt -l` — limpio, verificado en el contenedor

## Siguiente

`T-02-032` — enrolamiento y verificación de TOTP (`PR-02-18`). Es la tarea que
vuelve alcanzable desde HTTP el `totp_secret_enc` que hoy los tests escriben a
mano.
