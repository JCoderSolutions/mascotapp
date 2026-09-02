# MascotApp

Plataforma para que refugios de animales publiquen animales en adopción, gestionen
solicitudes con formularios propios y den transparencia total al adoptante.

Sin fines de lucro. Desplegada a costo cero (ver [ADR-0003](docs/vault/20-arquitectura/ADR-0003-costo-cero.md)).

## Si sos un agente y esta es tu primera sesión

**Empezá por [`PROJECT_STATE.md`](PROJECT_STATE.md).** Ahí está la tarea siguiente y el
ritual de arranque. No adivines: el estado es explícito.

## Estructura

```
PROJECT_STATE.md          Dónde vamos. Fuente única de continuidad
Makefile                  Task runner. Todo borrado legítimo vive acá
.claude/settings.json     Barreras de supervisión (deny/ask)
.engram/                  Rúbrica y cola de curaduría de memoria
api/openapi.yaml          Contrato único de API
apps/api/                 Backend Go
apps/web/                 Frontend React
packages/                 Código compartido Go <-> TS
docs/vault/               Vault de Obsidian (abrilo como vault)
```

## Stack

**Backend:** Go 1.23+ · chi · sqlc + pgx · PostgreSQL (Neon) · goose
**Frontend:** React 19 · Vite · TypeScript strict · TanStack · Tailwind + shadcn/ui
**Infra:** Cloud Run · Neon · Cloudflare R2 + Pages · Resend

## Comandos

```bash
make dev      # Levanta el stack local
make test     # Suite completa
make lint     # Todos los linters
make clean    # Borrado de artefactos (rutas explícitas)
```

## Reglas del proyecto

1. **TDD estricto.** Toda tarea de código empieza con un test que falla.
2. **Los artefactos técnicos van en inglés.** La documentación del vault, en español.
3. **`rm` está prohibido para agentes.** Todo borrado pasa por `make clean`.
4. **Una sola tarea en `[~]`** en todo el tablero.
5. **La plataforma no procesa pagos.** Solo enlaza a links propios del refugio.
6. **Ningún refugio publica sin verificación manual previa.**

## Documentos

- [Análisis crítico y plan maestro](docs/vault/10-propuesta/analisis-y-plan.md)
- [Decisiones de arquitectura](docs/vault/20-arquitectura/)
- [Tablero de fases](docs/vault/30-fases/)
