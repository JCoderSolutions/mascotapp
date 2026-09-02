# Verificación de límites de free tier — Fase 0

**Fecha de verificación:** 2026-08-28
**Advertencia:** Los límites de free tier de estos proveedores cambian sin previo aviso. Esta tabla es una fotografía tomada en la fecha indicada arriba. Antes de ejecutar la Fase 0 (o cualquier decisión de arquitectura que dependa de estos números), volver a verificar contra las fuentes oficiales citadas — no asumir que siguen vigentes.

---

## 1. Tabla comparativa por servicio

### Google Cloud Run

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| 2M requests/mes | 2.000.000 requests/mes | [docs.cloud.google.com/free/docs/free-cloud-features](https://docs.cloud.google.com/free/docs/free-cloud-features) | confirmado |
| 180k vCPU-s | 180.000 vCPU-segundos/mes | misma fuente | confirmado |
| 360k GiB-s | 360.000 GiB-segundos de memoria/mes | misma fuente | confirmado |
| Escala a cero | Sí, por defecto (no está documentado en la tabla del plan, pero es el comportamiento estándar) | [docs.cloud.google.com/run/docs/about-instance-autoscaling](https://docs.cloud.google.com/run/docs/about-instance-autoscaling): "When a revision does not receive any traffic, by default, it is scaled to zero instances." | confirmado |
| — (no reclamado) | 1 GB de egress saliente desde Norteamérica/mes incluido en el free tier | misma fuente que las cuotas de requests | dato adicional confirmado |
| ¿Tarjeta de crédito requerida? | **Sí.** Google exige una tarjeta de crédito/débito válida para verificar identidad al crear la cuenta de facturación, aunque el uso dentro del free tier no genera cargo. | [cloud.google.com/signup-faqs](https://cloud.google.com/signup-faqs) (vía búsqueda, contenido consistente con múltiples fuentes) | confirmado — **discrepancia respecto al plan** (ver abajo) |
| Cold start | Documentado cualitativamente, sin cifra exacta. Cloud Run intenta encolar requests hasta "10 segundos o 3.5x el tiempo de cold start estimado" antes de levantar una instancia nueva, y mantiene instancias "calientes" hasta 15 minutos de inactividad para minimizar cold starts. | [docs.cloud.google.com/run/docs/about-instance-autoscaling](https://docs.cloud.google.com/run/docs/about-instance-autoscaling) | confirmado (sin número absoluto de latencia — Google no lo publica) |

### Neon (Postgres)

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| 0.5 GB storage | 0.5 GB/proyecto | [neon.com/docs/introduction/plans](https://neon.com/docs/introduction/plans) | confirmado |
| 100 CU-hours/mes | 100 CU-hours/proyecto/mes | misma fuente | confirmado |
| 5 GB transfer | 5 GB de transferencia pública ("egress") incluida, a nivel de cuenta (compartida entre todos los proyectos) | misma fuente | confirmado — nota: es por **cuenta**, no por proyecto |
| Escala a cero | Sí. Auto-suspend tras 5 minutos de inactividad, y en el plan Free **no se puede desactivar** | misma fuente | confirmado |
| ¿Tarjeta de crédito requerida? | No. "The Free plan is permanent (not a trial); no credit card required." | misma fuente | confirmado |

### Cloudflare R2

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| ~10 GB storage | 10 GB-mes/mes (Standard storage; no aplica a Infrequent Access) | [developers.cloudflare.com/r2/pricing](https://developers.cloudflare.com/r2/pricing/) | confirmado |
| 1M Class A ops | 1.000.000 operaciones Clase A/mes | misma fuente | confirmado |
| 10M Class B ops | 10.000.000 operaciones Clase B/mes | misma fuente | confirmado |
| **Zero egress** | Confirmado explícitamente: "Egressing directly from R2, including via the Workers API, S3 API, and r2.dev domains does not incur data transfer (egress) charges and is free." No hay límite/cuota de egress — es gratis siempre, no solo dentro de un tope. | misma fuente | **confirmado — punto crítico del plan validado** |

### Cloudflare Pages

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| Bandwidth ilimitado | Confirmado para **assets estáticos**: "Requests to static assets are free and unlimited" en el plan free y en el paid. No aplica a Pages Functions. | [developers.cloudflare.com/pages/platform/limits](https://developers.cloudflare.com/pages/platform/limits/) y [developers.cloudflare.com/pages/functions/pricing](https://developers.cloudflare.com/pages/functions/pricing/) | confirmado, con matiz importante (ver discrepancias) |
| Builds gratis / límite de minutos de build | **500 builds/mes** en el plan free, 1 build concurrente a la vez, timeout de 20 minutos por build. El límite real no es en "minutos acumulados" sino en cantidad de builds. | [developers.cloudflare.com/pages/platform/limits](https://developers.cloudflare.com/pages/platform/limits/) | confirmado (el plan no tenía el número exacto — ahora está documentado) |
| — (no reclamado) | Pages Functions: 100.000 requests/día, cuota **compartida** con Workers en la misma cuenta (no es exclusiva de Pages) | [developers.cloudflare.com/pages/functions/pricing](https://developers.cloudflare.com/pages/functions/pricing/) | dato adicional relevante si el proyecto usa Pages Functions además de assets estáticos |

### Resend

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| ~3.000 emails/mes | 3.000 emails/mes | [resend.com/pricing](https://resend.com/pricing) | confirmado |
| 100/día | 100 emails/día | misma fuente | confirmado |
| ¿Tarjeta de crédito requerida? | No, para el plan Free (permanente, no trial). Sí puede pedirse en un "Free Trial" separado, que no es el caso aquí. | resend.com/pricing + búsqueda cruzada, contenido consistente en múltiples fuentes secundarias | confirmado con nivel de confianza algo menor por depender parcialmente de fuentes agregadoras, no solo de la página oficial |

### Sentry

| Límite reclamado en el plan | Límite verificado | Fuente | Estado |
|---|---|---|---|
| Free developer tier (cap de errores a verificar) | **5.000 errores/mes** en el plan Developer (gratis). También incluye 5 GB de logs/mes, 5M spans (tracing), 50 session replays, 1 uptime monitor, 1 cron monitor, 20 metric monitors, 10 dashboards custom, limitado a 1 usuario. | [sentry.io/pricing](https://sentry.io/pricing/) | confirmado |
| ¿Tarjeta de crédito requerida? | No, para el plan Developer gratuito | sentry.io/pricing + búsqueda cruzada | confirmado con nivel de confianza algo menor (mismo motivo que Resend: la página oficial no lo declara de forma tan explícita como fuentes secundarias) |

---

## 2. Discrepancias encontradas

1. **Google Cloud Run — tarjeta de crédito: el plan no lo menciona, y es un requisito real.**
   Aunque el *uso* dentro del free tier de Cloud Run no genera cargos, **crear la cuenta de facturación de GCP exige registrar una tarjeta de crédito/débito válida** (para verificación de identidad, con una autorización temporal de $0–$1 que se revierte). Esto es una fricción operativa que el plan no contemplaba explícitamente. Para un proyecto que quiere "cero fricción, cero riesgo de cobro sorpresa", esto significa: (a) alguien del equipo debe poner una tarjeta personal o de la organización, y (b) hay que configurar alertas de presupuesto/facturación para evitar sorpresas si se supera el free tier por error de configuración (ej. min-instances > 0 sin querer). No cambia la viabilidad del plan, pero sí el checklist de Fase 0: agregar "dar de alta billing account con tarjeta + alerta de presupuesto en $0" como tarea explícita.

2. **Cloudflare Pages — "bandwidth ilimitado" es cierto solo para assets estáticos, no para todo el tráfico.**
   El plan dice "unlimited bandwidth" sin matiz. La fuente oficial confirma que esto aplica a requests de **assets estáticos** (HTML/CSS/JS/imágenes servidos directamente). Si el frontend usa **Pages Functions** (SSR, API routes, middleware), esas requests se facturan/cuentan como Workers y comparten una cuota de **100.000 requests/día** con cualquier otro Worker de la cuenta. Si mascotapp es un SPA/sitio estático puro consumiendo la API de Cloud Run vía fetch desde el cliente, esto no afecta nada. Si en algún momento se agregan Pages Functions (ej. para SSR o proxy de API), hay que vigilar esa cuota de 100k/día compartida.

3. **Neon — el límite de transferencia (5 GB) es por cuenta, no por proyecto.**
   El plan lo presenta como una cifra suelta sin aclarar el alcance. La documentación oficial confirma que es una cuota **compartida a nivel de cuenta** entre todos los proyectos Neon de esa cuenta. Si mascotapp usa un único proyecto Neon, no cambia nada en la práctica, pero es relevante si en el futuro se crean proyectos adicionales (ej. entornos de staging separados) bajo la misma cuenta: compiten por los mismos 5 GB.

Todo lo demás (Cloud Run: requests/vCPU-s/GiB-s/escala a cero; Neon: storage/CU-hours/auto-suspend/sin tarjeta; R2: storage/Class A/Class B/**zero egress**; Resend: 3.000/mes y 100/día; Sentry: tier gratuito con cap de errores) **coincide con lo reclamado en el plan**. En particular, el punto que el plan señala como decisivo — **R2 con egress cero, sin tope** — quedó confirmado explícitamente en la fuente oficial, sin matices ni letra chica.

---

## 3. Riesgos de agotamiento

Perfil de la app: ~20 refugios, ~50 animales por refugio (≈1.000 animales activos), ~6 fotos por animal a ~300 KB tras procesamiento, tráfico bajo (algunos miles de pageviews/mes).

### Cloud Run
- **Requests:** 2M/mes es una cuota enorme para "algunos miles de pageviews/mes". Incluso asumiendo 10 requests de API por pageview (listados, detalle, imágenes vía proxy, etc.) y 20.000 pageviews/mes, eso son ~200.000 requests/mes — 10% de la cuota. Riesgo de agotamiento: **muy bajo**, salvo bug (loop infinito, bot scraping agresivo, o un frontend mal cacheado pegándole al backend en cada render).
- **vCPU-s / GiB-s:** Con tráfico bajo y Cloud Run escalando a cero entre requests, el consumo de cómputo es proporcional a requests activos, no a tiempo de reloj. Salvo que cada request sea muy pesada (ej. procesar imágenes de 300 KB en el propio request, no en background), 180k vCPU-s/mes alcanza para muchísimos más requests que 200k. Riesgo: **bajo**, con la salvedad de que si el procesamiento de imágenes (resize a 300 KB) ocurre síncronamente en la request HTTP en vez de en un worker aparte, cada request de subida de foto puede consumir CPU desproporcionado. Vale la pena confirmar en diseño si ese procesamiento es síncrono.
- **Riesgo real más probable:** no agotar la cuota por tráfico legítimo, sino por **mala configuración** — por ejemplo, `min-instances` > 0 (mantiene una instancia siempre viva, rompiendo el "scale to zero" y potencialmente generando costo real fuera del free tier) o un bug de reintentos/polling desde el frontend.

### Neon
- **Storage (0.5 GB):** Postgres no guarda las fotos (esas van a R2), solo metadata: refugios, animales, usuarios, adopciones, etc. Con ~1.000 animales y filas relacionadas (fotos como referencias/URLs, no binarios), 0.5 GB es razonablemente holgado para este volumen — miles de filas de texto/metadata ocupan unos pocos MB, no cientos de MB. Riesgo: **bajo**, salvo que se guarden logs o auditorías extensas directamente en la misma base sin rotación.
- **CU-hours (100/mes):** Esta es la cuota más sensible del stack. Con auto-suspend a los 5 minutos, el consumo depende de cuántas "sesiones" de actividad activa haya. 100 CU-hours/mes ≈ 3.33 CU-horas/día. Con tráfico bajo y bien distribuido, es plausible que alcance, pero si el backend hace polling o mantiene conexiones abiertas que evitan el auto-suspend (ej. un health-check cada pocos segundos, o un pool de conexiones mal configurado que mantiene la compute activa), se agota rápido. **Este es el recurso con mayor riesgo real de agotamiento del stack completo** — vale la pena monitorear desde el día 1 y evitar cualquier proceso que haga keep-alive innecesario contra la DB.
- **Egress (5 GB/mes):** Solo metadata viaja por Postgres (las imágenes van directo por R2), así que 5 GB de transferencia de datos de Postgres es mucho margen. Riesgo: **muy bajo**.

### Cloudflare R2
- **Storage (10 GB):** 1.000 animales × 6 fotos × 300 KB ≈ 1.8 GB. Con margen para crecimiento (más refugios, más animales, fotos de perfil de usuarios, etc.), 10 GB da margen para ~3.000 animales en las mismas condiciones antes de tocar el límite. Riesgo: **bajo a mediano plazo**, pero es el primer límite del stack que probablemente se toque si el proyecto crece más allá de lo estimado — vale la pena tener un plan de qué pasa al superarlo (R2 sigue siendo muy barato pasado el free tier, así que no es bloqueante, solo deja de ser gratis).
- **Class A ops (1M, escrituras/listados):** Cada subida de foto es al menos 1 operación Class A (PUT). Con 6.000 fotos totales en el catálogo inicial más operaciones de reemplazo/edición, esto es una fracción mínima del millón. Riesgo: **muy bajo**, salvo un flujo de subida mal implementado que genere múltiples PUTs redundantes por foto (ej. reintentos sin idempotencia, o thumbnails múltiples generados con muchas escrituras cada uno).
- **Class B ops (10M, lecturas):** Cada vista de foto en el frontend es una Class B (GET), salvo que haya cache (Cloudflare cachea en el edge por defecto para R2 servido vía Workers/dominio público). Con miles de pageviews/mes y varias fotos por página, el consumo es bajo comparado con 10M. Riesgo: **muy bajo**.
- **Egress:** Sin límite (gratis siempre). **Sin riesgo de agotamiento por definición** — este es el motivo por el que R2 fue elegido y la verificación lo confirma.

### Cloudflare Pages
- **Builds (500/mes):** Cada deploy (push a la rama de producción, o cada PR si hay preview deploys) consume un build. Con desarrollo activo en la Fase 0/1, es plausible hacer varios deploys por día, pero llegar a 500/mes (≈16/día) requeriría un ritmo de commits/deploys muy alto sostenido todo el mes. Riesgo: **bajo**, salvo CI mal configurado que dispare builds en cada push a cualquier rama en vez de solo a la rama de producción/PRs relevantes.
- **Requests (assets estáticos ilimitados; Functions 100k/día compartido):** Si el sitio es estático puro, no hay techo práctico. Si se agregan Pages Functions, 100k/día es generoso para "algunos miles de pageviews/mes", pero está compartido con cualquier otro Worker de la cuenta — vale la pena tenerlo en cuenta si más adelante se agregan Workers para otras cosas (webhooks, cron jobs, etc.) bajo la misma cuenta de Cloudflare.

### Resend
- **3.000 emails/mes, 100/día:** Los emails de esta app son transaccionales: magic links de login y notificaciones (ej. "tu solicitud de adopción fue aceptada"). Con ~20 refugios gestionando el catálogo y adoptantes autenticándose ocasionalmente, es difícil que el volumen total de la plataforma supere 100 emails/día en esta etapa temprana, pero **es el límite diario más ajustado del stack en términos relativos**: si en un día hay una campaña de difusión, un refugio hace un envío masivo de notificaciones, o hay un pico de registros/logins con magic link, 100/día se puede tocar. Riesgo: **bajo a mediano** — vale la pena diseñar el flujo de magic link para que no dispare emails redundantes (ej. no reenviar automáticamente si el usuario refresca la página) y no usar email para notificaciones masivas no esenciales.

### Sentry
- **5.000 errores/mes:** Este es, junto con las CU-hours de Neon, el otro límite con riesgo real de agotarse **no por uso real de usuarios, sino por errores repetitivos no controlados** (ej. un bug con un error que se dispara en cada request a un endpoint roto, o un error de frontend que se dispara en cada render de un componente mal parcheado). Con tráfico bajo, errores legítimos y esporádicos de una app en producción temprana probablemente se mantengan muy por debajo de 5.000/mes, pero un solo bug no capturado/no rate-limited puede agotar la cuota mensual en horas. Riesgo: **mediano** — no por el perfil de uso, sino por la ausencia de rate-limiting de errores repetidos (Sentry tiene fingerprinting/agrupación, pero cada ocurrencia individual sigue contando contra la cuota salvo que se configure explícitamente un sample rate o rate limit).

---

## 4. Resumen de confianza de la verificación

- **Confirmado con fuente oficial directa (alta confianza):** Cloud Run (requests, vCPU-s, GiB-s, escala a cero, egress de 1GB, requisito de tarjeta), Neon (storage, CU-hours, egress, auto-suspend, sin tarjeta), R2 (storage, Class A, Class B, egress cero), Cloudflare Pages (builds, límite de requests estáticos vs. Functions), Resend (3.000/mes, 100/día), Sentry (5.000 errores/mes y demás cuotas del plan Developer).
- **Confirmado pero con menor confianza (fuente oficial no explícita sobre tarjeta de crédito, corroborado por fuentes secundarias consistentes):** "sin tarjeta de crédito requerida" para Resend y para Sentry. Si esto es crítico para la decisión de Fase 0, recomiendo confirmarlo manualmente haciendo el signup real antes de comprometerse, ya que ninguna de las dos páginas de pricing oficiales lo declara con la misma explicitud que Neon.
- **Nada quedó marcado como `UNVERIFIED`** — todos los números centrales del plan pudieron confirmarse contra al menos una fuente oficial (`cloud.google.com`/`docs.cloud.google.com`, `neon.com`, `developers.cloudflare.com`, `resend.com`, `sentry.io`).
