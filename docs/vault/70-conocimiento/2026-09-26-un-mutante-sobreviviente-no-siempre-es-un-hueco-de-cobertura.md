---
type: convention
score: 3
topic_key: mascotapp/convention/mutation-testing-equivalent-mutants
task: T-02-032
status: guardado
aprobado: 2026-09-26 por el usuario
observation_id: obs-66a09bc2e7da40bb
rationale: "Complementa la disciplina ya establecida de chequear el build antes que FAIL/ok (T-02-030): un mutante que sobrevive no es automaticamente un hueco de test, y declarar 'equivalente' sin prueba es tan descuidado como declarar 'muerto' sin chequear el build."
---

# Un mutante sobreviviente no siempre es un hueco de cobertura

(T-02-032, MascotApp Fase 02. Pariente en dirección opuesta de
`[[2026-09-14-un-muerto-falso-es-peor-que-un-sobreviviente-falso]]`: esa nota
avisa de no confundir "no compiló" con "murió"; esta avisa de no confundir
"nadie lo mató todavía" con "no se puede matar".)

## La regla

Un mutante que sobrevive a la suite existente no es automáticamente un
hueco de cobertura. Puede ser un **mutante equivalente**: uno cuyo cambio
nunca produce una respuesta observable distinta, sin importar el input.
Confundir "sobrevivió" con "hace falta un test" desperdicia esfuerzo
escribiendo un test que no puede pasar de ninguna otra forma que no sea
trivialmente — porque no existe ningún input real que distinga las dos
versiones del código.

Declarar un mutante "equivalente" exige **leer el código downstream** que
hace la comprobación real y confirmar que ningún camino puede producir una
diferencia observable. No basta con que un test particular no lo haya
matado, ni con la intuición de que "seguro hay algún input".

## El caso

`TOTPVerifyHandler` abre el secreto TOTP sellado con `envelope.Open()` y
chequea el error antes de seguir:

```go
secret, err := envelope.Open(sealed)
if err != nil {
    // ...
}
```

Un mutante que quita el `if err != nil` **sobrevivió** a los ocho tests
existentes. La primera lectura, la intuitiva, fue "hace falta un test que
corrompa el secreto sellado para forzar ese `err != nil`". Ese test se
escribió — y **también pasó con el mutante activo**.

La razón, encontrada leyendo `envelope.go` en vez de seguir adivinando:
**todo** `return` de `Open()` en caso de error deja `secret = nil`. Y
`VerifyTOTP`, más abajo, ya rechaza cualquier secreto cuya longitud no sea
exactamente `TOTPSecretLength` — un `nil` no la tiene. El camino de error
de `Open()` y el guard de longitud de `VerifyTOTP` producen la misma
respuesta observable estén o no el `if err != nil` de por medio. No existe
ningún input, ni siquiera uno malicioso, que distinga las dos versiones.

## Por qué importa

El costo de no hacer esta verificación no es solo el tiempo de escribir un
test inútil: es la falsa sensación de haber cerrado un hueco cuando en
realidad se agregó ruido a la suite. Y en la dirección contraria — declarar
"equivalente" sin leer el código — el costo es peor, porque un hueco real
queda sin test y sin ninguna señal de que sigue abierto, tal como advierte
`[[2026-09-14-un-muerto-falso-es-peor-que-un-sobreviviente-falso]]` sobre
los muertos falsos: la ausencia de una señal de fallo no es una señal de
que todo está cubierto.

La disciplina en las dos direcciones es la misma: **una clasificación de
mutante —muerto, sobreviviente o equivalente— se prueba leyendo código, no
se infiere del resultado de una corrida.**

El check de `err != nil` se mantiene en el código de todas formas, por
intención y defensa en profundidad — un `envelope.Open()` que algún día
cambie su contrato de retorno no debería depender de que nadie lo note. Y
se agregó un test de manipulación (`tamper test`) que fija la promesa
documentada del contrato, sin pretender que ese test mate a este mutante
específico.

## Dónde más aplica

- cualquier chequeo de error que sea redundante con una validación
  posterior más estricta (longitud, formato, rango)
- guards defensivos agregados por claridad de intención en código que ya
  tiene una invariante más fuerte encima
- mutantes que tocan ramas de manejo de error cuyo único efecto observable
  es un valor que otra capa ya rechaza igual

La pregunta que lo detecta: **¿leí cada camino downstream y confirmé que
ninguno puede producir una diferencia observable, o estoy asumiendo que la
tiene porque el mutante sobrevivió?** Si es lo segundo, todavía no se probó
nada — ni que hace falta un test, ni que el mutante es equivalente.
