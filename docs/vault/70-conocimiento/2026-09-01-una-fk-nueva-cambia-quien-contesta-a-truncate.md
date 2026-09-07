---
type: constraint
score: 4
topic_key: mascotapp/security/truncate-cascade-bypasses-the-fk
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-ee57985f8321d331
task: T-01-025
rationale: "El test del trigger BEFORE TRUNCATE seguia verde por el motivo equivocado: la FK nueva rechazaba antes. CASCADE es el bypass de esa capa, y es lo unico que llega al trigger."
---

# Una FK nueva cambia quien contesta a TRUNCATE, y CASCADE es el bypass

`TRUNCATE` es la unica escritura que ninguna politica row-level puede ver, y por eso
`form_template_versions` lleva un trigger `BEFORE TRUNCATE ... FOR EACH STATEMENT`.

El dia que `form_submissions` apunto a esa tabla, **el test del trigger empezo a pasar por el
motivo equivocado**. Un `TRUNCATE` plano ahora se rechaza **antes** con `0A000`:

```
cannot truncate a table referenced in a foreign key constraint
```

Nunca llega al trigger. O sea que el test podia seguir verde con el trigger **borrado**.

**`TRUNCATE ... CASCADE` es el bypass de esa capa.** Camina la foreign key, vacia tambien la
tabla hija, y llega al trigger — que es el unico que puede refusarlo.

Asi que el caso prueba **las dos**, en orden: el `TRUNCATE` plano rechazado con `0A000`, y
despues el `CASCADE`, que **debe** morir con el `P0001` del trigger. Sin la segunda mitad, la
proteccion que se esta afirmando es la de la foreign key, no la del trigger — y son dos cosas
distintas que se pueden perder por separado.

**La forma general, que este proyecto ya vio dos veces:** cuando una capa nueva contesta antes
que la que estabas probando, el test sigue verde y deja de probar lo que decia probar. Igual que
la cascada de T-01-020 y que `published_at` en T-01-023 — el agujero nunca esta en lo que estabas
mirando.

Ver [[truncate-es-la-escritura-que-ninguna-politica-ve]].
