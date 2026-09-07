---
fecha: 2026-09-04
tarea: T-02-018
tipo: bug
engram: obs-9e00e3a6413dc7d2
---

# Un rollback contesta antes que el orden de tu guarda, y eso vuelve equivalente al mutante

Quinta instancia de *la capa que contesta primero vacía el test de abajo*.

## El caso

`RegenerateRecoveryCodes` borra los diez códigos de recuperación del usuario y
escribe diez nuevos. Un set vacío borraría los diez y no escribiría ninguno: el
usuario queda sin forma de volver a entrar, y la operación **reporta éxito**.
Ninguna restricción de la base lo atrapa — borrar diez filas e insertar cero es
una transacción perfectamente legal.

Se agregó una guarda que refusa el set vacío, **antes** del delete, con un test
que asertaba dos cosas: que devuelve error, y que el set viejo **sigue ahí**
después del rechazo.

El comentario del test decía, textual:

> *"la aserción de supervivencia es lo que fija el ORDEN. Una guarda puesta
> después del delete devolvería error igual y dejaría al usuario sin nada."*

**El mutante que mueve la guarda debajo del delete SOBREVIVIÓ.** El test siguió
verde.

## Por qué

`db.WithAuthUser` abre la transacción, corre el callback y **hace rollback ante
cualquier error que el callback devuelva**. Con la guarda abajo, el delete corre,
la guarda falla, la transacción se revierte y las filas vuelven. El resultado
observable es idéntico.

La aserción de supervivencia la contesta **el rollback**, no la posición de la
guarda. El comentario le atribuía al test una propiedad que el test no puede
probar.

## Que sea equivalente se PRUEBA, no se acepta

Se corrió un segundo mutante que **saca la guarda entera**: ahí el test muere en
la línea del `err == nil`, porque sin guarda la transacción commitea y el usuario
se queda con cero códigos.

| Afirmación | Mutante | Resultado |
|---|---|---|
| la guarda es load-bearing | sacarla entera | **muere** |
| su POSICIÓN es load-bearing | moverla debajo del delete | **sobrevive** — equivalente |

## La clase, no el caso

**Dentro de una misma transacción, el orden de una guarda respecto de una
escritura destructiva es inobservable para el llamador**, siempre que el error
provoque rollback. Toda esa familia de mutantes es equivalente, y todo test que
diga fijar ese orden está afirmando algo que no puede.

Lo que sí queda pinneado, y por eso la aserción se conserva en vez de borrarse:
que un rechazo es **atómico**. Atraparía una versión futura que corriera el
delete fuera de la transacción del llamador, o que se comiera el error.

## La corrección que importa

No fue de código — la guarda se queda arriba, porque emitir una sentencia que ya
sabés que vas a deshacer es trabajo al pedo. **Fue de los dos comentarios**, que
ahora dicen qué capa contesta cada aserción y admiten que la versión anterior
afirmaba de más.

Un comentario que le atribuye a un test una garantía que no da es peor que no
tener comentario: el próximo que lo lea no va a volver a correr la mutación, va a
creerle.

## La otra cara

Ver [[un-rechazo-que-escribe-no-puede-viajar-como-error]]: ahí el mismo rollback
**destruye** una contención de seguridad en vez de salvarla. Misma capa, efecto
opuesto. La pregunta útil es siempre la misma: *¿qué sobrevive al final de esta
transacción, y quién lo decidió?*
