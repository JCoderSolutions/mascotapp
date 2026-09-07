---
type: convention
score: 4
topic_key: mascotapp/convention/a-blind-judge-findings-are-inferential
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-94366e6a3c8b917d
task: T-01-016
rationale: "La regla mecánica del contrato habría mergeado una fuga de PII; convertir inferencia en hecho es el trabajo del orquestador."
---

# Un juez ciego no puede ejecutar, así que sus hallazgos llegan como "inferential"

Judgment Day corre dos jueces ciegos read-only. El contrato dice: **se arregla
solo lo que confirman los dos.** La regla existe para no actuar sobre la
especulación de un solo juez.

En la revisión de la plantilla RLS los dos jueces devolvieron **cero CRITICAL y
cero coincidencias**. Aplicar la regla al pie de la letra habría firmado
`APPROVED` y mergeado la migración tal cual.

Dos de los cuatro hallazgos estaban marcados **WARNING / inferential** — y los
jueces decían por qué: *"no provable as CRITICAL from the files in scope alone"*,
*"cannot be confirmed from this slice"*. No era falta de gravedad: **era falta de
poder ejecutar.**

Una sonda de treinta líneas contra la base real los convirtió:

- uno resultó **más grave** que su propio reporte (un grant de lectura
  self-service sobre PII, ver [[una-membership-invitada-es-un-grant-de-lectura]]);
- otro quedó **confirmado y diferido** con prueba, en vez de como sospecha;
- y un tercero quedó **refutado** con evidencia de fuera del alcance del juez.

## La regla

**Antes de firmar un veredicto, convertí cada hallazgo inferencial en un hecho o
en un refutado.** El orquestador puede ejecutar; el juez no. Esa asimetría no es
un defecto del método, es la división del trabajo — pero solo funciona si el
orquestador hace su mitad.

Corolario: la prueba ejecutable es **más fuerte** que un segundo juez, y
satisface el espíritu de la regla de corroboración aunque no su letra. Lo que la
regla prohíbe es actuar sobre una opinión, no sobre una repro.
