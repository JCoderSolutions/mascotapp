---
type: convention
score: 4
topic_key: mascotapp/convention/mutation-testing-anti-vacuity-invalidation
task: T-02-032
rationale: "Ya se repitió dos veces en esta fase (T-02-029, ahora T-02-032) que un test de invalidación sin control positivo pasa aunque el handler nunca haga nada. Vale la pena como checklist reusable para cualquier test futuro con la forma 'X deja de servir después de Y'."
---

Un test con la forma "el código A deja de funcionar después del evento B"
(revocar una sesión, invalidar un código de recuperación, expirar un token)
puede pasar aunque el sistema bajo prueba **nunca haya hecho funcionar A en
primer lugar**. Si A nunca fue un valor válido, "A ya no funciona después de
B" es trivialmente cierto sin que B haya hecho nada.

Encontrado en `T-02-032`: `TestTOTPEnrol_ASecondEnrolInvalidatesTheFirstRecoveryCodes`
afirmaba que los códigos de recuperación del primer enrolamiento dejan de ser
redimibles después de un segundo enrolamiento. Un mutante que salteaba
`RegenerateRecoveryCodes` por completo (nunca persistir ningún código) seguía
pasando el test sin cambios, porque un código que nunca se persistió ya era
"no redimible" desde el principio.

**Regla**: todo test de invalidación necesita un control positivo explícito
que pruebe que el estado ANTES del evento invalidante era genuinamente válido
— en este caso, redimir un código distinto del mismo set inmediatamente
después de crearlo, antes de que corra el evento que se supone lo invalida.
Sin ese control, el test mide "algo trivialmente falso sigue siendo falso",
no "el sistema invalidó algo real".

Esto extiende el hallazgo de `T-02-030` (`obs-296d31be8cc336f7`, "un muerto
falso es peor que un sobreviviente falso") a una categoría distinta de falso
positivo: no un clasificador que lee mal el resultado, sino un test que nunca
tuvo poder para fallar porque nunca probó su propia precondición.
