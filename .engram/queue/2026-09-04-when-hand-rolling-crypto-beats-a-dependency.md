---
type: architecture
score: 5
topic_key: mascotapp/security/three-conditions-to-override-never-hand-roll-crypto
task: T-02-013
status: guardado
observation_id: obs-bf6fea41522ee496
rationale: "\"Nunca escribas cripto propia\" es una de las reglas mejor establecidas que existe, y es correcta casi siempre — por eso pasarle por encima sin un criterio escrito es cómo un proyecto termina con un AES casero. Pero aplicada sin condiciones también hace que se agregue una dependencia a internal/auth para no escribir un algoritmo congelado hace quince años con vectores de prueba publicados. Lo que vale guardar no es la decisión de TOTP: es el TEST de tres condiciones que la habilitó, y sobre todo que las tres tienen que darse juntas. Dentro de tres meses alguien va a citar este caso para justificar el próximo, y sin las condiciones va a citar la conclusión en vez del criterio."
---

# Las tres condiciones que hay que cumplir a la vez para escribir cripto en vez de depender

P2-D8 eligió `github.com/pquerna/otp` para TOTP y rechazó el HMAC propio con el argumento
estándar: *"crypto you write is crypto you maintain forever"*. Se revirtió en `T-02-013` y se
construyó sobre `crypto/hmac` + `crypto/sha1` + `net/url`, sin tocar `go.mod`.

La regla general es correcta. Lo que la venció fue que **las tres condiciones de abajo se
dieron simultáneamente**:

1. **El estándar está congelado.** El argumento de mantenimiento supone un algoritmo que
   evoluciona: parámetros que se endurecen, primitivas que se deprecian. RFC 6238 se publicó
   en 2011 y no puede cambiar — si cambiara rompería todas las apps autenticadoras que ya
   existen. No hay flujo de mantenimiento que heredar. **Argon2id NO cumple esto**, y por eso
   `password.go` sí usa dependencia.
2. **Hay vectores de prueba publicados y externos.** El Apéndice B del RFC fija una clave,
   cinco instantes y los códigos exactos. Eso es evidencia más fuerte que la suite de tests de
   una dependencia: no la escribió quien escribió el código, y mide interoperabilidad con la
   app real del usuario en vez de consistencia interna. Sin vectores externos, la única prueba
   de que tu implementación es correcta es tu propia opinión.
3. **La dependencia caería donde un compromiso no se recupera.** En `internal/auth` la
   asimetría decide: escribirlo cuesta líneas bajo la revisión del proyecto; depender cuesta
   superficie de cadena de suministro permanente en el paquete cuyo compromiso entrega el
   sistema entero en vez de degradarlo.

**Si falta cualquiera de las tres, gana la regla original.**

## Lo que NO es el argumento

**El tamaño.** Argon2id también son pocas líneas de llamada y sigue siendo una dependencia,
correctamente. "Son solo 50 líneas" es la racionalización con la que se escribe cripto casera
mala, no un criterio.

## El contraejemplo que viene enseguida

`T-02-014` implementa JWT. Falla las tres condiciones: JOSE tiene familias de algoritmos, modos
de confusión conocidos (`alg: none`, confusión HS/RS) y una historia **activa** de
vulnerabilidades de implementación. Ahí manda P2-D11 con `golang-jwt/v5`, y este caso no es
precedente.

Que el contraejemplo esté en la tarea siguiente es la mitad útil del recuerdo: la regla se
aplicó dos veces seguidas con resultados opuestos, y eso es lo que prueba que es un criterio y
no una preferencia.
