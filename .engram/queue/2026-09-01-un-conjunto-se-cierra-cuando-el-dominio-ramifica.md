---
type: convention
score: 4
topic_key: mascotapp/convention/close-a-set-when-the-domain-branches
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-e0fefde5e839d79a
task: T-01-029
rationale: "Este esquema cierra casi todos sus conjuntos con CHECK y la costumbre ya empezaba a parecer una regla. La regla real tiene un criterio, y una tabla de la misma migracion cae de cada lado."
---

# Un conjunto se cierra cuando el dominio RAMIFICA sobre el

`00010` creo dos columnas de conjunto y les dio tratamientos **opuestos**, en la misma migracion.
El criterio no es la costumbre; es una pregunta:

> ¿El codigo **ramifica** sobre este valor, o solo lo **transporta**?

**`application_notes.visibility` — CERRADO** (`internal | shared`). Una fase posterior decide
**quien puede leer la nota** mirando esta columna. Un tercer valor llega a un `switch` sin rama, y
lo que haga el fallback decide si un adoptante ve lo que el refugio escribio sobre el. Que esta
fase solo lo ALMACENE no cambia nada: la columna tiene que ser cerrada desde el dia uno, porque el
dato malo entra antes de que exista la rama.

**`application_events.type` — ABIERTO** (solo `length BETWEEN 1 AND 64`). El dominio lo **emite**;
un lector que no reconoce un tipo lo **ignora**; y el conjunto crece con cada feature que registre
algo. Un `CHECK` ahi seria **una migracion por tipo de evento**, que es exactamente como una linea
de tiempo deja de escribirse: el que agrega el feature no quiere tocar una migracion, entonces
reusa un tipo existente y el evento miente.

**Los precedentes del proyecto caen del lado cerrado por la misma razon, no por costumbre:**
`pets.status` y `adoption_applications.status` son maquinas de estado sobre las que el dominio
ramifica; `media.kind` decide por que rama del pipeline de derivacion pasa el archivo (Fase 04).

**Y el corolario incomodo:** cerrar de mas tiene un costo que no se ve en los tests. Un `CHECK`
sobre una columna que solo se transporta convierte cada extension del producto en una migracion, y
las migraciones son append-only una vez aplicadas. Es mas facil cerrar despues que abrir despues.
