---
type: constraint
score: 5
topic_key: mascotapp/ops/bigserial-needs-a-sequence-grant
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-64400c1e36f5e3ab
task: T-01-032
rationale: "Es el unico grant del esquema que no es sobre una tabla, y su falla es invisible en desarrollo y aparece en la primera escritura de produccion. Ademas la mitad OPUESTA del grant tambien rompe algo, y esa es la parte que nadie mira."
---

# `bigserial` necesita un grant que no es sobre una tabla, y `ALL` tampoco sirve

`bigserial` **no es un tipo**. Es `bigint` + una **SECUENCIA** + un `DEFAULT nextval(...)`. Un
grant de tabla no dice **nada** sobre esa secuencia.

## Sin el grant

`GRANT INSERT ON audit_log TO app_tenant` produce una tabla en la que el tenant puede insertar
**solo cuando el mismo provee el id**. Que es exactamente lo que hace **todo test que escribe ids
explicitos**, y lo que **no hace ningun caller real**.

**El modo de falla es lo que lo vuelve peligroso:** invisible en desarrollo, aparece en la primera
escritura de produccion. Un test que quiera cazarlo tiene que insertar **sin** id — y eso hay que
escribirlo a proposito, porque el reflejo es pasar un id conocido para despues afirmarlo.

```sql
GRANT USAGE ON SEQUENCE audit_log_id_seq TO app_tenant;
```

## Con `ALL` en vez de `USAGE`

Y aca esta la mitad que nadie mira. `ALL` sobre una secuencia es `USAGE` + `SELECT` + **`UPDATE`**,
y `UPDATE` sobre una secuencia es **`setval`**.

Un tenant con `setval` puede poner el contador alto, appendear, ponerlo bajo y appendear de nuevo:
y entonces una fila **posterior** lleva un id **menor** que una anterior. El log se lee en orden de
id **justamente porque ese orden se supone que es el orden en que pasaron las cosas**. Un tenant
que puede mover el contador puede **reordenar su propia historia**.

Y ademas puede rebobinar sobre ids ya tomados, con lo que cada append siguiente choca contra la
primary key: un rastro de auditoria que **deja de registrar**.

**Un inserter necesita `nextval`. Nada en este producto necesita mover el contador a mano.**

## La anti-vacuidad del test, que no es opcional

El test que prueba que `setval` esta prohibido tiene que probar **primero** que `nextval` funciona.
Sin eso, un `USAGE` faltante pasa el rechazo mientras deja la tabla inescribible — o sea, el bug de
"sin el grant" pasando disfrazado del bug de "con `ALL`". Los dos mutantes se ven iguales desde
afuera y arreglan cosas opuestas.
