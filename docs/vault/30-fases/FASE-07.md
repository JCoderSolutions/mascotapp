# FASE 07 — Formularios dinámicos

**Objetivo:** Motor de formularios por refugio, con versionado inmutable y tipos de campo cerrados.

**Entregable verificable:** Constructor con rejilla de 12 columnas + vista previa, versionado inmutable, renderizador RHF+Zod, lógica condicional declarativa, envío con almacenamiento cifrado.

**RDD:** ✅ recomendado · Judgment Day antes del merge del cifrado de PII
**Depende de:** 03

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Alcance heredado de Fase 01 — NO se expande, ya está decidido

Movido acá el **2026-09-01**, en el pase de verificación de `T-01-026`. Estos requisitos vivían
en el delta de `dynamic-forms-data` de Fase 01 y esa fase **no tenía cómo cumplirlos**: los tres
escenarios están redactados como **`WHEN it is validated`**, o sea validación de capa de
aplicación, no esquema. Fase 01 entrega migraciones y políticas RLS.

**Al correr `/sdd-new fase-07`, estos requisitos entran en el delta de esta fase, textuales:**

### Requirement: Field identifiers are stable for the lifetime of a template *(la mitad de validación)*

A `field.id` inside a template definition SHALL be unique within its template and SHALL NOT be
reused or reassigned.

#### Scenario: Duplicate field identifiers are rejected

- GIVEN a definition containing two fields with the same `id` in any section or row
- WHEN it is validated
- THEN validation fails naming the duplicated identifier

### Requirement: Field types are a closed union

A field `type` SHALL be one of `text`, `textarea`, `number`, `date`, `select`, `multiselect`,
`radio`, `checkbox`, `email`, `phone`, `file`, `address`, `signature`. Layout SHALL be a
12-column grid where each field declares a `span` between 1 and 12. Conditional logic SHALL be
declarative data; user-supplied executable code MUST NOT be accepted or evaluated.

#### Scenario: An unknown field type is rejected

- GIVEN a definition containing a field of type `richtext`
- WHEN it is validated
- THEN validation fails naming the unsupported type

#### Scenario: An out-of-range span is rejected

- GIVEN a definition containing a field with `span` of 0 or 13
- WHEN it is validated
- THEN validation fails

### Por qué no se resolvió con un `CHECK` en la base

Se podía. Un `CHECK` de jsonb sobre `definition` tapaba los tres en Fase 01, y **es la capa
equivocada como ÚNICA capa**: el mensaje de error de un `CHECK` no puede *"nombrar el
identificador duplicado"* ni *"nombrar el tipo no soportado"*, que es literalmente lo que los
escenarios piden. La base puede ser la última línea de defensa acá, no la primera. Si esta fase
quiere además un `CHECK` como red de seguridad, es una decisión propia — pero el validador va
primero.

### La restricción que hace esto urgente dentro de esta fase

**La validación tiene que aterrizar ANTES o JUNTO con el primer camino de escritura de
plantillas**, y no después. `form_template_versions` congela las filas publicadas con un trigger:
una definición inválida que se publica **no se puede arreglar**, solo se puede publicar una
versión nueva al lado. No es deuda que se limpia; es deuda permanente.

**Hoy la ventana está cerrada por orden de fases** —ninguna fase entre la 01 y esta escribe
plantillas— y es esta fase la que la abre. El constructor de formularios y el validador son la
misma tarea, no dos.

## Tareas

> Esta fase **se expande al nivel de tarea al iniciarla**, no antes.
> Expandir las 12 fases hoy produce tareas obsoletas.
>
> Al abrir la fase: correr `/sdd-new fase-07` → `/sdd-ff`, y volcar acá el
> checklist de `tasks.md` con IDs `T-07-NNN`.

- [ ] **T-07-001** · (pendiente de expansión)

---

## Salida de fase

1. Todas las tareas en `[x]`.
2. Verificación de fase del plan maestro ejecutada y registrada.
3. Cola de Engram vacía o aprobada.
4. `/sdd-verify` en verde → `/sdd-archive`.

Siguiente: [[FASE-08]]
