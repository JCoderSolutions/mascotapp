# ADR-0005 — Piloto de OpenCode con modelos gratuitos

- **Fecha:** 2026-08-28
- **Estado:** aceptado (configuración lista; **el piloto NO se ejecuta hasta Fase 06**)
- **Fase:** 00 (config) / 06 (ejecución)

## Contexto

Se quiere evaluar si modelos gratuitos, vía OpenCode, pueden absorber trabajo
mecánico sin generar retrabajo. El requisito explícito fue: probarlo **de manera
controlada**, sin que cueste más corregir que hacer.

## Decisión

Se configura OpenCode (`opencode.json`) con **doble proveedor y permisos por ruta**,
pero el piloto se ejecuta **solo en Fase 06**, en rama aislada, con criterios de
continuación y de muerte **escritos por adelantado**.

## El criterio de aptitud

La premisa "modelos gratis para tareas sencillas" es subjetiva y garantiza el
retrabajo que se quiere evitar. El criterio real es:

> Una tarea es apta **solo si su corrección puede verificarse por completo, de
> forma automática y objetiva, con una comprobación que ya existe antes de empezar.**
>
> Si hay que **leer** la salida para saber si está bien, **no es apta**.

La lista blanca de `permission.edit` en `opencode.json` es esa regla hecha código:
i18n, stories, tests de funciones ya especificadas, seeds, bitácora.

La lista negra es igual de deliberada: dominio, migraciones, auth, middleware,
el contrato OpenAPI, todo lo generado, y todo el estado del proyecto
(`PROJECT_STATE.md`, `.engram/`, `openspec/`, tableros y ADRs).

## Hallazgos que forzaron el diseño (verificados, agosto 2026)

| Hallazgo | Consecuencia |
|---|---|
| El endpoint `:free` de Qwen3 Coder **desapareció** a mediados de 2026 | **Nunca atar el flujo a un modelo.** Doble proveedor con fallback |
| OpenRouter free: ~20 req/min, **200 req/día** | Una tarea agéntica son 30–80 llamadas → 3–5 tareas diarias. Si se corta a mitad, queda estado sucio |
| Groq free: 30 RPM, 14.4k RPD, pero TPM bajo | Más volumen, peor con contextos grandes |
| El costo real no es la inferencia, es **la revisión** | Código plausible-pero-incorrecto cuesta más revisarlo que escribirlo |

## Protocolo del piloto

| Parámetro | Valor |
|---|---|
| Fase | **06 — Catálogo público** (alto volumen mecánico, bajo riesgo, buena cobertura) |
| Aislamiento | Rama `experiment/opencode-free`. **Nunca directo a `main`** |
| Muestra | 10 tareas de la lista blanca |
| Registro | `docs/vault/40-bitacora/opencode-pilot.md` |
| Métricas | `first_pass_rate` · `rework_minutes` (mediana) · `wall_clock` vs. estimación directa |
| **Continuar si** | `first_pass_rate >= 70%` **y** mediana de `rework_minutes <= 5` |
| **Abortar si** | Dos tareas consecutivas cuestan más corregir que ejecutar. **Sin negociación** |

**Un piloto sin números registrados se considera fallido, no inconcluso.**

## Consecuencias

- Las claves (`OPENROUTER_API_KEY`, `GROQ_API_KEY`) van **solo en variables de
  entorno**. `gitleaks` en CI cubre el caso de que alguien las commitee.
- OpenCode **no escribe** estado ni memoria: `PROJECT_STATE.md`, `.engram/`,
  `openspec/`, tableros y ADRs están denegados. Produce código; Claude y Gentle AI
  mantienen el estado. **Una sola mano en el timón.**
- Si el piloto se aborta, se registra acá y **no se vuelve a discutir esta iteración**.

## Condiciones de reapertura

Que aparezca un modelo gratuito con límites de tasa compatibles con trabajo
agéntico sostenido (>1.000 req/día), o que el criterio de verificabilidad
automática pueda extenderse a más clases de tarea.
