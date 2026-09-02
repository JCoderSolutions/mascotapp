---
type: bug
score: 4
topic_key: mascotapp/convention/plpgsql-triggers
approved: 2026-08-31 por el usuario
observation_id: obs-19d7bb85bca38508
task: T-01-023
rationale: "El peor modo de falla posible: sin error, cero filas, y el test verde. Todo trigger BEFORE de fila que este esquema escriba de aca en adelante puede pisarlo."
---

# `RETURN NEW` en un `BEFORE DELETE` cancela la sentencia en silencio

En un trigger `BEFORE ... FOR EACH ROW`, devolver `NULL` **cancela la operacion para esa
fila**: sin error, sin aviso, y `RowsAffected() = 0`. Y en un `BEFORE DELETE`, **`NEW` es
`NULL`**.

Asi que este trigger, que se lee perfecto, hace que ningun borrado funcione nunca:

```plpgsql
IF OLD.published_at IS NOT NULL THEN
    RAISE EXCEPTION '...';
END IF;

RETURN NEW;   -- en un DELETE esto es NULL: cancela el borrado, callado
```

Correcto:

```plpgsql
IF TG_OP = 'DELETE' THEN
    RETURN OLD;
END IF;

RETURN NEW;
```

## El corolario para los tests, que es la mitad importante

Un test que solo mira `err != nil` **no ve esto**: no hay error. Habria reportado un draft como
"editable y borrable" mientras el trigger descartaba cada cambio.

**Toda asercion de que una escritura FUNCIONO tiene que mirar `RowsAffected()`, no la ausencia
de error.** Es la misma familia que el `25P02` de T-01-020: la sonda parecia afirmar algo y no
afirmaba nada.

Se encontro releyendo la migracion antes de correrla, y despues se confirmo con un mutante que
restaura el `RETURN NEW` pelado.

Relacionado: [[2026-08-31-el-primer-rechazo-aborta-la-transaccion]],
[[2026-08-31-truncate-es-la-escritura-que-ninguna-politica-ve]].
