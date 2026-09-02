# ADR-0003 — Despliegue a costo cero

- **Fecha:** 2026-08-28
- **Estado:** aceptado
- **Fase:** 00

## Contexto

El proyecto apoya a refugios sin fines de lucro. No se quiere cobrar en primera
instancia, así que el despliegue debe caber en free tiers.

## Decisión

| Servicio | Proveedor | Plan de degradación |
|---|---|---|
| API Go | Google Cloud Run (escala a cero) | Koyeb o Render |
| Postgres | Neon (escala a cero) | Supabase |
| Objetos e imágenes | Cloudflare R2 (**egreso cero**) | Insustituible |
| Frontend | Cloudflare Pages | Vercel Hobby |
| Correo | Resend | Brevo |
| Errores | Sentry | Opcional |

Techo de costo aceptado: **USD 20–40 al año**, dominio incluido.

## Consecuencias

- **Arranque en frío de 1–2 s** en la primera petición tras inactividad, porque Neon y
  Cloud Run escalan a cero. **Se le dice a los refugios; no se oculta.**
- Mitigación opcional: cron gratuito de GitHub Actions golpeando `/healthz` cada 10 min
  en horario hábil.
- R2 es la única pieza sin sustituto real: el egreso cero es lo que hace viable una app
  con mucha imagen.
- **Ninguna arquitectura acoplada a un proveedor.** Cada servicio tiene salida documentada.

## Riesgo aceptado

"Gratis para siempre" es falso: dominio, correo, crecimiento de imágenes y generación de
PDF tienen costo. Por eso hay techo explícito y plan de degradación por servicio.

## Nota de vigencia

Los límites de free tier de este ADR son de **agosto de 2026** y cambian.
La tarea T-00-012 los verifica y registra en `free-tier-limits.md` antes de comprometerse.

## Condiciones de reapertura

Que las donaciones permitan sostener infraestructura pagada, o que se agote un free tier
de forma sostenida.

---

## Actualización — verificación del 2026-08-28 (T-00-012)

Los seis servicios se verificaron contra fuentes oficiales. Detalle completo en
[[free-tier-limits]]. **24 valores confirmados, ninguno sin verificar.**

### Correcciones a este ADR

| Servicio | Lo que decía este ADR | Realidad verificada |
|---|---|---|
| Cloudflare Pages | "builds y ancho de banda sin costo" | **500 builds/mes**, no ilimitados. El ancho de banda ilimitado aplica **solo a assets estáticos**; Pages Functions comparte una cuota de 100k req/día con Workers |
| Cloud Run | (no se mencionaba) | **Requiere tarjeta de crédito** para verificar la cuenta de facturación (hold temporal de USD 0–1, reversado). El uso dentro del free tier no se cobra |
| Neon | "escala a cero" | **Auto-suspend a los 5 minutos** de inactividad, **no desactivable** en el plan Free. Los 5 GB de egreso son **por cuenta**, no por proyecto |
| Sentry | "free tier" | **5.000 errores/mes** en el plan Developer |

### Consecuencias

- **El frontend debe ser SPA estática pura**, golpeando la API de Cloud Run directamente.
  En cuanto se agregue SSR o Pages Functions, entra la cuota de 100k req/día y el
  "ancho de banda ilimitado" deja de aplicar. Es una restricción de arquitectura, no un detalle.
- **500 builds/mes** obliga a no encadenar despliegues automáticos en cada push a rama.
  Deploy solo desde `main`.
- Alguien tiene que poner una tarjeta para GCP. Ver nueva tarea **T-00-020**.

### Corrección importante al plan de mitigación de arranque en frío

Este ADR proponía un cron que golpee `/healthz` cada 10 minutos. Con los datos verificados:

- Neon suspende a los **5 minutos**, así que un ping de 10 minutos **no lo mantiene despierto** igual.
- Y si el ping **tocara la base**, quemaría CU-hours — que junto con los errores de Sentry son
  **los dos límites más ajustados del stack**.

**Decisión:** el cron golpea **solo `/healthz`**, que es liveness y **no toca la base
por diseño**. Mantiene tibio Cloud Run y deja que Neon suspenda. `/readyz` sí toca
dependencias, y por eso **nunca** debe ser el objetivo del keep-alive.

### Riesgo real de agotamiento

No es el tráfico esperado. Son **los bugs**: un polling que impida el auto-suspend de Neon,
o un error repetitivo que inunde Sentry. Ambos límites se queman por defecto de código,
no por éxito del producto.
