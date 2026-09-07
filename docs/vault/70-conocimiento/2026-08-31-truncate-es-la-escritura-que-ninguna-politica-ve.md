---
type: constraint
score: 4
topic_key: mascotapp/security/rls-truncate
approved: 2026-08-31 por el usuario
observation_id: obs-7547cb4172cbc931
task: T-01-020
rationale: "Una politica RLS parece cubrir toda escritura. Hay exactamente una que jamas ve, y es justo la que borra todo."
---

# `TRUNCATE` es la unica escritura que ninguna politica de fila puede ver

Row Level Security se evalua **por fila**: `USING` decide que filas son visibles,
`WITH CHECK` decide que filas pueden quedar escritas. `TRUNCATE` **no visita ninguna fila**
— descarta el almacenamiento entero — asi que ni `USING` ni `WITH CHECK` se consultan jamas.

Consecuencia: una tabla con RLS forzada, sin politica de `DELETE`, y con todo revocado menos
`SELECT` e `INSERT`, **igual se puede vaciar** si alguien conserva el privilegio `TRUNCATE`.

Lo unico que lo alcanza es un trigger **de sentencia**:

```sql
CREATE TRIGGER pet_status_history_no_truncate
    BEFORE TRUNCATE ON pet_status_history
    FOR EACH STATEMENT EXECUTE FUNCTION pet_status_history_is_append_only();
```

`FOR EACH ROW` no sirve aca: no hay filas que recorrer. Son dos triggers distintos sobre la
misma funcion, no uno.

## Y el trigger no es redundante con el `REVOKE`

Ni el `REVOKE` ni la politica ausente sobreviven a un rol con `BYPASSRLS`, y **en Neon eso
no es hipotetico**: `neon_superuser` lo tiene. **La RLS se saltea con `BYPASSRLS`; los
triggers no.** Por eso el test corre **como OWNER** — un superusuario con todos los
privilegios que ademas se saltea toda politica — de modo que si pasa, solo puede ser obra
del trigger. Su SQLSTATE es el `P0001` de plpgsql, distinto a proposito del `42501` del
grant, para que las dos capas sigan siendo distinguibles cuando una se caiga.

Relacionado: [[2026-08-29-neon-bypassrls-defeats-policies]],
[[2026-08-31-append-only-con-fk-que-cascadea-es-teatro]].
