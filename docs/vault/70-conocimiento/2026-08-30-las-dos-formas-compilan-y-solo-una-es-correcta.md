---
type: architecture
score: 5
topic_key: mascotapp/security/a-single-column-fk-does-not-isolate
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-a5ce66db164f9410
task: T-01-017
rationale: "Una FK de una columna y una compuesta se ven igual en el diff, migran igual, y una abre una fuga entre tenants."
---

# Una FK de una columna y una compuesta compilan las dos; solo una aisla

`shelters.logo_media_id` apunta a `media`. La forma obvia:

```sql
logo_media_id uuid REFERENCES media (id)
```

Compila, migra, y **deja que el refugio B ponga como logo una fila de `media` del
refugio A** — una fila que B no puede ver, no puede leer, no puede listar, y
estaría publicando en su propia página.

La política no lo impide, porque **la política nunca se consulta**: los chequeos
de integridad referencial corren como el dueño de la constraint, no como el rol
que consulta. **Siempre saltean row security.**

La forma correcta:

```sql
FOREIGN KEY (logo_media_id, id) REFERENCES media (id, shelter_id)
```

El par es `(logo_media_id, id)` porque para `shelters` la columna de tenant **es**
`id`. Y `MATCH SIMPLE` — el default — significa que si cualquier columna de la
clave es NULL, no se chequea nada: un refugio sin logo pasa sin restricción, que
es justo lo que se quiere.

Eso convierte la fuga de *prohibida* en **irrepresentable**.

## Por qué esto necesita un test y no alcanza el review

**Las dos formas compilan. Las dos migran. Las dos pasan el meta-test.** El diff
entre ellas son ocho caracteres. Ningún linter, ningún tipo, ningún juez leyendo
SQL lo garantiza — solo un test que intente el reclamo cruzado y exija `23503`.

Y ese test necesita su propio paso anti-vacuidad: **A tiene que poder apuntar a su
propia fila primero**, o una constraint que rechace todo pasaría igual.

## Corolario sobre el `UNIQUE (id, shelter_id)`

Esa clave se ve redundante al lado de la primary key, y no lo es: es **la clave
referenciada** de toda FK compuesta de tenant. Alguien que la "limpie" hace
imposible declararlas. Por eso se afirma directo, no implícitamente.

Ver [[2026-08-30-una-membership-invitada-es-un-grant-de-lectura]] — misma familia: el
mecanismo que saltea la política.
