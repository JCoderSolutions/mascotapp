# ADR-0002 — Multi-tenancy con Row Level Security

- **Fecha:** 2026-08-28
- **Estado:** aceptado
- **Fase:** 00

## Contexto

Varios refugios comparten una sola aplicación y una sola base. Un refugio no puede ver
datos de otro — ni siquiera por un error de código. El free tier no permite una base
por inquilino.

## Decisión

**Base compartida, esquema compartido, discriminador `shelter_id`, y Row Level Security
de PostgreSQL.** Defensa en tres capas:

1. **Middleware** — resuelve el tenant desde el claim `shelter_id` del JWT.
   **Nunca** desde la URL ni desde un header controlable por el cliente.
2. **Transacción** — abre fijando la GUC del tenant, con la app conectada como rol
   **no-superusuario** (`app_tenant`).
3. **Base** — política RLS por tabla, con `FORCE ROW LEVEL SECURITY`.

> **Corrección (2026-08-29, exploración de Fase 01).** Este ADR decía
> `SET LOCAL app.shelter_id = '<uuid>'`. `SET LOCAL` es una sentencia de utilidad y
> **no acepta parámetros de vínculo**, así que esa forma obliga a interpolar el UUID
> del tenant dentro del SQL como string. La forma correcta es la función, que sí los
> acepta:
>
> ```sql
> SELECT set_config('app.shelter_id', $1, true)
> ```
>
> El tercer argumento `true` significa "local a la transacción actual", que es
> exactamente la semántica de `SET LOCAL`. Hoy el UUID viene de un claim validado, así
> que no hay inyección; la corrección es para que la forma insegura no se copie hacia
> adelante el día que la fuente sea menos confiable.

El catálogo público usa un rol y una conexión separados, de solo lectura (`app_public`),
con su propia política restringida a animales publicados.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| Esquema por refugio | Dolor de migraciones; no escala a cientos de refugios |
| Base por refugio | Imposible en free tier |
| Solo `WHERE shelter_id = ?` | Un `WHERE` olvidado filtra datos de otro refugio. Es cuestión de tiempo |

## Consecuencias

- Un `WHERE` olvidado devuelve **cero filas** en lugar de filtrar datos ajenos. La base
  es la última línea de defensa y no depende de la disciplina de quien escribe el SQL.
- **Trampa conocida:** RLS **no aplica a superusuarios**. Si la app se conecta con el rol
  dueño, toda la protección se anula en silencio.
- **Trampa específica de Neon (verificada 2026-08-29).** El rol `neon_superuser` tiene el
  atributo **`BYPASSRLS`**, y se otorga automáticamente a **todo rol creado desde la
  consola, el CLI o la API** de Neon, incluido el rol por defecto del proyecto. Los roles
  creados con `CREATE ROLE` **en SQL** no lo reciben.
  Consecuencia: provisionar `app_tenant` desde el dashboard haría que **todas** las
  políticas de este ADR fueran un no-op en producción, mientras los tests A/B siguen
  **pasando en local** — el Postgres del compose no tiene `neon_superuser`.
  Por eso los roles se crean **solo desde la migración**, y la suite lleva una aserción
  de guardia: el rol con el que conecta debe tener `rolsuper = false` y
  `rolbypassrls = false` en `pg_roles`.
- Cada consulta debe correr dentro de una transacción que fije la GUC. Se encapsula en un
  wrapper para que no sea opcional.

## Requisito de test — no negociable

Por cada tabla con `shelter_id`, un test de integración que pruebe que **el tenant A no
puede leer, escribir ni borrar filas del tenant B**. Sin ese test, la tabla no se
considera terminada.

## Condiciones de reapertura

Que un refugio institucional exija aislamiento físico de datos por contrato.
