---
type: convention
score: 4
topic_key: mascotapp/convention/mutation-testing-anti-vacuity-invalidation
task: T-02-032
status: guardado
aprobado: 2026-09-26 por el usuario
observation_id: obs-c459886d42e6fd99
rationale: "Ya se repitió dos veces en esta fase (T-02-029, ahora T-02-032) que un test de invalidación sin control positivo pasa aunque el handler nunca haga nada. Vale la pena como checklist reusable para cualquier test futuro con la forma 'X deja de servir después de Y'."
---

# Un test de invalidación sin control positivo no prueba nada

(T-02-032, MascotApp Fase 02. Pariente de
`[[2026-09-14-un-muerto-falso-es-peor-que-un-sobreviviente-falso]]` — los dos
son formas en que un reporte de mutación verde miente sin que ningún mutante
esté mal clasificado, y de
`[[2026-09-08-un-test-que-compara-dos-salidas-entre-si-no-fija-ninguna-de-las-dos]]`
— los dos son propiedades que se escriben como si bastara con observar un
solo extremo del antes/después.)

## La regla

Un test con la forma "el código A deja de funcionar después del evento B"
—revocar una sesión, invalidar un código de recuperación, expirar un
token— puede pasar aunque el sistema bajo prueba **nunca haya hecho
funcionar A en primer lugar**. Si A nunca fue un valor válido, "A ya no
funciona después de B" es trivialmente cierto sin que B haya hecho nada.

Todo test de invalidación necesita un **control positivo** explícito: una
aserción, antes del evento invalidante, de que el estado que se va a
invalidar era genuinamente válido. Sin ese control, el test mide "algo
trivialmente falso sigue siendo falso", no "el sistema invalidó algo real".

## El caso

`TOTPEnrolHandler` genera diez códigos de recuperación de un solo uso por
cada enrolamiento. Un segundo enrolamiento del mismo usuario tiene que
invalidar el primer set completo — es la propiedad de seguridad que evita
que un atacante con un set viejo filtrado siga teniendo acceso después de
que el dueño legítimo rote sus credenciales.

`TestTOTPEnrol_ASecondEnrolInvalidatesTheFirstRecoveryCodes` afirmaba
exactamente eso: enrolar, enrolar de nuevo, y comprobar que un código del
primer set ya no es redimible. Parece una prueba completa de la propiedad.

**Un mutante que salteaba `RegenerateRecoveryCodes` por completo —nunca
persistir ningún código nuevo ni viejo— sobrevivió sin cambios.** La razón:
un código que nunca se persistió en ningún lado ya era "no redimible" desde
el instante en que se generó. El test nunca verificó que el primer set
hubiera funcionado alguna vez; solo verificó que, al final, no funcionaba.
Eso es cierto tanto si el sistema invalidó el set como si el sistema nunca
lo hizo válido.

## Por qué importa

El test tenía la forma correcta y la intención correcta, y aun así **nunca
tuvo poder para fallar** frente al defecto que se supone que cubre. No es un
error de clasificación del harness de mutación (esa es la familia de
`[[2026-09-14-un-muerto-falso-es-peor-que-un-sobreviviente-falso]]`): acá el
mutante muere de verdad, el build compila, todo se lee bien — el agujero
está en que el test nunca fijó su propia precondición.

El arreglo es barato una vez que se lo nombra: redimir un código **distinto**
del mismo set, inmediatamente después del primer enrolamiento y antes de
que corra el segundo. Eso prueba que el set era funcional, y entonces "ya no
funciona después del segundo enrolamiento" mide algo real.

## Dónde más aplica

Cualquier test cuya afirmación central sea "X deja de servir después de Y":

- revocación de sesión o de token de refresh — probar que el token servía
  *antes* de revocarlo, no solo que no sirve después
- expiración de un magic link o de un código de un solo uso
- rotación de secretos o de API keys — el secreto viejo tiene que haber sido
  válido para que su rechazo posterior signifique algo
- cualquier "borrado lógico" o `revoked_at` — un registro que nunca se pudo
  usar no prueba que el campo de revocación funcione

La pregunta que lo detecta al escribir el test: **¿en algún punto del test
se demuestra que el estado que se va a invalidar realmente servía?** Si la
única aserción es sobre el estado final, el test no distingue "se invalidó"
de "nunca fue válido".
