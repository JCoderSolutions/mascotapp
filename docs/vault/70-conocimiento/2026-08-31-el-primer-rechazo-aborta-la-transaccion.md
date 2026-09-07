---
type: bug
score: 3
topic_key: mascotapp/convention/testing-aborted-transaction
approved: 2026-08-31 por el usuario
observation_id: obs-8ab9e31b8abb632d
task: T-01-020
rationale: "El test pasaba en verde con una de sus dos propiedades sin afirmar, y la causa era el exito de la primera asercion."
---

# En una tabla append-only, el primer rechazo aborta la transaccion y silencia la segunda sonda

El caso A/B de `pet_status_history` sondea `UPDATE` y despues `DELETE`, ambos esperando ser
rechazados. El `UPDATE` fallaba como corresponde... y a partir de ahi **toda** sentencia de
esa transaccion devuelve `25P02, current transaction is aborted`.

Asi que la sonda de `DELETE` **jamas pudo evaluar su propiedad**. Y como esperaba un error,
recibia uno: verde. El test pasaba por el motivo equivocado.

Se arregla dandole a cada sonda su propio savepoint (`tx.Begin` anidado en pgx):

```go
sub, err := tx.Begin(ctx)
defer func() { _ = sub.Rollback(ctx) }()
return tt.expectRefusal(ctx, sub, verb, statement, args)
```

## La regla

**Un test que espera errores no puede encadenar aserciones en una sola transaccion.** El
exito de la primera destruye la capacidad de medir de las siguientes, y el modo de falla es
el peor posible: silencioso y en verde. Verificar el **SQLSTATE exacto** — no un "fallo"
generico — es lo que lo saca a la luz.

Lo encontro `make test-api-container` en su primera corrida. La ronda de mutacion a nivel
SQL no podia verlo: el defecto estaba en el instrumento de Go, no en el esquema.

Relacionado: [[2026-08-29-mutation-testing-vacuous-coverage]].
