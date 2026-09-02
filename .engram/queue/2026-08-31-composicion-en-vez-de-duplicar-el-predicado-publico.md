---
type: architecture
score: 4
topic_key: mascotapp/arch/public-catalog
approved: 2026-08-31 por el usuario
observation_id: obs-db33d8e90bb53b65
task: T-01-020
rationale: "Duplicar 'que es publico' en cada tabla garantiza que en algun refactor una copia se quede vieja, y esa copia es una fuga."
---

# "Publico" se define en un solo lugar; las demas tablas componen

La politica publica de `media` aterrizo en su **tercera** planificacion (00003, despues
00005, y por fin 00006 — cuando existe `pet_media`, la tabla que hace expresable el join).
Una politica no puede referenciar una tabla que todavia no existe.

La tentacion era repetir el predicado del catalogo — `status = 'available' AND published_at
IS NOT NULL AND deleted_at IS NULL` — dentro de la politica de `media`. Se hizo lo otro:

```sql
-- pet_media: "existe el pet"
USING (EXISTS (SELECT 1 FROM pets p WHERE p.id = pet_media.pet_id))

-- media: "existe el adjunto"
USING (deleted_at IS NULL
       AND EXISTS (SELECT 1 FROM pet_media pm WHERE pm.media_id = media.id))
```

Ninguna de las dos sabe que significa "publico". Cada subquery ya viene filtrada por la RLS
de la tabla que lee bajo `app_public`, asi que la definicion vive **solo** en
`public_catalog` sobre `pets`, y la foto **sigue a su pet sola**.

Probado caminando un pet por draft → publicado → retractado y mirando aparecer y desaparecer
la foto, **sin tocar nunca la fila de media**.

## El costo que si tiene

Una politica que lee otra tabla **es una dependencia**: `DROP TABLE pet_media` falla con
`2BP01` mientras exista la politica de `media`. El `-- +goose Down` tiene que hacer
`DROP POLICY ... ON media` primero, nunca `CASCADE`.

Relacionado: [[2026-08-30-una-politica-que-lee-otra-tabla-es-una-dependencia]],
[[2026-08-31-un-exists-de-politica-tambien-exige-el-grant]].
