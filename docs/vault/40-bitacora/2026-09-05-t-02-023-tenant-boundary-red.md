---
fecha: 2026-09-05
fase: "02"
agente: claude-opus-5
---

## Tarea cerrada

- **T-02-023** — RED de `middleware_auth.go` (`PR-02-12`). **El criterio de éxito de la Fase 02.**

Rama `feat/pr-02-12-middleware`, que sale de `feat/pr-02-16-email`. 445 líneas de test, cero de
implementación.

## El RED es un fallo de compilación, y eso es lo correcto acá

```
undefined: httpapi.ClaimsFromContext
undefined: httpapi.TxFromContext
undefined: httpapi.ShelterIDPathParam
undefined: httpapi.RequireAuth
undefined: httpapi.RequireTenant
```

No hay aserción que falle porque no hay nada que ejecutar. El `dod` de la tarea lo pide así.

## Lo que se está probando NO es "vuelven las filas correctas"

Esto es lo que hace difícil este test, y vale escribirlo antes de que alguien lo lea dentro de
seis meses y piense que le falta cobertura.

RLS **no es** una verificación de identidad. Evalúa correctamente contra el scope que le den. Si
le entregás el refugio equivocado, la política evalúa perfecto contra el tenant equivocado y **no
se levanta un error en ningún lado**. Es una lectura cruzada, silenciosa, con todos los semáforos
en verde.

Entonces la propiedad bajo test es una sola:

> El valor que llega a `WithTenant` salió del claim verificado y de **nada más**.

## Dos tests que existen porque los obvios no distinguen nada

**1. Scope en una ruta SIN segmento de refugio en el path.**

En un request donde el path y el claim coinciden, un middleware que lee el claim y uno que
parsea el path producen **el mismo uuid**. El test de "camino feliz" no los distingue. Una
implementación que tome el scope del path pasa todos los demás tests del archivo.

Con una ruta que no tiene el segmento, la única fuente posible es el claim. Esa es la mitad
positiva del límite.

**2. El uuid cero se rechaza.**

`uuid.Nil` es **no-nil** en Go. Un chequeo de presencia escrito como `claims.ShelterID != nil`
sobre un decode que defaulteó el campo lo deja pasar entero hasta `WithTenant` — que sí lo
rechaza, pero **una capa demasiado tarde**. El comentario de `AccessClaims` ya avisaba de esto en
`token.go`; acá queda con un test atrás.

## Los espías fallan al contacto

El scoper y el handler **fallan el test apenas los tocan**. "Nunca invocado" es el default, y
llegar a ellos hay que habilitarlo explícitamente en el test que lo espera.

Y cada rechazo se aserta **dos veces**: el status que ve el cliente, y que ni el scoper ni el
handler corrieron.

> Un 403 escrito **después** de que `WithTenant` ya abrió una transacción sobre el refugio del
> atacante no es un rechazo. Es una fuga con un status code que pide disculpas.

## Tokens reales, firmados de verdad

Nada está stubbeado del lado de la verificación. El test emite tokens con `auth.TokenIssuer` y
los firma con una clave real; hasta el caso de "firma inválida" usa un emisor distinto con otro
secreto, en vez de romper una cadena a mano.

Un verificador falso dejaría este archivo entero en verde sobre un middleware que no verifica
absolutamente nada. Es el mismo error que ya está anotado como
[[../20-arquitectura/indice-engram|obs-9e00e3a6413dc7d2]] con otra ropa: la capa que contesta
primero vacía el test de abajo.

## El orden de los chequeos es parte del contrato

| # | Chequeo | Fallo |
|---|---|---|
| 1 | El token verifica | **401** |
| 2 | El claim `shelter_id` existe y no es el uuid cero | **403** |
| 3 | Path / query / header no contradicen al claim | **403** |
| 4 | *Recién ahí* → `WithTenant` | — |

El 401 y el 403 no son intercambiables: el primero dice *"no sé quién sos"*, el segundo *"sé
quién sos y no te alcanza"*. Un token válido sin refugio es lo segundo.

## ⚠️ Presupuesto de `PR-02-12`

| | implementación | total |
|---|---:|---:|
| límite | 250 | 800 |
| `est:` del PR | 290 | 684 (proyección) |
| `T-02-023` sola | 0 | **445** |

`T-02-024` (`est: 140`) todavía no está escrita. El límite total está en **riesgo real** y el de
implementación depende enteramente de cómo caiga el GREEN.

Lo digo ahora, no cuando ya no haya alternativa. Si el GREEN se va de rango, las opciones son
`size:exception` o partir el PR — y partirlo es la que acaba de funcionar bien con `PR-02-16`.

## Siguiente

`T-02-024` — el GREEN. Su `dod` exige **mutación**: la rama del 403-por-mismatch y la de
"`WithTenant` solo desde el claim" son las dos garantías que tienen que sobrevivir a todo
mutante.
