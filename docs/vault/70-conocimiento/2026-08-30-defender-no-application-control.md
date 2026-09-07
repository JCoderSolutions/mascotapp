---
type: constraint
score: 3
topic_key: mascotapp/ops/testing
task: T-01-015
reviewed: 2026-09-01 (T-01-035, pase de curaduria)
disposition: descartado -- su causa raiz se probo FALSA y el que la corrige ya esta guardado
superseded_by: 2026-08-31-smart-app-control-no-es-defender.md  # causa raiz FALSA: era Smart App Control, no Defender ni el scratch
rationale: "Tres diagnósticos fallidos porque el mensaje genérico no nombraba al componente; el cuarto sí, y es accionable."
---

# El bloqueo de binarios de test es **Windows Defender**, no una política opaca

Durante tres tareas el síntoma fue:

```
An Application Control policy has blocked this file
```

Un mensaje que no nombra nada. Produjo tres diagnósticos, los tres falsos:
vaciar `GOTMPDIR`, la cantidad de contenedores, y "es determinista por contenido
del binario".

En T-01-015 apareció la otra cara del mismo bloqueo:

```
Operation did not complete successfully because the file contains a virus
or potentially unwanted software
```

Eso sí nombra al componente: **Defender pone en cuarentena el binario de test
recién linkeado.** Go escribe un ejecutable nuevo, sin firmar, en cada corrida.

## Lo que se puede hacer, y lo que no

- **Lo que arregla:** una exclusión de Defender para la salida de build. Es del
  usuario, no del proyecto.
- **Lo que no:** reintentar, cambiar `GOTMPDIR`, o teorizar. Bloqueó el mismo
  binario 6 veces seguidas desde las dos ubicaciones.
- **El workaround que sirvió:** `go test -c -o <scratchpad>/x.exe` y ejecutar el
  binario directo.

## La lección de proceso

Un mensaje de error genérico se llevó tres diagnósticos. **Cuando el síntoma no
nombra un componente, la primera prioridad es conseguir un mensaje que sí lo
nombre** — no proponer una causa. Cada teoría que se anota como conclusión hay
que borrarla después, y la de al lado empieza a leerse con menos confianza.
