# Vault de MascotApp

Markdown plano, sin plugins obligatorios. Abrí esta carpeta como vault en Obsidian.
El repositorio es la verdad operativa; Engram es el índice semántico de decisiones.

| Carpeta | Contenido |
|---|---|
| `00-inbox/` | Notas sin clasificar. Se vacía, no se acumula |
| `10-propuesta/` | Propuesta original y plan maestro aprobado |
| `20-arquitectura/` | ADRs — decisiones fechadas e **inmutables** |
| `30-fases/` | **Tablero de tareas.** Una fase por archivo |
| `40-bitacora/` | Log diario escrito por los agentes |
| `50-specs/` | Especificaciones por área |
| `60-presentacion/` | Material para presentar la propuesta |
| `99-plantillas/` | Plantillas de tarea, ADR y bitácora |

## Por dónde empezar

1. `../../AGENTS.md` — el contrato de trabajo, si sos un agente
2. `../../PROJECT_STATE.md` — dónde vamos ahora mismo
3. `30-fases/FASE-<actual>.md` — la tarea siguiente
4. `10-propuesta/analisis-y-plan.md` — el plan completo
5. `20-arquitectura/indice-engram.md` — el *porqué* de lo ya decidido, sin MCP

## Regla

Un ADR publicado no se edita. Si una decisión cambia, se escribe un ADR nuevo
que supersede al anterior y se enlaza en ambos sentidos.
