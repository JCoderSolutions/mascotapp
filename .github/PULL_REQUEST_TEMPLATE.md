## Qué hace este PR

<!-- Una o dos frases. Qué cambia y por qué, no cómo. -->

## Tarea / spec relacionada

- Tarea: `T-NN-NNN`
- Spec: `openspec/changes/<change>/specs/...` (o el ADR/nota de vault equivalente)

## Tamaño

- Implementación: **___ líneas** (presupuesto: ≤250 — Go que no es test, SQL de migración, archivos de `query/`, `go.mod`)
- Diff total: **___ líneas** (presupuesto: ≤800 — todo lo anterior más los tests)
- [ ] Dentro de presupuesto
- [ ] Excede presupuesto — ver sección `size:exception` abajo

<!-- Si excede presupuesto, completar: -->
### `size:exception` (si aplica)

Motivo estructural concreto por el que este PR excede el presupuesto (no "se
armó así"): ej. "incluye migración + rollback + tests de ambos caminos, no se
puede partir sin dejar un estado intermedio inconsistente".

## Definition of Done

- [ ] `make test-api-container` y/o `make test-web` (según corresponda): rojo → verde
- [ ] `make lint` limpio
- [ ] Cobertura ≥80% en el código tocado, o justificar por qué no aplica
- [ ] `PROJECT_STATE.md` actualizado (incluido `next_action`)
- [ ] Candidatos de memoria evaluados contra [`.engram/RUBRIC.md`](../.engram/RUBRIC.md) (score ≥3 → `.engram/queue/`)

## Memoria (Engram)

- `observation_id` (si esta tarea generó memoria nueva, tras la aprobación del usuario): `________` (o "—" si no aplica)

## Notas para quien revisa

<!-- Cualquier contexto que no sea obvio del diff: decisiones descartadas,
riesgos conocidos, por qué esta forma y no otra. -->
