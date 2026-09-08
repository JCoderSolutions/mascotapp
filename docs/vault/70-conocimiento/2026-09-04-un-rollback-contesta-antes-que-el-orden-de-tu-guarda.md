---
type: bug
score: 4
topic_key: mascotapp/convention/the-layer-that-answers-first
task: T-02-018
status: guardado
approved: 2026-09-04 por el usuario (T-02-018, cierre de PR-02-10)
observation_id: obs-9e00e3a6413dc7d2
extends: obs-9e00e3a6413dc7d2
nota_de_guardado: "Mismo `observation_id` que las cuatro instancias anteriores: el `topic_key` compartido suma la nota al hilo existente. El id nombra el TEMA, no el candidato — la trazabilidad de esta instancia vive en este archivo. Ver [[2026-09-04-una-defensa-en-profundidad-nueva-desarma-el-test-de-abajo]]."
rationale: "Quinta instancia del patrón, y la primera donde la capa intrusa no es una policy, ni un trigger, ni un chequeo mío: es el ROLLBACK de la transacción. Vale guardarla aparte porque el rollback vuelve EQUIVALENTE toda una familia de mutantes —cualquier reordenamiento de una guarda respecto de una escritura destructiva, dentro de la misma transacción— y eso es una clase, no un caso. Y porque el error concreto no estuvo en el test sino en el COMENTARIO del test: afirmaba que la aserción fijaba el orden, y la mutación demostró que no. Un comentario que le atribuye a un test una propiedad que no prueba es peor que no tener comentario, porque el próximo que lo lea va a creerle."
---

# Un rollback contesta antes que el orden de tu guarda, y eso vuelve equivalente al mutante

Quinta instancia de `obs-9e00e3a6413dc7d2` (*la capa que contesta primero vacía el test de
abajo*). (T-02-018, MascotApp Fase 02.)

## El caso

`RegenerateRecoveryCodes` borra los diez códigos de recuperación del usuario y escribe diez
nuevos. Un set vacío borraría los diez y no escribiría ninguno: el usuario queda sin forma de
volver a entrar a su cuenta, y la operación **reporta éxito**. Ninguna restricción de la base
lo atrapa — borrar diez filas e insertar cero es una transacción perfectamente legal.

Se agregó una guarda que refusa el set vacío, **antes** del delete, con un test propio que
asertaba dos cosas:

1. que devuelve error, y
2. que el set viejo **sigue ahí** después del rechazo.

Y el comentario del test decía, textual: *"la aserción de supervivencia es lo que fija el
ORDEN. Una guarda puesta después del delete devolvería error igual y dejaría al usuario sin
nada."*

**El mutante que mueve la guarda debajo del delete SOBREVIVIÓ.** El test siguió verde.

## Por qué

`db.WithAuthUser` abre la transacción, corre el callback y **hace rollback ante cualquier
error que el callback devuelva**. Con la guarda abajo, el delete corre, la guarda falla,
la transacción se revierte y las filas vuelven. El resultado observable es idéntico.

O sea: la aserción de supervivencia la contesta **el rollback**, no la posición de la guarda.
El comentario le atribuía al test una propiedad que el test no puede probar.

## Que sea equivalente se PRUEBA, no se acepta

El mutante 2 es equivalente, pero eso no se asumió. Se corrió un mutante 3 que **saca la
guarda entera**: ahí el test muere en la línea del `err == nil`, porque sin guarda la
transacción commitea y el usuario se queda con cero códigos.

Los dos juntos separan las dos afirmaciones:

| Afirmación | Mutante | Resultado |
|---|---|---|
| la guarda es load-bearing | 3 — sacarla | **muere** |
| su POSICIÓN es load-bearing | 2 — moverla debajo del delete | **sobrevive** — equivalente |

## La clase, no el caso

Esto no es un accidente de esta función. **Dentro de una misma transacción, el orden de una
guarda respecto de una escritura destructiva es inobservable para el llamador**, siempre que
el error provoque rollback. Toda esa familia de mutantes es equivalente, y todo test que diga
fijar ese orden está afirmando algo que no puede.

Lo que sí queda pinneado, y por eso la aserción se conserva en vez de borrarse: que un
rechazo es **atómico**. Atraparía una versión futura que corriera el delete fuera de la
transacción del llamador, o que se comiera el error.

## La corrección que importa

No fue de código — la guarda se queda arriba, porque emitir una sentencia que ya sabés que vas
a deshacer es trabajo al pedo. **Fue de los dos comentarios**, en el test y en la
implementación, que ahora dicen qué capa contesta cada aserción y admiten explícitamente que
la versión anterior afirmaba de más.

Un comentario que le atribuye a un test una garantía que no da es peor que no tener
comentario: el próximo que lo lea no va a volver a correr la mutación, va a creerle.

## La otra cara

Ver [[2026-09-04-un-rechazo-que-escribe-no-puede-viajar-como-error]]: ahí el mismo rollback
**destruye** una contención de seguridad en vez de salvarla. Un rechazo que revoca una familia
de tokens y después devuelve `error` pierde la revocación, y el ladrón conserva su sesión.

Misma capa, efecto opuesto. La pregunta útil es siempre la misma: *¿qué sobrevive al final de
esta transacción, y quién lo decidió?*
