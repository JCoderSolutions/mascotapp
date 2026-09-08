---
type: convention
score: 5
topic_key: mascotapp/convention/fixtures-bound-what-tests-can-assert
task: T-02-029
status: guardado
aprobado: 2026-09-08 por el usuario
observation_id: obs-2e0880f4d065d81a
rationale: "Explica por qué una suite puede verse completa y tener huecos sistemáticos, y da la forma de encontrarlos sin leer todos los tests. Los dos mutantes que sobrevivieron en T-02-029 no eran descuidos independientes: eran la misma causa —un helper que solo construía una forma— y ninguna revisión los habría visto, porque cada test individual está bien escrito. Aplica a cada archivo de tests que se agregue de acá en adelante, y la pregunta que propone (¿qué formas NO puede construir esta fixture?) es de treinta segundos."
---

# La fixture es el límite de lo que un test puede decir

(T-02-029, MascotApp Fase 02.)

## La regla

Un helper que solo sabe construir **una** forma produce una suite que solo prueba
esa forma. Los huecos que eso deja **no se ven leyendo los tests**: cada uno está
bien escrito y afirma exactamente lo que dice. Se ven mutando el código.

## El caso

`T-02-029` hace obligatorio un segundo factor para roles privilegiados. Ocho
tests, escenarios positivos y negativos, criterio E2E cubierto. **Dos mutantes
sobrevivieron**, y no eran descuidos independientes: eran **la misma causa**.

El helper construía usuarios registrando un refugio, y registrar hace `owner`.
Así que **todo caso privilegiado del archivo era un `owner` con membresía
`active`** — no por decisión, sino porque era la única forma que la fixture sabía
producir.

| Mutante | Qué revela |
|---|---|
| Sacar `admin` del conjunto privilegiado | ningún test tenía un `admin` |
| Ignorar `Status == "active"` | ningún test tenía una membresía `invited` o `revoked` |

Las dos condiciones estaban **escritas en el código y explicadas en su
comentario**. Ninguna estaba afirmada.

## Por qué importa que fueran la misma causa

Arreglarlas una por una habría dejado la causa en pie, y el próximo hueco
aparecería igual. Lo que las cerró fue **generalizar el helper**:

```go
// antes: solo owners activos, por construcción
func registerOwner(...) uuid.UUID

// después: la forma es un parámetro
func grantMembership(t, env, userID, role, status string)
```

Con eso los dos tests nuevos costaron cuatro líneas cada uno.

## La trampa

Una fixture cómoda **parece** una buena abstracción. Reusar el handler de
registro para sembrar usuarios es correcto por otro motivo —acopla los tests al
almacenamiento real de credenciales, que es deseable— y **por eso mismo nadie la
cuestiona**.

El problema no es que la fixture esté mal. Es que **decide en silencio el
universo de casos**, y esa decisión no aparece en ningún test.

## La pregunta que lo detecta

> **¿Qué formas NO puede construir esta fixture?**

Hecha al escribir el helper, cuesta treinta segundos. Cada respuesta es una
región del espacio de estados que la suite no va a tocar, y hay que decidir a
propósito si eso está bien.

## El corolario que apareció en la misma tarea

Cinco tests de la tarea anterior **se rompieron** al llegar esta, y el código
tenía razón: sembraban con registro —lo que hace `owner`— y después esperaban un
login con contraseña sola, que ahora está prohibido.

La corrección fue **cambiar la fixture, no las cinco aserciones**. Las aserciones
siempre quisieron decir "un usuario común"; la fixture era la que mentía sobre a
quién representaba.

> Cuando varios tests fallan juntos por un cambio legítimo, sospechar de lo que
> comparten antes que de lo que afirman.

## Dónde más aplica

- `dbtest.Postgres` y los helpers de `rlstest`: qué formas de tenant no saben armar
- toda suite donde los casos privilegiados y los comunes vienen del mismo constructor
- fixtures que siembran por la ruta feliz — solo prueban estados alcanzables por
  esa ruta, y los estados heredados o corruptos quedan fuera por construcción
