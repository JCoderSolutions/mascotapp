# MascotApp

Plataforma multi-refugio de adopción de animales. Los refugios publican sus animales, arman
sus propios formularios de solicitud y llevan el proceso de adopción de punta a punta; el
adoptante ve el estado real de su solicitud en todo momento.

Sin fines de lucro, y desplegada a **costo cero** — una restricción de diseño, no un detalle
([ADR-0003](docs/vault/20-arquitectura/ADR-0003-costo-cero.md)).

> **Estado: en desarrollo.** Fases 00 y 01 cerradas (fundaciones, modelo de datos y RLS);
> Fase 02 en curso (autenticación y multi-tenancy). El esquema tiene aislamiento por refugio
> **probado tabla por tabla contra un PostgreSQL real**, no contra mocks.
>
> Todavía **no hay demo pública**: el catálogo público es la Fase 06. Lo que hay para ver hoy
> es el backend, las decisiones de arquitectura y cómo se trabaja — que es de lo que trata el
> resto de este documento.

---

## El problema real

No es un CRUD de perritos. Los tres problemas que definen la arquitectura son:

1. **Aislamiento multi-tenant verificable.** Varios refugios en una base compartida, sin que
   un `WHERE shelter_id = ?` olvidado filtre datos ajenos.
2. **Datos personales regulados.** Una solicitud de adopción lleva domicilio, documento y
   referencias. Eso es PII, no texto libre.
3. **Confianza.** Publicar refugios sin verificar convierte la plataforma en vehículo de
   estafas de "cuota de adopción".

---

## Decisiones que vale la pena leer

Cada una está fechada, con las alternativas que se descartaron y por qué.

| Decisión | El punto |
|---|---|
| [**Row Level Security sobre discriminador**](docs/vault/20-arquitectura/ADR-0002-multi-tenancy.md) | Un `WHERE` olvidado es cuestión de tiempo. Con RLS ese olvido devuelve **cero filas**, no las de otro refugio. La base es la última línea de defensa y no depende de la disciplina de quien escribe el SQL |
| [**`WithTenant` es la única puerta**](docs/vault/20-arquitectura/ADR-0008-withtenant-es-la-unica-puerta.md) | El scope se fija entre `BEGIN` y `COMMIT`, nunca en el pool: `pgxpool` recicla conexiones físicas, y un setting de sesión sobreviviría al reciclado para que el próximo request herede el tenant anterior |
| [**TLS sí, mTLS no**](docs/vault/20-arquitectura/ADR-0004-tls-sin-mtls.md) | Análisis canal por canal de por qué mTLS acá sería seguridad de culto al cargo — con las **condiciones que reabrirían la decisión**, escritas de antemano |
| [**Append-only y sus cuatro capas**](docs/vault/20-arquitectura/ADR-0010-tablas-append-only-y-sus-cuatro-capas.md) | "Append-only por convención" no es append-only. Grants, triggers, policies y tests, y qué cubre cada capa que las otras no |
| [**El presupuesto de revisión que estaba mal medido**](docs/vault/20-arquitectura/diagnostico-presupuesto-400.md) | Siete PRs seguidos rompieron el límite de 400 líneas. En vez de subirlo, se diagnosticó por qué la regla medía la cosa equivocada |

Los once ADRs están en [`docs/vault/20-arquitectura/`](docs/vault/20-arquitectura/).

---

## Cómo se trabaja acá

**TDD estricto.** Toda tarea de código empieza con un test que falla — y el RED se verifica,
normalmente como error de compilación, no como costumbre.

**Mutation testing en todo lo crítico.** Un test verde solo prueba que corrió. Se rompe la
implementación a propósito y se exige que un test muera; el mutante que **nadie** mata es un
test que no estaba midiendo nada. Tres veces en esta fase la mutación corrigió al autor, no al
código heredado.

**Revisión ciega dual antes de los merges irreversibles.** Dos revisores independientes, en
paralelo, sobre un target congelado.

> 📌 **[Judgment Day sobre la rotación de refresh tokens](docs/vault/20-arquitectura/judgment-day-pr-02-11.md)** — encontró un defecto de
> seguridad **real**: bajo `READ COMMITTED`, la revocación de una familia de tokens no era
> atómica, así que un sucesor insertado por una rotación concurrente sobrevivía a la
> revocación de su propia familia. Para siempre.
>
> La primera ronda de corrección **falló**, y el ledger dice exactamente por qué: se propuso
> un índice único parcial, que hace cumplir un invariante *dentro* de una transacción cuando
> el problema era una carrera *entre* dos. Se cerró en la segunda ronda con un advisory lock
> por familia — 125 trials en verde, y quitando el lock fallan las tres corridas.

