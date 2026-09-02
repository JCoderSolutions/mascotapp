# FASE 06 — Catálogo público

**Objetivo:** La experiencia del adoptante: encontrar al animal correcto y que Google lo indexe.

**Entregable verificable:** Listado y ficha, filtros tipados, paginación, SEO/JSON-LD/OpenGraph, rol `app_public` de solo lectura. **⚑ Acá corre el piloto de OpenCode (§7.2 del plan).**

**RDD:** —
**Depende de:** 05

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Alcance heredado de Fase 01 — NO se expande, ya está decidido

Arrastrado el **2026-09-01** al cerrar `T-01-035`. Los dos tocan el catálogo público, que es lo que
esta fase construye.

> **DECISIÓN DEL USUARIO, 2026-09-01: LT-2 aterriza en ESTA fase.** La condición de verificación
> de refugios entra acá, junto con la política de `app_public` sobre `shelters` — porque la
> condición la **necesita**: el subquery `EXISTS` de una política exige que el rol tenga `SELECT`
> sobre la tabla referenciada, así que agregar la condición sola **no angosta el catálogo, lo
> rompe** con un error de permisos en toda lectura pública.
>
> **Por qué acá y no en Fase 10, que es donde vive el flujo de verificación:** Fase 10 es
> **post-MVP** (el corte está al final de la Fase 08). Diferirla ahí significaba lanzar el MVP con
> el vector de estafa de §1.1 abierto. Mientras tanto, verificar un refugio es un `UPDATE` manual;
> la interfaz sigue siendo Fase 10.
>
> `TestPublicCatalog_DoesNotYetEnforceShelterVerification` es el test de caracterización que se
> **invierte** ese día, y afirma que las dos mitades solo pueden aterrizar juntas.

### 1. `app_public` no tiene política sobre `shelters`

El diseño lista `shelters` entre las tablas públicas; el tablero de Fase 01 asignó políticas
públicas solo a `pets` y `media`. T-01-013 decidió **no inventar** un acceso de lectura que ningún
test cubre — y un grant sin su política es el estado más ancho posible, no el más seguro.

Esta fase necesita datos del refugio para la ficha pública, así que el hueco se cierra acá.
**Y se ata con LT-2:** cuando la condición de verificación aterrice, la política de `pets` va a
necesitar leer `shelters`, y el subquery `EXISTS` de una política exige que el rol tenga `SELECT`
sobre la tabla referenciada (T-01-020, mutante M13). O sea que **las dos mitades tienen que
aterrizar juntas** — agregar la condición sola no angosta el catálogo, lo **rompe** con un error de
permisos en toda lectura pública. `TestPublicCatalog_DoesNotYetEnforceShelterVerification` es el
test de caracterización que lo dice y se invierte ese día.

### 2. `pets.breed_id` permite una raza de otra especie

La FK es de una columna a `breeds (id)`, así que `species_id = gato` con `breed_id = Labrador` es
representable. **No es frontera de seguridad**: corrompe el **filtro tipado del adoptante** (AD-3),
que es justamente lo que esta fase construye.

El arreglo está escrito y es barato — `UNIQUE (id, species_id)` en `breeds` más una FK compuesta
desde `pets` — pero es una migración nueva que corrige una cerrada, y conviene que aterrice junto al
código que la necesita.

## Tareas

> Esta fase **se expande al nivel de tarea al iniciarla**, no antes.
> Expandir las 12 fases hoy produce tareas obsoletas.
>
> Al abrir la fase: correr `/sdd-new fase-06` → `/sdd-ff`, y volcar acá el
> checklist de `tasks.md` con IDs `T-06-NNN`.

- [ ] **T-06-001** · (pendiente de expansión)

---

## Salida de fase

1. Todas las tareas en `[x]`.
2. Verificación de fase del plan maestro ejecutada y registrada.
3. Cola de Engram vacía o aprobada.
4. `/sdd-verify` en verde → `/sdd-archive`.

Siguiente: [[FASE-07]]
