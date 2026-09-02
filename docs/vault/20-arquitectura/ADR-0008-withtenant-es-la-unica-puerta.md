# ADR-0008 — `WithTenant` es la única puerta: el alcance del tenant vive en la transacción

- **Fecha:** 2026-09-01
- **Estado:** aceptado
- **Fase:** 01

## Contexto

[[ADR-0002]] dice que la capa 2 de la defensa es *"cada transacción abre fijando el `shelter_id`
del tenant"*. Eso deja sin decidir **dónde** se fija esa variable, y las opciones no son
equivalentes.

La aplicación usa un **pool** de conexiones. Una conexión se presta, se usa y se devuelve, y la
siguiente petición —de **otro refugio**— puede recibir esa misma conexión. Cualquier estado que
sobreviva a la devolución es estado que cruza tenants.

Y hay dos hechos que decidieron la forma exacta:

1. **`SET LOCAL` es una sentencia de utilidad y no acepta parámetros de vínculo.** Usarla obliga a
   interpolar el UUID dentro del texto del SQL. Hoy ese UUID viene de un claim validado, pero la
   forma insegura se copia hacia adelante.
2. **Una GUC revertida al COMMIT vuelve a la cadena VACÍA, no a NULL.** Así que
   `current_setting('app.shelter_id', true)::uuid` **explota** con `22P02` en vez de devolver
   NULL, y una política escrita de esa forma falla ruidosamente en la petición equivocada.

## Decisión

**Todo acceso a la base pasa por `WithTenant`, que abre una transacción, fija el alcance como
GUC LOCAL A ESA TRANSACCIÓN, y lo devuelve al pool sin nada pegado.**

```go
SELECT set_config('app.shelter_id', $1, true)
```

La función —no `SET LOCAL`— porque **sí** acepta parámetros de vínculo. El tercer argumento
`true` significa "local a la transacción actual", que es exactamente la semántica que se quería.

Y toda política del esquema lee esa GUC con la misma forma, sin excepción:

```sql
shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid
```

El `nullif(..., '')` es obligatorio, no defensivo: convierte la cadena vacía que deja el COMMIT en
NULL, y `shelter_id = NULL` es NULL — o sea, **falso**. Una consulta fuera de alcance ve **cero
filas**, que es exactamente lo que [[ADR-0002]] promete.

**El `shelter_id` sale del claim del token y de ningún otro lado.** Nunca de la URL, nunca de un
header.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| Fijar la GUC al abrir la conexión (`AfterConnect` del pool) | El alcance sobrevive a la petición. La conexión vuelve al pool con el refugio anterior pegado, y la siguiente petición lo hereda. Es la forma que convierte un pool en una fuga. |
| `SET` de sesión en vez de `SET LOCAL` / `set_config(..., true)` | Lo mismo, explícito. |
| `SET LOCAL app.shelter_id = '<uuid>'` | Correcto en alcance, **inseguro en forma**: no acepta bind, así que obliga a interpolar. Corregido en [[ADR-0002]] el 2026-08-29. |
| Pasar el `shelter_id` a cada query como parámetro y filtrar en el SQL | Es la disciplina que [[ADR-0002]] existe para eliminar. Ver la consecuencia de abajo. |

## Consecuencias

**A favor**

- El alcance **no puede** sobrevivir a la transacción. No es una convención: es la semántica de
  `set_config(..., true)`.
- Una consulta sin alcance ve cero filas en vez de las filas equivocadas.
- El UUID viaja como parámetro. La forma segura es la única forma disponible.

**En contra, y se acepta**

- **Todo acceso necesita una transacción**, incluso una lectura de una sola fila. Es una ida y
  vuelta más y un poco de ceremonia en cada call site.
- **La firma manda un `pgx.Tx`, no un pool.** Por eso `sqlc` se configura con `sql_package:
  "pgx/v5"`: la salida de `database/sql` no compilaría contra esto.
- **Es una convención que el compilador no hace cumplir.** Nada impide que alguien tome el pool y
  consulte directo. Lo que lo contiene es que sin la GUC la consulta ve **cero filas** — el
  descuido falla ruidosamente y a favor del aislamiento, que es la propiedad que se compró.

**La consecuencia que decide cómo se escriben las queries, y se ve como estilo:** una query
**no filtra por `shelter_id`**. La política lo hace. Una que filtre ella misma devuelve **lo
mismo** exista o no la política — así que el día que una política se caiga, esa query sigue dando
la respuesta correcta y la pérdida se vuelve **invisible**. El cinturón tiene que ser la capa que
no se puede olvidar, y los tiradores no pueden tapar su ausencia. Los `INSERT` están exentos:
proveer una columna `NOT NULL` no es filtrar por ella.

## Condiciones de reapertura

1. Que aparezca un camino de acceso que legítimamente no sea por tenant — el panel de superadmin
   de la Fase 10, por ejemplo. Necesita su propio rol y su propia puerta, no una excepción a
   ésta.
2. Que `refresh_tokens` (Fase 02) elija scopear por usuario en vez de por refugio. Ahí conviven
   dos GUCs o hay un rol dedicado; la decisión es de esa fase y está anotada como pregunta
   abierta.
3. Que el costo de una transacción por lectura se vuelva medible bajo el arranque en frío de
   Neon. Hoy no lo es.
