# ADR-0001 — Stack tecnológico

- **Fecha:** 2026-08-28
- **Estado:** aceptado
- **Fase:** 00

## Contexto

Se necesita un stack ligero, rápido y moderno que sobreviva en free tier con escalado
a cero, para una app de refugios con mucha imagen y poco tráfico. La propuesta delegó
la elección explícitamente ("las tecnologías serán propuestas por ti").

## Decisión

**Backend:** Go 1.23+ · `chi` v5 · `sqlc` + `pgx/v5` · `goose` · `maroto/v2` para PDF ·
`log/slog` en JSON.

**Frontend:** React 19 + Vite + TypeScript strict · TanStack Router/Query ·
React Hook Form + Zod · Tailwind v4 + shadcn/ui.

**Contrato:** OpenAPI 3.1 como fuente única, generando servidor (Go) y cliente (TS).

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| Node/TS en el backend | Arranque en frío y huella de memoria peores que un binario Go en escalado a cero |
| ORM (GORM, Ent) | Planes de ejecución impredecibles; `sqlc` da SQL explícito con structs tipadas |
| PDF vía navegador headless | Demasiado pesado para el free tier de Cloud Run |
| Next.js + Vercel | Acopla el frontend a un proveedor; Vite + Pages es más portable |

## Consecuencias

- **A favor:** binario único, ~300 ms de arranque en frío, ~20 MB en reposo. Es lo que
  hace viable el costo cero.
- **En contra:** Go tiene menos azúcar que TS para el dominio; hay que escribir más
  código explícito. Se acepta a cambio de previsibilidad.
- `sqlc` obliga a escribir SQL a mano. Es deliberado: el SQL es el contrato con la base.

## Condiciones de reapertura

Que el tráfico supere el free tier de Cloud Run de forma sostenida, o que aparezca una
necesidad de tiempo real a escala que justifique otra topología.
