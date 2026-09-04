# Desviación de P2-D8: TOTP sobre la biblioteca estándar, no `github.com/pquerna/otp`

> Fecha: 2026-09-04 · Tarea: `T-02-013` · PR: `PR-02-08b`
> Estado: **aplicada**. Revierte una decisión del diseño P2-D8 de la Fase 02.

## Lo que decía el diseño

P2-D8 eligió `github.com/pquerna/otp` para el segundo factor, y registró explícitamente
la alternativa descartada:

> **Rejected:** hand-rolled HMAC (crypto you write is crypto you maintain forever).

Ese argumento es correcto como regla general. **No aplica a este caso concreto**, y por eso
se revierte.

## Por qué se revierte

### 1. No hay mantenimiento que heredar

El argumento "la cripto que escribís es cripto que mantenés para siempre" supone un
algoritmo que evoluciona: parámetros que se endurecen, ataques que aparecen, primitivas
que se deprecian. Argon2id es así — por eso `password.go` **sí** usa una dependencia.

TOTP no. RFC 6238 se publicó en 2011 y **no ha cambiado desde entonces**, y no puede
cambiar: el valor del estándar es que millones de aplicaciones autenticadoras ya
implementan exactamente eso. Un cambio en el algoritmo rompería todas. El flujo de
mantenimiento que la regla anticipa no existe acá.

### 2. La corrección queda anclada fuera de este repositorio

El Apéndice B de RFC 6238 publica vectores de prueba: una clave conocida, cinco instantes,
y los códigos exactos que un implementador correcto debe producir. `totp_test.go` los
fija, y sus valores esperados **se computaron** con un programa aparte de `crypto/hmac` +
`crypto/sha1` antes de escribir la implementación — no se recordaron.

Eso es evidencia más fuerte que la suite de tests de una dependencia, porque no la escribió
la misma persona que escribió el código, y porque mide interoperabilidad con la app que el
usuario tiene en el bolsillo, no consistencia interna.

La mutación lo confirmó: al fijar el offset de truncación dinámica en cero, los cinco
vectores fallan y el guard de anti-vacuidad de `RejectsACodeFromAnotherSecret` dispara
primero. Al ampliar la ventana de tolerancia de uno a dos pasos, fallan **exactamente** los
dos casos que reclaman esa propiedad, y ninguno más.

### 3. El costo de la dependencia es asimétrico y cae en el peor paquete

`github.com/pquerna/otp` no estaba en el módulo caché, así que adoptarla significaba un
`go get` — una acción de cadena de suministro, gateada por la lista `ask` de §7.3.

Lo que se compara no es "50 líneas contra cero líneas". Es:

| | Escribirlo | Depender |
|---|---|---|
| Costo | 192 líneas bajo la revisión de este proyecto | superficie de cadena de suministro permanente |
| Dónde cae | `internal/auth` | `internal/auth` |
| Si sale mal | un bug que los vectores del RFC detectan | compromiso irrecuperable del segundo factor |
| Quién lo revisa | Judgment Day antes del merge | nadie de este proyecto |

En el paquete de autenticación esa asimetría decide. Una dependencia comprometida ahí no
degrada el sistema: lo entrega.

## Lo que NO justifica esta desviación

Que sea poco código. **El tamaño no es el argumento** — Argon2id también son pocas líneas
de llamada y sigue siendo una dependencia, correctamente. Lo que decide es la combinación
de las tres condiciones de arriba, y las tres tienen que darse:

1. el estándar está congelado,
2. hay vectores de prueba publicados y externos que fijan la corrección,
3. la dependencia cae en un paquete donde un compromiso no se recupera.

**Si alguna de las tres falta, la regla original de P2-D8 gana.** Esta desviación no
autoriza reimplementar criptografía en general, y no debe citarse como precedente para
JWT (`T-02-014`), donde la condición 1 no se cumple: JOSE tiene familias de algoritmos,
modos de confusión conocidos, y una historia activa de vulnerabilidades de implementación.
Ahí se usa `golang-jwt/v5` con HS256, como fija P2-D11 (`design.md:421`).

## Lo que se implementó

`apps/api/internal/auth/totp.go` — 192 líneas, `crypto/hmac` + `crypto/sha1` +
`crypto/subtle` + `crypto/rand` + `encoding/base32` + `net/url`. Cero dependencias nuevas:
`go.mod` y `go.sum` sin modificar.

Tres decisiones dentro de la implementación que valen registrar:

- **La ventana de tolerancia es de un paso, y el bucle no corta al encontrar coincidencia.**
  Cortar temprano haría que el tiempo de ejecución revelara *cuál* paso coincidió, que es
  un oráculo de reloj. Se comparan los tres candidatos siempre, con
  `subtle.ConstantTimeCompare`.
- **`VerifyTOTP` devuelve `bool`, no `error`.** Distinguir *por qué* falló le diría a un
  atacante si el secreto existe, si tiene el largo correcto, o cuán desviado está su reloj.
  No hay ningún llamador legítimo que necesite la distinción.
- **Un código de seis dígitos se valida por rango de bytes ASCII, no con `unicode.IsDigit`.**
  `０８１８０４` son seis dígitos para Unicode y no son lo que produce ninguna app
  autenticadora.

## Lo que esto NO resuelve

`VerifyTOTP` **no** hace que un código sea de un solo uso. Un código presentado dos veces
dentro de su propia ventana se acepta dos veces. Rechazar eso necesita un registro de
códigos gastados, y ese registro es responsabilidad del handler de login — queda pendiente
para `PR-02-11`. Está escrito en el comentario de la función para que no se asuma resuelto.

## Referencias

- RFC 6238 §4, Apéndice B — el algoritmo y los vectores
- RFC 4226 §5.3 — truncación dinámica; §4 — longitud recomendada del secreto (160 bits)
- [[ADR-0001-stack]] — la elección original de stack
- `design.md:421` (P2-D11) — la decisión de JWT, que esta desviación **no** toca
- `openspec/changes/phase-02-auth-and-multitenancy/design.md` — P2-D8
