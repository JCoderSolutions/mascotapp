---
type: convention
score: 5
topic_key: mascotapp/convention/doubles-must-not-be-weaker-than-production
task: T-02-030
status: guardado
aprobado: 2026-09-24 por el usuario
observation_id: obs-0210963cf0f98b48
rationale: "Tres mutantes distintos de una sola tarea sobrevivieron por esta misma causa, y ninguno era un defecto del código: el doble era demasiado rápido, demasiado permisivo, y el arnés no cancelaba lo que un servidor real cancela. Los tres fallos que escondían son de la clase que en producción no deja rastro — sin error, sin log, con un 200 al llamador. Y la pregunta que los detecta se hace al escribir el doble, que es cuando todavía es gratis. Es pariente directo de la regla de fixtures y la completa: aquella acota QUÉ formas se pueden construir, ésta acota QUÉ fallas se pueden observar."
---

# Un doble más permisivo que la cosa real vuelve intestable el bug que esconde

(T-02-030, MascotApp Fase 02. Pariente de
`[[2026-09-08-la-fixture-es-el-limite-de-lo-que-un-test-puede-decir]]`.)

## La regla

Un doble de test tiene que ser **al menos tan estricto y al menos tan lento** como
aquello que reemplaza, en las dimensiones que la propiedad bajo prueba usa. Si es
más permisivo, el bug que esconde **no se puede escribir como test**.

## Tres instancias, una tarea

Tres mutantes sobrevivieron por esta causa, y **ninguno era un defecto del
código**:

### 1. El doble era demasiado rápido

El envío de correo es asíncrono **a propósito**: síncrono, el camino con cuenta
paga un viaje al proveedor que el camino sin cuenta no paga, y eso es un oráculo
de tiempo a través de dos cuerpos idénticos.

El mutante que saca el `go` **sobrevivió**. Con un sender en memoria, síncrono y
asíncrono cuestan lo mismo.

> La propiedad no es *"el envío es rápido"*. Es *"la respuesta no depende del
> envío"*, y **solo un doble lento las distingue.** El test nuevo usa uno de
> 500 ms y verifica que la respuesta vuelva antes.

### 2. El doble era demasiado permisivo

`email.checkSend` refusa un contexto cancelado. Mis dobles lo ignoraban, así que
un handler que le pasa a la goroutine un contexto ya cancelado **grababa el envío
igual**.

### 3. El arnés no cancelaba lo que un servidor real cancela

Y esta es la mitad que no está en el doble. `httptest.NewRecorder` **nunca
cancela** el contexto del request — no tiene servidor que lo haga. `net/http` sí,
en cuanto `ServeHTTP` retorna.

Un handler que le pasa el contexto del request a una goroutine que sobrevive a la
respuesta se ve **perfectamente sano** bajo el recorder, y en producción no manda
un solo correo: **sin error, sin log, con un 202 al llamador**.

El arreglo fue correr ese test contra un `httptest.Server` real.

## La trampa

Un doble rápido y permisivo **parece mejor**: los tests corren en microsegundos y
no fallan por ruido. Esa es exactamente la propiedad que lo vuelve ciego.

Y las tres fallas que escondía comparten forma: **en producción no dejan rastro**.
No hay excepción, no hay log, el llamador recibe un 200. Son las que más falta
hace atrapar, y son justo las que un doble cómodo no puede ver.

## La pregunta que lo detecta

Al escribir el doble, no al depurar:

> **¿Qué hace la cosa real que este doble no hace?**

Cada respuesta es un modo de falla que la suite no va a poder observar. Tres
dimensiones que aparecieron acá: **el tiempo** que tarda, las **precondiciones**
que valida, y el **ciclo de vida** que el arnés administra alrededor.

## Cómo se relaciona con la regla de fixtures

Son la misma familia, en ejes distintos:

| Regla | Acota |
|---|---|
| La fixture es el límite | **qué formas** se pueden construir |
| Ésta | **qué fallas** se pueden observar |

Las dos se detectan igual —**mutando el código, no leyendo los tests**— porque en
los dos casos cada test individual está bien escrito y afirma exactamente lo que
dice.

## Dónde más aplica

- todo doble de un puerto que valida algo (`email.Sender`, el futuro adaptador de
  R2, el de Resend): el doble tiene que rechazar lo que el real rechaza
- `httptest.NewRecorder` frente a `httptest.Server` cada vez que el handler lance
  una goroutine, escriba después de responder, o dependa del ciclo de vida del
  request
- cualquier doble en memoria de algo que en producción cruza la red, cuando la
  propiedad bajo prueba tiene que ver con **cuánto tarda** o con **qué pasa si
  falla**
