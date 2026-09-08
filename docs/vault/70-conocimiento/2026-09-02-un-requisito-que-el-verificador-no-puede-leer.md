---
type: convention
score: 5
topic_key: mascotapp/convention/spec-must-carry-the-property
task: fase-02 planning
approved: 2026-09-02 (aprobacion explicita del usuario)
observation_id: obs-51c9db2915bb5533
rationale: "El diseno protegia shelters.storage_bytes_used con un argumento correcto; el spec no lo nombraba, asi que sdd-verify nunca lo iba a chequear. La propiedad existia en el documento equivocado."
---

# Un requisito que el verificador no puede leer no es un requisito

El diseno de la Fase 02 prohibe que un tenant escriba `shelters.storage_bytes_used` — **el
contador contra el que se chequea la cuota** — y lo argumenta bien: *"un tenant que puede escribir
el contador derrota el chequeo de cuota tan a fondo como uno que puede escribir el limite"*.

**El spec no lo nombraba.** Sus escenarios decian `status`, `storage_quota_bytes` y
`memberships.role`, y nada mas. O sea: la propiedad estaba **razonada en el diseno y ausente del
contrato**, y `sdd-verify` —que valida contra el spec— nunca la iba a chequear. La migracion
podia salir sin esa columna y **todo pasaba en verde**.

**La division de trabajo que esto revela.** El diseno dice **como** y **por que**; el spec dice
**que tiene que ser cierto**. Una propiedad de seguridad que solo vive en el diseno esta apoyada
en que la persona que implementa lea el diseno entero y no se saltee un renglon de una tabla —
que es correccion apoyada en disciplina, el modo de falla que [[ADR-0002-multi-tenancy]] existe para eliminar.

**La pregunta que lo detecta, y se hace en el gatekeeping:** *por cada propiedad que el diseno
promete, hay un escenario del spec que se pone rojo si desaparece?* Si la respuesta es no, la
propiedad esta en el documento equivocado.

Y el mismo pase encontro dos columnas mas por la misma via: `verified_at` y `verified_by` son la
**segunda y tercera columna** del agujero de auto-verificacion que abre `status`. Estaban
igual de ausentes, por la misma razon.
