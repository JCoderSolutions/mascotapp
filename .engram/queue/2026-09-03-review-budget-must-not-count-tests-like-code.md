---
type: convention
score: 5
topic_key: mascotapp/convention/review-budget-counts-code-and-tests-separately
task: T-02-009
status: guardado
observation_id: obs-acb72253edc70966
rationale: "Un presupuesto de revisión que suma tests e implementación en un solo número cobra igual por revisar una lista de casos ya verdes que por revisar una política de seguridad. En un proyecto con TDD estricto y mutation testing —donde los tests son el 70% del diff— eso se convierte en un impuesto a las prácticas que más protegen el código, y empuja a una de dos salidas, ambas peores: escribir menos pruebas, o volver la excepción rutina. La regla general es que un límite tiene que medir la cosa cuyo riesgo dice acotar."
---

# Un presupuesto de revisión que cuenta tests como código está midiendo la cosa equivocada

MascotApp fijó un presupuesto de **400 líneas autoradas por PR**. Siete PRs consecutivos de
la Fase 02 lo excedieron; tres necesitaron `size:exception`. Cuando una regla se incumple una
vez es un caso; siete veces seguidas, **la regla es la hipótesis a revisar, no el trabajo**.

## Lo que apareció al medir contra `git` en vez de contra la tabla

| PR | total | no-test | tests | tests% |
|---|---:|---:|---:|---:|
| 1 | 399 | 139 | 260 | 65% |
| 2 | 579 | 129 | 450 | 77% |
| 3 | 359 | 120 | 239 | 66% |
| 4 | 280 | 90 | 190 | 67% |
| 5 | 836 | 193 | 643 | 76% |
| 6 | 291 | 92 | 199 | 68% |
| 7 | 538 | 221 | 317 | 58% |

**El código no violó el presupuesto ni una sola vez.** Mediana 129, máximo 221 — nunca cerca
de 400. Lo que lo reventaba eran los tests, **58–77% de cada PR**, que son la consecuencia
directa de tres reglas elegidas a propósito: TDD estricto, mutation testing por tarea, y un
caso de anti-vacuidad detrás de cada aserción negativa.

## Por qué sumar las dos cosas es un error y no una simplificación

Un presupuesto de revisión es un *proxy de capacidad de revisión*. Pero revisar 250 líneas de
casos table-driven —cada uno nombrado, con su razón al lado, ya corridos, ya verdes, con sus
mutantes muertos— **no cuesta lo mismo** que revisar 250 líneas de una política RLS o de una
comparación de hashes. En un caso el revisor lee una lista; en el otro busca un defecto.

Sumarlas produce un **impuesto al testing**, y ese impuesto se paga en uno de dos lugares: se
escriben menos pruebas, o la excepción se vuelve rutina y deja de significar nada. Las dos
salidas son peores que el problema original.

## El arreglo: dos presupuestos, porque son dos preguntas

| Presupuesto | Límite | Qué cuenta |
|---|---:|---|
| Implementación | **250** | Go que no es test, SQL de migración, archivos de query, `go.mod` |
| Diff total | **800** | todo lo anterior más los tests |

Los dos números se defienden solos: **250** sale de la medición (máximo observado 221), y
**800** es la segunda opción estándar del propio marco SDD (`review_budget_lines: 400 | 800`),
no un número fabricado para que entre todo. **Se rechazó 900 a propósito** porque habría
legalizado hacia atrás todos los casos que motivaron el análisis.

Resultado sobre los 17 PRs restantes: **uno solo** rompe el techo nuevo. Eso es lo que un
presupuesto tiene que producir — una regla que no marca nada no está midiendo, y una que
marca todo no es una regla.

## El segundo hallazgo, que nadie estaba mirando: el estimador

`est:` predecía **implementación**, no el PR. Contra el código real **sobra** en seis de siete
casos (1,25×–2,02×); el único donde se queda corto es el único PR de Go puro sin base de
datos. Contra el **total** se queda corto por **2,36×**.

Regla derivada: estimá el código, y proyectá los tests aparte — **~2×** el código en trabajo
de base de datos, **~1,5×** en Go puro. Un solo número que mezcla las dos cosas va a seguir
fallando por 2× sin importar dónde esté el techo.

## Y lo que se dejó expuesto a propósito

El límite de implementación tiene **una sola observación cerca** y está proyectado a
excederse en el PR siguiente. **Esa proyección no movió el número.** Un presupuesto fijado
con siete mediciones no se re-fija con una cuenta; si el próximo PR lo excede, eso es una
conversación real con un número real. Medir, no predecir — la misma disciplina que produjo
el diagnóstico.

Relacionado: [[recompute-announced-numbers]] — la medición contra `git` también corrigió un
número anunciado (un PR reportado en 314 que era 359, porque se midió antes de un agregado
posterior).
