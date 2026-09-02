---
type: architecture
score: 5
topic_key: mascotapp/arch/cascade-direction-follows-the-child
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-5711866a288ae91b
task: T-01-027
rationale: "El proyecto tenia una regla implicita 'siempre RESTRICT' que funciono seis veces y es incorrecta como regla. La direccion la decide QUE ES el hijo, no la costumbre."
---

# La direccion de la cascada la decide QUE ES el hijo, no la costumbre

Este esquema puso `ON DELETE RESTRICT` en cada referencia compuesta, y por buenas razones cada
vez: el hijo era la **historia** y el padre la **fila viva**, asi que una cascada le dejaba al
tenant borrar el rastro borrando lo que el rastro apunta. `pet_status_history` es el caso
canonico — append-only con cascada es **teatro**, el refugio borra el historial borrando al
animal.

`form_submissions.application_id -> adoption_applications` va con **`ON DELETE CASCADE`**, y no
es una inconsistencia: **la direccion esta dada vuelta.**

Aca el hijo es **EL DATO PERSONAL** y el padre es **EL CASO**. §5.4 pide una purga de retencion
de las solicitudes rechazadas y un borrado a pedido del titular, y las dos cosas *son* que las
respuestas se vayan. Un `RESTRICT` dejaria el documento del adoptante —domicilio, documento de
identidad, referencias— **despues** de purgar el caso al que pertenecia. Eso no es prudencia: es
exactamente la falla de privacidad que la politica de retencion existe para prevenir.

**Lo que hace que sea seguro es que el rastro de auditoria NO vive ahi.** `application_events` y
`audit_log` son append-only y separados, y **los dos tienen que referenciar `adoption_applications`
con `RESTRICT`** — por la misma razon por la que este no. Si el rastro viviera en el mismo hijo
que el dato personal, no habria eleccion correcta: o se pierde el rastro, o se retiene la PII.
**Que sean tablas distintas es lo que permite que cada una tenga la regla que le corresponde.**

**La pregunta a hacerse en cada FK, en vez de aplicar la costumbre:** si el padre se va, ¿que
tiene que pasar con el hijo *para el negocio*? Si el hijo es evidencia de lo que paso, RESTRICT.
Si el hijo es dato personal cuyo motivo de existir era el padre, CASCADE. Si es las dos cosas,
esta mal modelado y hay que partirlo.
