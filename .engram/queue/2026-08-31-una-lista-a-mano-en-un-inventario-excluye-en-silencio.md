---
type: convention
score: 4
topic_key: mascotapp/convention/inventory-from-declaration
approved: 2026-08-31 por el usuario
observation_id: obs-a2740164931022d6
task: T-01-023
rationale: "Segunda vez en dos tareas que una lista escrita a mano deja tablas sin verificar. La falla no es 'falta una asercion': es un test que reporta un esquema limpio que nunca miro."
---

# Un inventario filtrado por una lista a mano excluye tablas nuevas en silencio

El inventario de politicas filtraba con un `c.relname IN ('shelters', 'users', ...)`
**hardcodeado**. Cuando T-01-023 agrego `form_templates` y `form_template_versions`, sus
politicas **nunca se leyeron**: la query no las pedia.

El modo de falla es el peor de los dos posibles. No es "falta una asercion" — es que **el test
reporta un esquema limpio que no miro**. Y el sintoma que aparece es un desajuste de largo
entre `got` y `want`, que manda a buscar una politica faltante en la migracion en vez de una
tabla faltante en la query.

## El arreglo

Atar el filtro a la **declaracion**, que ya es la fuente de verdad de que tablas existen:

```go
AND c.relname = ANY ($1)
```
...con `rlstest.Schema.ModelTables()`. Ahora una tabla no puede quedar fuera del inventario sin
quedar tambien fuera de la declaracion, y quedar fuera de la declaracion ya falla por su cuenta
(`ClassifyAll`).

## La regla

**Segunda vez en dos tareas.** T-01-022 encontro lo mismo en la matriz de grants, donde las
celdas elegidas a mano cubrian las tablas que alguien se acordo el dia que las escribio; se
resolvio enumerando el catalogo entero.

> Un chequeo de conformidad se maneja por enumeracion — del catalogo o de la declaracion —
> nunca por una lista que alguien tiene que acordarse de extender. Si la lista es la fuente de
> verdad, entonces olvidarse es indistinguible de estar en orden.

Relacionado: [[2026-08-31-extender-el-runner-ab-no-eximir-la-tabla]],
[[2026-08-30-un-suite-verde-sobre-cero-tablas]].
