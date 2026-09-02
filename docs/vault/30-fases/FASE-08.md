# FASE 08 — Flujo de adopción

**Objetivo:** La adopción como máquina de estados, no como buzón de formularios.

**Entregable verificable:** Estados de solicitud, asignación de responsable, notas internas, timeline auditable, notificaciones por correo.

**RDD:** —
**Depende de:** 06, 07

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Alcance heredado de Fase 01 — decidido, no abierto

Arrastrado el **2026-09-01** al cerrar `T-01-035`.

### La purga de retención de §5.4 no es un `DELETE` de la solicitud

Con `application_events`, `application_notes` y `documents` los tres en `ON DELETE RESTRICT`,
**una solicitud que tenga cualquier historia no se puede borrar en duro** — y eso está bien, el
rastro es lo que hace que una auditoría signifique algo.

**Decisión del usuario:** la purga **borra `form_submissions`** —las respuestas, que es donde vive
la PII de §5.4— y **la solicitud queda como registro**. No cuesta ninguna migración.

**El costo, dicho y aceptado:** `adoption_applications.applicant_user_id` es `NOT NULL`, así que
el **vínculo al titular sobrevive**. **No es un borrado completo del sujeto.** Si alguna vez hace
falta que lo sea, las dos salidas ya están escritas: volver `applicant_user_id` nullable para poder
cortarlo, o un estado `purged` explícito en el conjunto de status.

**Y una corrección que viaja con esto:** el comentario de `00009` que justifica la `CASCADE` de
`form_submissions.application_id` **por** esta purga está incompleto — con historia presente esa
cascada **no se dispara nunca**. La cascada sigue siendo correcta para el caso sin historia; la
purga real es el `DELETE` directo sobre `form_submissions`.

## Tareas

> Esta fase **se expande al nivel de tarea al iniciarla**, no antes.
> Expandir las 12 fases hoy produce tareas obsoletas.
>
> Al abrir la fase: correr `/sdd-new fase-08` → `/sdd-ff`, y volcar acá el
> checklist de `tasks.md` con IDs `T-08-NNN`.

- [ ] **T-08-001** · (pendiente de expansión)

---

## Salida de fase

1. Todas las tareas en `[x]`.
2. Verificación de fase del plan maestro ejecutada y registrada.
3. Cola de Engram vacía o aprobada.
4. `/sdd-verify` en verde → `/sdd-archive`.

Siguiente: [[FASE-09]]
