# FASE 11 — Endurecimiento y lanzamiento

**Objetivo:** Que la app sea usable por todo el mundo y sobreviva a producción.

**Entregable verificable:** Auditoría WCAG 2.2 AA, i18n, presupuesto de rendimiento (LCP < 2.5s en 3G, p95 < 300ms), pentest, runbooks, E2E Playwright completos.

**RDD:** ✅ recomendado
**Depende de:** 10

Estados: `[ ]` pendiente · `[~]` en progreso · `[x]` hecha · `[!]` bloqueada

---

## Tareas

> Esta fase **se expande al nivel de tarea al iniciarla**, no antes.
> Expandir las 12 fases hoy produce tareas obsoletas.
>
> Al abrir la fase: correr `/sdd-new fase-11` → `/sdd-ff`, y volcar acá el
> checklist de `tasks.md` con IDs `T-11-NNN`.

- [ ] **T-11-001** · (pendiente de expansión)

### Arrastres ya identificados

Se anotan acá porque nacieron en otra fase y **no se pueden cerrar hasta desplegar**.
Al expandir la fase, convertirlos en tareas con ID propio.

- [ ] **Fijar `TRUSTED_PROXIES` con los rangos reales del front-end** — viene de
      [[FASE-00]] T-00-022. El resolver de IP de cliente está escrito y probado,
      pero su set de proxies confiables **está vacío por defecto**, así que hoy
      toda petición resuelve al peer TCP.
      - riesgo si se olvida: el rate limiting por IP de Fase 02 mete a **todos**
        los usuarios en el mismo bucket (el del front-end de Cloud Run), y el
        `ip_hash` del audit log guarda siempre la misma dirección. Falla cerrada
        —nunca acepta una IP falsificada— pero queda inútil
      - dod: rangos determinados **contra la documentación vigente de Google al
        momento de desplegar**, no de memoria · configurados en el entorno de
        Cloud Run · **verificados contra tráfico real**: una petición desde una
        IP conocida debe resolver a esa IP, no al proxy
      - nota: si el despliegue termina detrás de un Global External ALB, los
        rangos son los del balanceador, no los del front-end de Cloud Run.
        El algoritmo (primera entrada no confiable desde la derecha) soporta
        ambas topologías sin cambios de código; solo cambia la configuración

---

## Salida de fase

1. Todas las tareas en `[x]`.
2. Verificación de fase del plan maestro ejecutada y registrada.
3. Cola de Engram vacía o aprobada.
4. `/sdd-verify` en verde → `/sdd-archive`.

Siguiente: [[FASE-12]]
