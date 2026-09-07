---
fecha: 2026-09-06
fase: "02"
agente: claude-opus-5
---

## Tarea cerrada

- **T-02-024** — GREEN de `middleware_auth.go`. Con esto **`PR-02-12` cierra**, y con él el
  criterio de éxito de la Fase 02.

## Dos middlewares, no uno

`RequireAuth` verifica el bearer y pone los claims en el contexto. `RequireTenant` resuelve el
scope y abre la transacción.

Están separados a propósito: hay rutas **autenticadas y sin refugio** —elegir un refugio, leer tu
propio perfil— y si fueran un solo middleware, esas rutas tendrían que inventarse un tenant para
poder autenticar. Inventar un tenant es exactamente lo que este archivo existe para impedir.

## El orden de los chequeos es el contrato

| # | Chequeo | Fallo |
|---|---|---|
| 1 | ¿Está autenticado siquiera? | **500** — es un bug de cableado |
| 2 | ¿El claim nombra un refugio real? | **403** |
| 3 | ¿Algo en el request lo contradice? | **403** |
| 4 | *Recién ahí* → `WithTenant` | — |

Todo rechazo ocurre **antes** del paso 4, así que un request rechazado no llega a la base.

> Un 403 escrito después de que `WithTenant` ya abrió transacción sobre el refugio del atacante
> es una disculpa, no un rechazo.

El **500** del paso 1 merece una línea: si `RequireTenant` está montado sin `RequireAuth` arriba,
contestar 403 escondería un error de cableado detrás de un rechazo plausible en **todos** los
requests. El 500 dice que el servidor está mal, que es lo que pasa.

## Detalles que se ganaron su comentario

- **`authContextKey` es su propio tipo**, no el `contextKey` de `clientip.go`. Dos bloques `iota`
  sobre un mismo tipo comparten valores en silencio, y la colisión le entregaría el valor de un
  middleware al lector de otro.
- **Un identificador que no parsea es un desacuerdo**, nunca un "no vino ninguno". Lo segundo
  convertiría un path malformado en un bypass del chequeo entero.
- **403 y no 404.** Esconder el recurso convierte este límite en un oráculo de existencia para el
  que adivina bien, y sigue filtrando para el que adivina mal. Lo peor de los dos mundos.
- **`TenantScoper` es una costura, no indirección decorativa.** Evita que `httpapi` importe un
  pool, y es lo que permite que un espía asierte *"nunca invocado"* de forma exacta.

## Un fixture mal, no la implementación

El primer GREEN falló un test. No era el código: era **mi token forjado**, construido con `amr`
vacío, que `Issue` rechaza desde `T-02-014`.

Una falsificación tiene que estar mal en **exactamente una** cosa —la firma— o el test pasa por
el motivo equivocado. Corregido en el test, no en la implementación.

## Mutación — seis mutantes, seis muertos

| Mutante | Lo mató |
|---|---|
| Scope tomado del path cuando el path existe | `AForgedPathShelterIDIsRefused` |
| Scope tomado **solo** del path | `ScopeComesFromTheClaimOnARouteWithNoShelterInThePath` — **y nada más** |
| Fail-open ante un desacuerdo | tres tests |
| Se saca la mitad `uuid.Nil` del guard | `TheZeroShelterUUIDIsNotAShelter` |
| Cualquier esquema de `Authorization` aceptado | el subtest `wrong_scheme` |
| Transacción abierta **antes** del chequeo | la aserción de **"nunca invocado"** |

Dos merecen nombre propio:

**El segundo.** En el RED escribí que *"una implementación que tome el scope del path pasa todos
los demás tests del archivo"*. Eso era una afirmación mía. Ahora está **medida**: bajo ese
mutante, los siete tests restantes pasan y solo cae el de la ruta sin segmento.

**El sexto.** Lo mató la aserción de "nunca invocado", **no** un status code. El status seguía
siendo 403. Esa es, exactamente, la diferencia entre un rechazo y una disculpa.

### Un séptimo intento que no fue mutante

El de "aceptar cualquier esquema" **no compilaba** la primera vez (`scheme` sin usar). No imprimió
`FAIL` ni `ok`, y mi filtro de `rg` no matchea un error de compilación: casi lo cuento como
sobreviviente.

Lo agarré porque el mutante que no imprime **nada** es sospechoso por sí mismo. Reescrito con `_`
corrió y murió. **Un mutante que no compila no es un mutante que sobrevive: es un mutante que
nunca existió.**

## La advertencia de presupuesto no se cumplió, y la dejo escrita

Ayer marqué que `PR-02-12` iba camino a romper los dos límites: `T-02-023` sola medía 445 sobre
un `est:` de 290.

| | implementación | total |
|---|---:|---:|
| límite | 250 | 800 |
| proyección (`est:` × 2,36) | — | 684 |
| **medido** | **227** | **675** |

**Entra en los dos, sin excepción.** Y la proyección total le pegó con 1,3% de error.

La advertencia queda **tachada, no borrada**. Era razonable con la evidencia que había, y el
registro de una alarma **falsa** es lo que mantiene honesta a la próxima. La lección es más
angosta que "dejá de avisar":

> **Un RED que se va a 3× su `est:` no dice nada sobre el GREEN.** El `est:` cuenta superficie, y
> las dos mitades se pasan por motivos que no tienen nada que ver entre sí.

## Verificación

- suite completa en contenedor, **`exit=0` capturado**, siete paquetes verdes
- los **ocho** tests nombrados en la salida `-v` dentro del contenedor
- `golangci-lint` sobre `httpapi` — 0 issues (hubo uno de `revive`: mi comentario de resumen
  quedó pegado al `const` y pasaba por doc comment; se despegó con una línea en blanco)
- `gofmt` limpio · `govulncheck` sin cambios
- cero residuo de mutación, verificado con `diff` byte a byte contra el original

## Lo que este middleware todavía NO hace

**No está montado en el router.** El cableado de `router.go` es `T-02-040`, la última de la fase,
exactamente donde el diseño lo pone. Hasta entonces esto es una garantía probada y no conectada.

## Siguiente

`T-02-025` — `cors.go` + `csrf.go` (`PR-02-13`): allowlist con credenciales y verificación de
`Origin`/`Sec-Fetch-Site`, fail-closed cuando no viene ninguno de los dos.
