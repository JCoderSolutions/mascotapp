---
type: constraint
score: 5
topic_key: mascotapp/security/neon-rls-bypass
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-ff9448b3c1505e50
task: phase-01-domain-and-data (sdd-propose)
rationale: "Es la forma más silenciosa de romper todo el aislamiento entre refugios: se rompe en producción y los tests siguen en verde. Nadie lo adivina leyendo el código."
---

En Neon, **crear el rol de aplicación desde la consola, el CLI o la API desactiva
todas las políticas RLS**, en silencio.

La cadena, verificada contra la documentación oficial de Neon el 2026-08-29:

1. El rol `neon_superuser` **tiene el atributo `BYPASSRLS`**, que permite a sus
   miembros saltarse row-level security por completo.
2. Ese rol se otorga **automáticamente a todo rol creado por consola, CLI o API**,
   incluido el rol por defecto del proyecto (`neondb_owner`).
3. Los roles creados con `CREATE ROLE` **en SQL** no reciben esa membresía. La propia
   documentación de Neon dice que SQL es justamente el camino para roles con
   privilegios restringidos, y su guía de RLS recomienda explícitamente evitar
   `neondb_owner` y usar un rol propio sin `BYPASSRLS`.

Por qué importa tanto: el modo de falla es del peor tipo posible. Provisionar
`app_tenant` desde el dashboard haría que **todas** las políticas de MascotApp fueran
un no-op en producción, mientras los tests A/B de aislamiento **siguen pasando en
local** — el Postgres del compose no tiene `neon_superuser`, así que la suite nunca
ve el problema. Silencioso, específico del entorno, e invisible para los tests que
existen precisamente para atrapar esto.

Reglas que se derivan:

- Los roles `app_tenant` y `app_public` se crean **solo** desde la migración goose,
  en SQL. Nunca desde el dashboard de Neon.
- La suite de tests lleva una aserción de guardia: el rol con el que conecta debe
  tener `rolsuper = false` **y** `rolbypassrls = false`, leídos de `pg_roles`. Es la
  única verificación que atrapa un rol provisionado a mano antes de que llegue a
  producción.
- Los runbooks de despliegue deben decirlo explícitamente.

Esto amplía la trampa que ya documentaba [[ADR-0002]] ("RLS no aplica a
superusuarios") con un giro que nadie adivina: en Neon la trampa la dispara **cómo
creaste el rol**, no cuál rol elegiste.
