---
type: convention
score: 3
topic_key: mascotapp/convention/mutation-testing-equivalent-mutants
task: T-02-032
rationale: "Complementa la disciplina ya establecida de chequear el build antes que FAIL/ok (T-02-030): un mutante que sobrevive no es automaticamente un hueco de test, y declarar 'equivalente' sin prueba es tan descuidado como declarar 'muerto' sin chequear el build."
---

Un mutante que sobrevive a la suite existente no es automáticamente un hueco
de cobertura: puede ser un **mutante equivalente**, uno cuyo cambio nunca
produce una respuesta observable distinta, sin importar el input. Confundir
"sobrevivió" con "hace falta un test" desperdicia esfuerzo escribiendo un test
que no puede pasar de otra forma que no sea trivialmente.

Encontrado en `T-02-032`: quitar el chequeo `err != nil` después de
`envelope.Open()` en `TOTPVerifyHandler` sobrevivió a los ocho tests
existentes. La primera lectura fue "hace falta un test que corrompa el
secreto sellado" — y ese test se escribió y también pasó con el mutante
activo. La razón: **todo** `return` de `envelope.Open()` en caso de error
devuelve `secret = nil`, y `VerifyTOTP` ya rechaza cualquier secreto cuya
longitud no sea exactamente `TOTPSecretLength`. No existe ningún input que
distinga las dos versiones del código.

**Regla**: declarar un mutante "equivalente" exige leer el código que hace
la comprobación downstream (acá, cada `return` de `Open()` y el guard de
longitud en `VerifyTOTP`) y confirmar que NINGÚN camino puede producir una
diferencia observable — no basta con que un test particular no lo haya
matado. La misma disciplina que ya existe para no confundir un build roto
con un mutante muerto (`T-02-030`) aplica acá en la dirección contraria: no
confundir "ningún test lo mató" con "no se puede matar".