Hubo dos hasta ahora, y las dos encontraron algo:
[sobre las políticas RLS](docs/vault/40-bitacora/2026-08-30-judgment-day-rls.md) y sobre la
rotación de tokens.

**La bitácora registra los errores, no solo los aciertos** —
[`docs/vault/40-bitacora/`](docs/vault/40-bitacora/) — incluidas las advertencias que resultaron
falsas y quedaron tachadas en vez de borradas. Un ejemplo:
[una defensa CSRF correcta en general que habría rechazado el 100% del tráfico legítimo](docs/vault/40-bitacora/2026-09-06-t-02-025-cors-csrf.md).

**CI bloqueante:** build · `go vet` · `gofmt` · tests con race detector · `golangci-lint` ·
`govulncheck` · verificación de que el código generado está al día · lint, typecheck, tests y
build del frontend.

---

## Continuidad entre sesiones de agentes

Este repo está construido para que el trabajo lo ejecuten agentes de IA que **pierden contexto
entre sesiones**. Eso es una restricción de arquitectura, y se resuelve con estado explícito:

- [`PROJECT_STATE.md`](PROJECT_STATE.md) — frontmatter machine-readable con la tarea actual,
  qué la bloquea y la acción siguiente. Fuente única de "dónde vamos".
- `docs/vault/30-fases/` — tablero de tareas con IDs estables (`T-<fase>-<n>`), nunca
  reutilizados. Invariante duro: **como máximo una tarea en `[~]`** en todo el tablero.
- `.engram/queue/` — sala de espera de candidatos a memoria, con rúbrica de puntuación y
  aprobación humana explícita. Nada entra a la memoria de largo plazo sin que un humano diga sí.
- `docs/vault/70-conocimiento/` — lo aprobado, ya versionado. Una nota por decisión, con la
  regla, la trampa que la hace no obvia, y cómo se descubrió.

El objetivo es que un agente en frío retome la última tarea sin adivinar. Si sos un agente y
esta es tu primera sesión: **empezá por [`PROJECT_STATE.md`](PROJECT_STATE.md)**.

---

## Stack

**Backend** — Go 1.27 · [chi](https://github.com/go-chi/chi) · [sqlc](https://sqlc.dev) + pgx/v5 ·
PostgreSQL · [goose](https://github.com/pressly/goose) · JWT propio + Argon2id + TOTP sobre stdlib

**Frontend** — React 19 · Vite 8 · TypeScript 6 (strict) · TanStack Router + Query ·
Tailwind + shadcn/ui

**Infra (free tier)** — Cloud Run · Neon · Cloudflare R2 + Pages · Resend

El contrato de API vive en [`api/openapi.yaml`](api/openapi.yaml) y **genera los tipos de los dos
lados**: Go en el servidor, TypeScript en el cliente. Un solo documento, dos lados tipados, y CI
falla si el generado quedó desactualizado.

---

## Correrlo local

Necesitás Docker y Go 1.27+.

```bash
make doctor   # Preflight: ¿está cada herramienta presente Y ejecutable acá?
make dev      # Levanta Postgres + MinIO + API + web
make test     # Suite completa (Go + web)
make lint     # Todos los linters
```

Los tests de base de datos usan [testcontainers](https://testcontainers.com): levantan un
PostgreSQL real, corren las migraciones y prueban las políticas RLS contra el motor de verdad,
no contra un mock.

---

## Estructura

```
PROJECT_STATE.md          Dónde vamos. Fuente única de continuidad
api/openapi.yaml          Contrato de API — genera tipos Go y TS
apps/api/                 Backend Go
  internal/db/            Migraciones, queries sqlc, wrappers de transacción
  internal/auth/          Argon2id, TOTP, JWT, sesiones, códigos de recuperación
  internal/httpapi/       Router, middleware, CORS/CSRF
apps/web/                 Frontend React
packages/                 Tipos compartidos Go <-> TS
docs/vault/               ADRs, specs, tablero de fases y bitácora (vault de Obsidian)
```

---

## Reglas del proyecto

1. **TDD estricto.** Ninguna tarea pasa a hecha sin RED→GREEN verificado.
2. **Artefactos técnicos en inglés** (código, columnas, endpoints, commits, tests). La
   documentación del vault, en español.
3. **La plataforma no procesa pagos.** Solo enlaza a links de pago propios del refugio,
   verificados en el onboarding. Alcance PCI: cero.
4. **Ningún refugio publica sin verificación manual previa.** Es requisito de MVP, no de fase 2.
5. **La IA del producto nunca decide sobre personas.** Prioriza y explica; jamás rechaza.

---

## Licencia

[MIT](LICENSE) — Jose Quintero, 2026.
