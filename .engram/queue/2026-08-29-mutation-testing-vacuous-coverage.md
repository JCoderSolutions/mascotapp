---
type: convention
score: 4
topic_key: mascotapp/convention/mutation-testing-finds-vacuous-coverage
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-7472df2f86aeaeb4
task: T-00-022
rationale: "Explica por qué en este proyecto cada tarea de seguridad paga el costo extra de mutar la implementación en vez de confiar en la cobertura. Sin el caso concreto, alguien va a saltearse el paso por considerarlo ceremonia."
---

En MascotApp, un test en verde no cuenta como prueba hasta que se muta la
implementación y el test falla. No es ceremonia: ya atrapó un hueco real.

Caso concreto (T-00-022, resolución de IP de cliente): de cuatro mutantes, tres
murieron a la primera. El cuarto —cambiar `return peer` por `continue` cuando
una entrada de `X-Forwarded-For` es malformada— **sobrevivió con la suite en
verde**.

La causa es un patrón que se repite y hay que vigilar: **todos los casos de
prueba de "entrada malformada" tenían un solo elemento en la cadena.** Con un
solo elemento, "detenerse ante lo inválido" y "saltear lo inválido" producen
exactamente el mismo resultado. El test parecía cubrir la regla y no la cubría.

Regla general que se deriva: cuando una función decide **detenerse** en vez de
**continuar** ante una condición, el test necesita al menos un elemento
*después* del punto de detención. Si no, no está probando la decisión — está
probando el caso degenerado donde ambas ramas coinciden.

En este caso la diferencia era de seguridad, no cosmética: saltear permitía
devolver una dirección falsificada por el atacante.
