# Diagnóstico: ¿el presupuesto de 400 líneas por PR es viable?

> Fecha: 2026-09-03 · Disparado por siete estimados consecutivos que cayeron alto
> Método: medición contra `git`, no contra la tabla de `tasks.md`

## La pregunta

Siete PRs de la Fase 02 excedieron su estimado. Tres necesitaron `size:exception`. Cuando
una regla se incumple una vez es un caso; cuando se incumple siete veces seguidas, **la regla
es la hipótesis a revisar, no el trabajo**.

## Los datos

Medido con `git diff --numstat` entre las puntas de rama, solo código
(`apps/api` + `scripts`), excluyendo `sqlcgen/` y `go.sum`. Excluye también todo el
bookkeeping —vault, openspec, `.engram`, `PROJECT_STATE`— que nunca formó parte del
presupuesto.

| PR | est | **total** | impl (Go) | SQL migración | otros | **tests** | tests% |
|---|---:|---:|---:|---:|---:|---:|---:|
| `PR-02-01` | 250 | **399** | 139 | 0 | 0 | 260 | 65% |
| `PR-02-02` | 260 | **579** | 9 | 120 | 0 | 450 | 77% |
| `PR-02-03` | 150 | **359** | 84 | 0 | 36 | 239 | 66% |
| `PR-02-04` | 160 | **280** | 4 | 58 | 28 | 190 | 67% |
| `PR-02-05` | 270 | **836** | 78 | 115 | 0 | 643 | 76% |
| `PR-02-06` | 140 | **291** | 0 | 92 | 0 | 199 | 68% |
| `PR-02-07` | 160 | **538** | 219 | 0 | 2 | 317 | 58% |
| | **1.390** | **3.282** | **533** | **385** | **66** | **2.298** | **70%** |

> **Corrección de un número anunciado.** `PR-02-03` se reportó en su momento como 314. El
> total real de la rama es **359**: la diferencia son las 45 líneas de
> `checkPolicyLandsAtIsHonest` que el orquestador agregó *después* de que el agente midiera
> (commit `5bbbad6`). El número se recomputó contra `git`, que es la única fuente que no
> se queda vieja.

## El hallazgo

**El código no violó el presupuesto ni una sola vez.**

Sumando todo lo que no es test —Go de implementación, SQL de migración, entradas de sqlc—
los siete PRs dan: **139, 129, 120, 90, 193, 92, 221**.

- Máximo: **221**
- Mediana: **129**
- Ninguno pasó de 250, mucho menos de 400.

Lo que revienta el presupuesto son **los tests, que son el 58–77% de cada PR** (70% del
total). Y no son tests inflados: son la consecuencia directa de tres reglas que este
proyecto eligió a propósito —TDD estricto, mutation testing por tarea, y un caso de
anti-vacuidad por cada aserción negativa— más una convención de comentarios que explica el
*por qué* de cada caso.

## El diagnóstico, en dos partes

### 1. El estimado no está midiendo lo que dice medir

`est:` predice **implementación**, no el PR. Comparado contra el código real, el estimado
incluso **sobra** en seis de siete casos:

| PR | est | impl real | est ÷ impl |
|---|---:|---:|---:|
| `PR-02-01` | 250 | 139 | 1,80× |
| `PR-02-02` | 260 | 129 | 2,02× |
| `PR-02-03` | 150 | 120 | 1,25× |
| `PR-02-04` | 160 | 90 | 1,78× |
| `PR-02-05` | 270 | 193 | 1,40× |
| `PR-02-06` | 140 | 92 | 1,52× |
| `PR-02-07` | 160 | 221 | **0,72×** |

El único caso donde el estimado se queda corto contra la implementación es `PR-02-07`, el
único PR de **Go puro sin base de datos**. Eso es coherente: en trabajo de base, la
implementación es una migración corta y la prueba es cara; en cripto, el código pesa.

**Contra el total, el estimado se queda corto por 2,36× (mediana 2,08×).**

### 2. El presupuesto está midiendo dos cosas distintas con la misma regla

400 líneas existe como *proxy de capacidad de revisión*. Pero revisar 250 líneas de casos
table-driven —cada uno nombrado, con su razón escrita al lado, ya corridos, ya verdes, y con
sus mutantes muertos— **no cuesta lo mismo** que revisar 250 líneas de una política RLS o de
comparación de hashes. En un caso el revisor lee una lista; en el otro busca un defecto.

Un presupuesto que suma las dos cosas obliga a pedir excepción por escribir buenas pruebas.
Eso no es un presupuesto: es un impuesto al testing, y termina en uno de dos lugares — se
escriben menos pruebas, o la excepción se vuelve rutina y deja de significar nada. **Las dos
salidas son peores que el problema.**

## La proyección, que es lo urgente

Con el factor medido de **2,36×** sobre lo que resta:

- Estimado de la fase: **4.890** líneas, 40 tareas, 24 PRs
- Ya entregado: **1.390** estimadas → **3.282** reales
- Resta estimado: **3.500** → proyección real: **~8.260**
- **Proyección de fase completa: ~11.500 líneas contra 4.890 planificadas**

La rebanada (c) sola está estimada en 3.140. Bajo el factor medido son **~7.400**. Los 17 PRs
que quedan no van a entrar en 400 con la regla actual, y ya sabemos que no es porque el
trabajo esté mal.

## Recomendación

**Dos presupuestos, porque son dos preguntas distintas.**

| Presupuesto | Límite | Qué cuenta |
|---|---:|---|
| **Implementación** | **250** | Go que no es test, SQL de migración, entradas de sqlc, `go.mod` |
| **Diff total** | **800** | Todo lo anterior más los tests |

Por qué esos números, y no dos que hagan entrar todo cómodamente:

- **250 para implementación** sale de los datos: el máximo observado es 221 y la mediana 129.
  Deja poco aire a propósito. Es el número que sigue el riesgo de defecto, y donde una
  `size:exception` vuelve a significar algo.
- **800 para el diff total** no lo inventé: es la **segunda opción estándar del propio marco
  SDD** (`review_budget_lines: 400 | 800`). Elegirlo es cambiar de opción documentada, no
  fabricar un número a medida. Bajo ese techo, **seis de los siete PRs entran**; `PR-02-05`
  (836) sigue afuera por 4,5% y conserva su excepción. Deliberadamente **no** elegí 900,
  que habría legalizado todo hacia atrás — que es exactamente el movimiento que este
  proyecto rechaza en todos lados.

**Y arreglar el estimador**, que es la mitad que nadie estaba mirando: `est:` debería
declararse como **implementación**, con los tests proyectados aparte a ~2× para trabajo de
base de datos y ~1,5× para Go puro. Un solo número que mezcla las dos cosas va a seguir
fallando por 2×, sin importar dónde se ponga el techo.

## Lo que NO recomiendo, y por qué

- **Subir el presupuesto a 900 o 1.000 y seguir.** Fija el síntoma y borra la señal: si todo
  entra, el número dejó de decir nada.
- **Escribir menos tests para entrar en 400.** Es la única lectura de los datos que produce
  peor software, y las pruebas que sobran no existen: cada caso extra de esta fase mató un
  mutante o cerró una vacuidad.
- **Dejarlo como está y seguir pidiendo excepción.** Tres de siete ya la pidieron. A la
  quinta, la excepción es la regla y ya nadie la lee.
