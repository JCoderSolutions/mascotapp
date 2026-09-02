---
type: constraint
score: 5
topic_key: mascotapp/ops/neon-pool-config-cost
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-0a07268799d53b37
task: T-01-004 (construccion de pools)
rationale: "Los defaults de pgxpool son razonables en cualquier Postgres y ruinosos en Neon free. El costo no aparece como error: aparece en la factura, o como un proyecto que deja de funcionar a mitad de mes."
---

**En Neon free, la configuración del pool es un contrato de COSTO, no una preferencia de
tuning. Los defaults de `pgxpool` agotan el free tier.**

Hechos verificados contra la documentación oficial de Neon el 2026-08-29:

1. El plan free suspende la compute a los **5 minutos** de inactividad y **no permite
   desactivar** Scale to Zero.
2. Neon **cierra las conexiones ociosas** por su cuenta. Una conexión abierta no impide el
   suspend — pero sí queda muerta del lado del pool.
3. El free tier da **100 CU-hours al mes**, unas 3,3 horas por día. Es el recurso más
   agotable de todo el stack, por encima de storage o requests.

## Lo que hacen los defaults de pgxpool

| Ajuste | Default | Problema en Neon free |
|---|---|---|
| `MaxConnIdleTime` | **30 min** | Seis veces la ventana de suspend. Deja sockets en el pool que Neon ya cerró |
| `MinConns` / `MinIdleConns` | 0 | Correcto — **pero subirlos a 1 es el error más caro disponible** |

Un `MinConns = 1` mantiene una conexión abierta para siempre. La compute nunca suspende, y
las 100 CU-hours del mes se gastan **en una base de datos ociosa**, en un par de días.

Lo brutal es que no falla. No hay error, no hay test rojo, no hay log. Simplemente el
proyecto deja de andar a mitad de mes.

## La configuración

```go
const NeonAutoSuspendAfter = 5 * time.Minute  // externo, no es una perilla

cfg.MinConns = 0        // explícito aunque ya sea el default
cfg.MinIdleConns = 0
cfg.MaxConnIdleTime = 2 * time.Minute   // cerrar ANTES que Neon
cfg.MaxConns = 4                        // por proceso; Cloud Run corre varias instancias
```

Los tests afirman cada uno de estos valores **con el porqué escrito al lado**, y hay un test
que fija `NeonAutoSuspendAfter` en 5 minutos, para que subirlo y hacer pasar otro test falle
ruidosamente. Verificado por mutación: `MinConns = 1` y `MaxConnIdleTime = 30 min` los dos
matan un test.

## Una hipótesis que resultó FALSA, y por qué importa

Sospeché que el health check de fondo de `pgxpool` (cada 1 minuto por defecto) era un
keep-alive que derrotaría el autosuspend — que es **exactamente** la trampa que la guía de
Ecto de Neon describe para el `idle_interval` de Postgrex.

Leí `checkConnsHealth` en pgx v5.10.0: solo inspecciona estado **local** (duración de ocio,
expiración) y destruye conexiones localmente. **Cero round trips de red.** No es keep-alive.

Se quedó en su default. La lección: la analogía con Postgrex era buena y la conclusión era
incorrecta. Un ajuste "por las dudas" acá habría sido cambiar código en base a una corazonada
y después defenderlo como si fuera un hallazgo.

Relacionado: [[free-tier-limits]], [[2026-08-29-neon-bypassrls-defeats-policies]].
