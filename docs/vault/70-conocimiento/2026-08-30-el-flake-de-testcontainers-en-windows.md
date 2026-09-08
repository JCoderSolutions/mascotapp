---
type: constraint
score: 4
topic_key: mascotapp/ops/testcontainers-windows-provider-race
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-df8668122e55b55d
task: T-01-013
rationale: "El síntoma se lee como una migración rota y no lo es; sin el nombre, cada sesión lo re-diagnostica."
---

# El transitorio de testcontainers en Windows es una carrera en la detección de provider

Síntoma: un **paquete entero** falla a `0.00s`, todos sus tests con el mismo
mensaje:

```
rootless Docker is not supported on Windows, failed to create Docker provider
```

Se lee como una migración rota o un daemon caído. No es ninguna de las dos.

Causa: tres paquetes levantan su propio contenedor, y cuando sus binarios de test
abren el named pipe de Docker Desktop en el mismo instante, la detección de
provider de testcontainers pierde la carrera y concluye "rootless".

## Medido, no supuesto

| | fallos |
|---|---|
| `go test ./...` (paralelo) | **6 de 17** |
| `go test -p 1 ./...` | **0 de 12** |

`make test-api` ahora pasa `-p 1`. El costo es casi nulo: los contenedores ya
dominan el reloj y el módulo tiene cinco paquetes.

## Lo que hay que desaprender

El comentario en `dbtest/container.go` culpaba a la **cantidad de contenedores**
y se anotó el 2026-08-29 como "transitorio inexplicado". Compartir un contenedor
por paquete sigue siendo correcto — pero **no era la cura**, y el comentario
quedó corregido en vez de en pie.

Es el segundo diagnóstico de este proyecto que resultó falso y se corrigió en el
lugar donde estaba escrito, no en un changelog. El otro es el bloqueo de
Application Control, que sigue abierto: ver
[[2026-08-30-el-harness-de-mutacion-que-medía-el-host]].
