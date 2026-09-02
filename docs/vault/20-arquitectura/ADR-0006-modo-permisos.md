# ADR-0006 — Modo de permisos y supervisión de agentes

- **Fecha:** 2026-08-28
- **Estado:** aceptado
- **Fase:** 00

## Contexto

El trabajo lo ejecutan agentes autónomos. Hacía falta saber **qué se hace cumplir
de verdad** y qué es ilusión, en vez de asumirlo.

## Lo verificado contra la documentación oficial

| Mecanismo | ¿Funciona en `bypassPermissions`? |
|---|---|
| Reglas **`deny`** | **Sí, en todos los modos** |
| Reglas **`ask`** | **Sí — fuerzan prompt al humano** |
| Reglas **`allow`** | **No tienen ningún efecto** |
| Cortacircuitos `rm` en critical path | Sí, pregunta siempre |
| Escritura a rutas protegidas (`.git`, `.claude`) | Permitida |
| Bloqueos del **modo plan** | **No se aplican** |
| Hooks `PreToolUse` | **No documentado** — pendiente en T-00-018 |

Comprobado en la práctica, no supuesto:
- `rm apps/web/src/App.tsx` → **denegado**
- `Edit .claude/settings.json` → **denegado** (el agente no puede tocar sus propias barreras)

## Decisión

**Capa 0 — el contenedor es la única barrera real.** El devcontainer monta
**solo el repositorio**. Verificado: desde adentro, `C:\Users\Jose` no existe.

**Capa 1 — `deny`** para lo destructivo. **Capa 2 — `ask`** para lo irreversible:
lo que sale de la máquina (push, PR, deploy), toca datos reales (migraciones, psql)
o incorpora código de terceros (cadena de suministro).

**Modo recomendado en el host: `auto`, no `bypassPermissions`.**
`auto` da caudal casi igual pero pasa cada acción por un clasificador que ya
bloquea force-push, borrar recursos con estado, repuntar URLs de registries y
comentar tests de seguridad. `bypassPermissions` no tiene nada de eso.
`bypass` tiene un lugar legítimo: **dentro del devcontainer**, donde el
contenedor acota el daño.

## El límite honesto

**Las reglas comparan la cadena del comando, no su efecto.** Un `make clean` que
internamente corre `rm -rf` **no se bloquea**. Eso convierte a las capas 1 y 2 en
**barandas contra error del modelo, no en un sandbox**.

Por eso el borrado legítimo vive en targets del `Makefile`: el agente no decide
qué borrar, ejecuta un borrado versionado y revisado. La diferencia es confiar en
el criterio del modelo versus confiar en un artefacto que un humano aprobó.

## Consecuencias

- Un agente **no puede ampliarse los permisos**: `Edit(./.claude/settings.json)`
  está denegado. Cambiar la política requiere una mano humana, a propósito.
- Costo aceptado: el agente tampoco puede borrar archivos muertos. Por eso existe
  `.trash/` (cuarentena, gitignored) y la tarea T-00-023.
- **Trampa encontrada:** las reglas Bash **sin `*` final son coincidencia exacta**.
  `Bash(rm -rf /)` no cubre `rm -rf ./apps`. Ver `settings-propuesto.json`.

## Condiciones de reapertura

Que T-00-018 demuestre que los hooks `PreToolUse` sí disparan en bypass — eso
habilitaría resolución de rutas real ("dentro del repo") en vez de patrones,
que es la única forma de expresar esa política correctamente.
