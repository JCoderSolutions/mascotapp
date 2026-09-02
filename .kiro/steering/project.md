---
inclusion: always
---

# MascotApp

**Todo el contrato de trabajo vive en [`AGENTS.md`](../../AGENTS.md), en la raíz
del repositorio. Leelo antes de tocar nada.**

Este archivo existe solo como puntero para el IDE de Kiro, que carga
`.kiro/steering/*.md`. No duplica contenido a propósito: dos copias de una regla
son dos reglas que se van a contradecir.

Lo mínimo, por si no seguís el enlace:

- Empezá por `PROJECT_STATE.md` → `docs/vault/30-fases/FASE-<fase actual>.md` →
  `openspec/changes/<sdd_change>/tasks.md`.
- **La suite de Go se corre con `make test-api-container`.** `go test` a secas no
  arranca en este host: Smart App Control bloquea cada binario recién linkeado.
- TDD estricto. Una sola tarea en `[~]` a la vez.
- Artefactos técnicos en inglés; documentación del vault en español.
- Commits convencionales, **sin atribución a IA**.
