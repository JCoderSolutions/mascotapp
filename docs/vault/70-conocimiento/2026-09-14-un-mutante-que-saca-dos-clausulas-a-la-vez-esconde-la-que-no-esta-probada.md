---
type: convention
score: 4
topic_key: mascotapp/convention/mutate-one-clause-at-a-time
task: T-02-031
status: guardado
aprobado: 2026-09-24 por el usuario
observation_id: obs-a13da1b17fdd6562
rationale: "Es un modo en que un reporte de mutación verde miente sin que ningún mutante esté mal clasificado. El arnés funciona, el test es real, el mutante muere — y aun así una cláusula queda sin probar. Aparece cada vez que una condición tiene más de un término, que es casi siempre en los handlers de este proyecto (errores que se tratan como rechazos, roles privilegiados, estados activos). La regla es barata de aplicar al escribir el mutante y cara de descubrir después, porque el reporte no da ninguna pista."
---

# Un mutante que saca dos cláusulas a la vez esconde la que no está probada

(T-02-031, MascotApp Fase 02. Pariente de
`[[2026-09-14-un-muerto-falso-es-peor-que-un-sobreviviente-falso]]`.)

## La regla

Cuando una condición tiene más de un término, se muta **una cláusula por vez**.
Un mutante que las saca todas juntas muere en cuanto **cualquiera** tiene test, y
deja las otras sin probar debajo de un reporte verde.

## El caso

El handler de refresh convierte dos errores en rechazos 401:

```go
if errors.Is(err, auth.ErrRefreshTokenInvalid) || errors.Is(err, db.ErrNoAuthUser) {
    refuseRefresh(w)
    return
}
```

- `ErrNoAuthUser`: una cookie cuyo prefijo es el uuid cero. Dieciséis bytes en
  cero son una codificación válida, así que `ParseRefreshCookie` la acepta. Es
  **alcanzable de forma determinística** desde HTTP.
- `ErrRefreshTokenInvalid`: una rotación concurrente perdida. Solo aparece bajo
  **concurrencia real**.

El primer mutante reemplazó la condición entera por `if false`. Sobrevivió, y el
arreglo obvio era escribir el test del uuid cero — que es fácil y determinístico.

**Ese test habría matado al mutante combinado.** Y la cláusula de la carrera
habría quedado exactamente igual de sin probar que antes, con un reporte que decía
"todos los mutantes muertos".

## Por qué el reporte no avisa

Nada está mal clasificado. El arnés chequea el build primero, el test es real, el
mutante muere de verdad. La mentira no está en ninguna pieza: está en **qué
pregunta hizo el mutante**. Un mutante combinado pregunta *"¿alguna de estas
cláusulas tiene test?"*, y la respuesta útil era *"¿cada una tiene test?"*.

## El arreglo

Partir el mutante, una cláusula por vez:

| Mutante | Qué saca | Qué lo mata |
|---|---|---|
| M7a | solo `ErrNoAuthUser` | test determinístico del uuid cero |
| M7b | solo `ErrRefreshTokenInvalid` | test de carrera con N requests simultáneos |

Y la carrera obligó a una segunda disciplina: **una muerte que depende del tiempo
se corre más de una vez**. M7b se corrió dos veces. Un solo "MUERTO" en un camino
dependiente del tiempo prueba tan poco como un solo intento de provocar la
carrera.

## La trampa

La cláusula fácil de testear **tapa** a la difícil. La determinística se prueba
primero porque es barata, mata al mutante combinado, y la concurrente —la que más
cuesta y la que más falta hace— nunca llega a escribirse.

## La pregunta que lo detecta

Al escribir un mutante sobre una condición:

> **¿Cuántos términos tiene, y este mutante saca uno o todos?**

Si saca todos, el resultado solo dice algo sobre el término mejor testeado.

## Dónde más aplica

- `found && credentials.Status == "active"` en login y magic link
- `membership.Status == "active" && rolesRequiringTOTP[membership.Role]` — el
  caso de T-02-029, donde `admin` y `active` sobrevivieron por separado
- toda condición `err == A || err == B` que traduzca errores a respuestas
