---
type: constraint
score: 4
topic_key: mascotapp/security/rls-exists-grant
approved: 2026-08-31 por el usuario
observation_id: obs-e2fdbf0b785f08df
task: T-01-020
rationale: "Sin el grant no se reciben cero filas, se recibe un error de permisos. La diferencia decide si una pagina publica se ve vacia o se cae."
---

# El subquery `EXISTS` de una politica exige que el rol tenga `SELECT` sobre la tabla que lee

Ya estaba registrado que ese subquery **hereda la RLS** de la tabla referenciada. Faltaba la
otra mitad, y la fase la venia **asumiendo**: tambien exige el **privilegio de tabla**.

Lo probo el mutante **M13** de T-01-020. Quitando
`GRANT SELECT ON pet_media TO app_public`, la politica de `media`:

```sql
CREATE POLICY public_catalog ON media FOR SELECT TO app_public
    USING (deleted_at IS NULL
           AND EXISTS (SELECT 1 FROM pet_media pm WHERE pm.media_id = media.id));
```

no devuelve **cero filas de media**. Devuelve un **error de permisos** sobre `pet_media`.

## Por que importa

- Un grant faltante en una tabla puente no degrada en "no se ve nada": **rompe la consulta**.
  El catalogo publico no queda vacio, tira 500.
- La simetria a tener en la cabeza: **escribir en una tabla puente es leer a traves de
  ella**, y ahora tambien **no poder leerla es no poder leer a traves de ella**.
- Practico: cada politica que referencia otra tabla suma **dos** filas al inventario — la
  politica y el grant. Verificar solo la politica deja la mitad sin afirmar.

## Y el mutante casi se pierde

M13 aborto la corrida de sondas y se anoto como "no evaluado" en vez del kill que era —
**un error del instrumento contado contra la red**, exactamente el mismo de T-01-012. Se
arreglo haciendo que la sonda devuelva `-1` ante excepcion en vez de morir. Un sobreviviente
inesperado se investiga, nunca se acepta: cuatro veces sobre cuatro el defecto estuvo en el
instrumento, no en la red.

Relacionado: [[2026-08-30-una-politica-exists-hereda-el-rls-de-la-tabla-que-lee]],
[[2026-08-29-mutation-testing-vacuous-coverage]].
