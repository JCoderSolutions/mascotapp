---
type: convention
score: 5
topic_key: mascotapp/convention/mutation-runs-need-a-positive-signal
task: T-02-030
status: pendiente-de-aprobacion
extends: obs-296d31be8cc336f7
rationale: "Corrige y afila una regla que ya está en memoria. La entrada anterior avisaba que un mutante que no compila puede leerse como sobreviviente; esta corrida mostró la lectura opuesta y peor — el mismo mutante leído como MUERTO — y dio la cadena exacta que lo causa (`FAIL <pkg> [build failed]`). Dos de siete mutantes eran falsos muertos y ninguno hizo ruido. La asimetría entre los dos errores es lo que decide en qué orden chequear, y sin ella un harness escrito con la mejor intención sigue mintiendo."
---

# Un muerto falso es peor que un sobreviviente falso

(T-02-030, MascotApp Fase 02. Extiende
`[[2026-09-06-un-mutante-que-no-compila-no-es-un-mutante-que-sobrevive]]`.)

## La cadena exacta

`go test` sobre un error de compilación **no** imprime `--- FAIL`. Imprime:

```
# github.com/gentleman/mascotapp/apps/api/internal/httpapi
internal/httpapi/magic_link.go:84:3: declared and not used: credentials
FAIL	github.com/gentleman/mascotapp/apps/api/internal/httpapi [build failed]
FAIL
```

Un clasificador que pregunta `^FAIL` **antes** de preguntar por el build lee eso
como **mutante muerto**. En esta corrida, **dos de siete** lo eran.

## La asimetría, que es lo que decide el orden

La entrada anterior avisaba de la lectura opuesta: un mutante que no compila
tomado por **sobreviviente**. Las dos son errores del mismo origen, y **no
cuestan lo mismo**:

| Error | Qué produce | Cómo se nota |
|---|---|---|
| Sobreviviente falso | te manda a escribir un test que no hacía falta | **ruidoso** — se nota al escribirlo |
| **Muerto falso** | reclama una cobertura que no existe | **silencioso** — nadie vuelve a mirar |

Por eso el chequeo de build va **primero**, no en el `else`. Y el caso por
defecto es `INDETERMINADO`, no una suposición:

```bash
if   echo "$out" | grep -q 'build failed'; then echo "NO_COMPILA"
elif echo "$out" | grep -q '^ok  ';        then echo "SOBREVIVIO"
elif echo "$out" | grep -q -- '--- FAIL';  then echo "MUERTO"
else echo "INDETERMINADO"; fi
```

`--- FAIL` en vez de `^FAIL`: la primera es una línea de test fallado, la segunda
también la imprime el build.

## Por qué los mutantes dejan de compilar tan seguido

No es torpeza: es la forma del mutante. Sacar una condición de un `if` suele
dejar **la variable que esa condición leía** sin usar, y Go lo rechaza.

```go
credentials, found, err := findCredentials(...)
if found && credentials.Status == "active" {   // el original
if found {                                     // el mutante -> credentials sin usar
```

El arreglo es escribir el mutante de modo que **siga usando** lo mismo:

```go
if found && credentials.Status != "" {   // afloja el chequeo Y compila
```

## La trampa

El baseline sin mutar **no detecta esto**. Corre limpio, dice `SOBREVIVIO`, y el
clasificador parece correcto — porque el camino que está roto es el que solo
recorre un mutante que no compila.

Un baseline verifica que el clasificador reconozca el **éxito**. No verifica que
distinga sus dos modos de falla.

## Dónde más aplica

Cualquier arnés que clasifique por texto de salida:

- `ok <paquete>` no prueba que un test nombrado corrió
- un pipe se come el código de salida del comando que importaba
- un `grep` sobre stdout no ve lo que se fue a stderr

Todos son la misma familia: **la ausencia de una señal de error no es una señal
de éxito**, y el orden en que se preguntan las señales decide qué mentira te
cuenta el arnés.
