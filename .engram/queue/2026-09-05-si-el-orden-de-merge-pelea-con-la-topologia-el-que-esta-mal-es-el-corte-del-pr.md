---
type: convention
score: 4
topic_key: mascotapp/convention/pr-cut-owns-the-merge-order
task: T-02-022
status: pendiente-de-aprobacion
rationale: "Esta fase entrega 24 PRs encadenados y las siguientes van por el mismo camino. La tentación de resolver un conflicto de orden con topología de ramas va a volver, y la segunda vez va a parecer razonable otra vez. El costo de equivocarse no es un rebase molesto: es que PROJECT_STATE.md —el contrato de continuidad del que depende todo agente en frío— pasa a tener dos respuestas contradictorias según en qué rama estés parado. Eso es exactamente el riesgo IA-4 del plan, fabricado por proteger el orden de una lista."
---

# Si el orden de merge pelea con la topología de ramas, el que está mal es el corte del PR

(T-02-022, MascotApp Fase 02.)

## La situación

Una fase entregada como **cadena de PRs** tiene dos ordenamientos que se pueden desincronizar:

- el orden de **tareas**, que sigue dependencias de compilación;
- el orden de **merge**, que sigue dependencias de revisión.

Cuando una tarea temprana pertenece a un PR tardío, los dos se contradicen.

## Lo que hice mal

Para proteger un orden de merge que decía *12 antes que 16*, hice que los PRs 12–15 salieran de
la base común como **hermanas** de la rama del PR 16, en vez de encadenarlas encima.

Funcionó exactamente un commit.

## Por qué se rompe, y no es por el rebase

Apenas me paré en la rama hermana, `PROJECT_STATE.md` volvió a decir *"sigue T-02-022"* — una
tarea ya cerrada en la rama de al lado.

Ese archivo **es** el contrato de continuidad: es lo primero que lee un agente en frío para saber
dónde está el trabajo. Con ramas hermanas hay **dos respuestas contradictorias a "dónde vamos"**,
y cuál obtenés depende de en qué checkout estés parado.

Es el riesgo `IA-4` del plan —dos fuentes de verdad— **fabricado por mí**, para proteger el orden
de una lista.

> Protegí la lista y rompí aquello para lo que la lista existe.

## La regla

> **El orden de merge y la topología de ramas nunca negocian. Si se contradicen, lo que está mal
> es el CORTE DEL PR.**
>
> Un PR que contiene dos tareas separadas por otros cuatro PRs no es una unidad revisable, sea
> cual sea el orden en que se mergee. Partilo. La cadena de ramas se queda en **una sola línea**,
> siempre, y `PROJECT_STATE.md` tiene **una sola respuesta**.

## Cómo se ve aplicada

`PR-02-16` contenía el puerto de email (`T-02-022`, sin dependencias) y el handler de magic link
(`T-02-030`, que necesita `auth_handlers.go` de los PRs 14/15). Se partió:

| | Contenido | Merge |
|---|---|---|
| `PR-02-16a` | el puerto y su stub | posición 12, justo tras `PR-02-11` |
| `PR-02-16b` | el handler | posición 17, sobre `PR-02-15` |

Mismo corte que ya había tomado `PR-02-08` → `08a`/`08b`, por otro motivo (tamaño).

## El síntoma que te avisa antes

El corte estaba mal **antes** de que apareciera el conflicto de orden, y había una señal: el PR
sin partir proyectaba 448 líneas y su primera mitad sola ya medía 469. Un PR que se pasa de
presupuesto con la mitad de su contenido casi nunca es un problema de presupuesto — es un
problema de corte que se manifiesta como uno de presupuesto.

Partirlo arregló las dos cosas de una: `PR-02-16a` mide 158/469 y entra en los dos límites sin
excepción.
