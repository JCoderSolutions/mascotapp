---
type: convention
score: 5
topic_key: mascotapp/convention/relational-assertions-need-a-content-assertion
task: T-02-028
status: guardado
aprobado: 2026-09-08 por el usuario
observation_id: obs-fb6503b1977f61ad
rationale: "Es un modo de falla de los TESTS, no del código, así que se cuela por debajo de toda revisión que mire la implementación. Y aparece justo donde más duele: en las propiedades que se enuncian como igualdad — indistinguibilidad, idempotencia, determinismo, round-trips — que son casi siempre propiedades de seguridad. Este proyecto tiene por delante tres endpoints más que la necesitan (magic link, refresh, rate limiting), y en el magic link la indistinguibilidad es requisito explícito del spec. La lección se aplica al escribir el test, que es cuando todavía es gratis."
---

# Un test que compara dos salidas entre sí no fija ninguna de las dos

(T-02-028, MascotApp Fase 02.)

## La regla

Cuando un test afirma que dos salidas son **iguales**, esa afirmación por sí sola
no fija ninguna de las dos. Hace falta **una segunda aserción sobre el
contenido** de al menos una.

## El caso

Login tiene que ser indistinguible entre una dirección que no existe y una
contraseña equivocada. El test se escribe solo:

```go
wrongPassword := postLogin(t, handler, known, "not-the-right-password")
unknownEmail  := postLogin(t, handler, "nobody@refugio.test", password)

if wrongPassword.Body.String() != unknownEmail.Body.String() { ... }
```

Compara estado, cuerpo y **todas** las cabeceras. Parece exhaustivo.

**El mutante que cambia el mensaje a `"no account with that email"` sobrevivió.**

## Por qué

Los dos caminos pasan por **una sola función** — `unauthorizedLogin`. Cambiarla
cambia los dos a la vez, así que las dos respuestas siguen siendo byte a byte
idénticas, y el test pasa mientras el handler le dice a **todo el mundo** cuál
chequeo falló.

> El test afirmaba una **relación** y nunca un **contenido**. La relación se
> mantiene invariante ante el defecto, porque el defecto es común a los dos
> lados.

## La trampa

Cuanto **mejor** esté factorizado el código, más probable es el agujero. Una sola
función de rechazo es la forma correcta de escribirlo — y es exactamente lo que
hace que la comparación entre las dos salidas sea insensible al defecto.

Es decir: la refactorización que hace correcto al código es la que vuelve
inservible al test.

## El arreglo

Nombrar explícitamente lo que **no** puede aparecer:

```go
for _, leak := range []string{"email", "account", "password", "user",
                              "exist", "found", "unknown", "incorrect", "wrong"} {
    if strings.Contains(body, leak) { t.Errorf(...) }
}
```

La lista enumerada, no un `!= ""`. Es la misma forma que ya usa el test de fuga
de errores de base de `[[2026-09-08-un-mensaje-de-error-devuelto-al-cliente-es-una-superficie-de-seguridad]]`:
enumerar **cada cosa que el defecto produciría** en vez de describir la ausencia.

## Dónde más aplica

Toda propiedad que se enuncia como igualdad — y casi todas son de seguridad:

- **magic link** (`T-02-030`): la indistinguibilidad es requisito explícito del
  spec, y el mismo test escrito así pasaría con la fuga puesta
- **refresh**: los rechazos por token desconocido y por token ya rotado tienen
  que ser iguales *y* no decir cuál fue
- **rate limiting** (P2-D10): la respuesta al limitado y al no existente
- **idempotencia**: `f(x) == f(f(x))` se sostiene si `f` devuelve una constante
- **round-trips**: `decode(encode(x)) == x` pasa con un códec que no hace nada

La pregunta que lo detecta al escribir el test: **¿existe un defecto que rompa
las dos salidas de la misma manera?** Si existe, la comparación no lo ve, y hace
falta afirmar el contenido.

## Un pariente que no se ve igual

El otro mutante de esta misma tarea —salir temprano cuando la cuenta no existe—
sobrevivió por la razón vecina: las dos respuestas **siguen siendo iguales**, y lo
que difiere es el **tiempo**. Ahí la salida observable no alcanzaba en absoluto,
y hubo que medir el trabajo. Es la misma lección un paso más allá: **la igualdad
de lo que se ve no es igualdad de lo que pasó.**
