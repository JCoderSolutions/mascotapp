---
type: convention
score: 4
topic_key: mascotapp/convention/hybrid-store-is-recovery
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-90f889e7090661ac
task: T-01-005 / T-01-006 (incidente de truncado)
rationale: "El store híbrido parecía burocracia de doble escritura hasta que fue lo único que recuperó un artefacto perdido. Esa es la razón real de la regla, y no está escrita en ningún lado."
---

**El store híbrido de SDD no es papeleo. Es la copia de recuperación.**

El 2026-08-29 trunqué `openspec/changes/phase-01-domain-and-data/tasks.md` con un error de
splice en un script de edición:

```python
s = s[:i] + s[i:j] + """...texto nuevo..."""
#                                          ^ falta  + s[j:]
```

Se perdieron **30 tareas** (T-01-006 … T-01-035): estimados de línea, trazas a spec, marcas de
blacklist, preguntas abiertas. El repo no tenía commits todavía, así que no había `git` que
valiera.

**Lo recuperó Engram `#41`** (`sdd/phase-01-domain-and-data/tasks`), la otra mitad del store
híbrido, que traía el índice completo con estimados, las tareas no-negociables, la blacklist del
piloto, los pares de paralelismo, la decisión de la clave compuesta de `pet_media`, las
preguntas abiertas y las contradicciones reportadas. Con eso más `design.md` y los siete specs
—intactos— la reconstrucción fue fiel.

## Lo que hay que dejar escrito

1. **La regla "ambas mitades del store, siempre" tiene una razón que nadie enuncia:** no es
   redundancia ceremonial, es que las dos mitades fallan de maneras distintas. Un archivo se
   trunca; una observación de Engram no.
2. **Por eso el agente que reporta éxito habiendo escrito solo una mitad es un problema serio**,
   no un detalle de proceso. Ya había pasado con `sdd-explore`, que escribió Engram y no el
   archivo. Si hubiera pasado al revés con `sdd-tasks`, esto no se recuperaba.
3. **Regla de edición que sale de acá:** al splicear un archivo por índices, la cola (`s[j:]`)
   es obligatoria y hay que verificarla. Preferir `str.replace` sobre aritmética de índices
   siempre que se pueda: `replace` no puede perder el resto del archivo. Y después de cualquier
   reescritura estructural, **contar** lo que quedó (35 tareas, secuencia 1..35, suma de
   estimados) en vez de asumir.

Relacionado: [[analisis-y-plan]] §7.1, que define el mapeo SDD ↔ vault pero describe el híbrido
como requisito de funcionamiento del dispatcher, sin mencionar que también es lo que te salva
de vos mismo.
