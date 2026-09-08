---
type: architecture
score: 5
topic_key: mascotapp/security/aead-aad-must-cover-the-unencrypted-header
task: T-02-011
status: guardado
observation_id: obs-5899d1ed7c1b5256
rationale: "Un formato con cabecera fuera del AAD parece correcto y pasa el round-trip: la cabecera no está cifrada porque no es secreta, y de ahí es fácil concluir que tampoco hace falta autenticarla. Pero un byte de versión o de key id que nadie autentica es un byte que un atacante con escritura sobre la columna edita a gusto, y la confusión de versiones es la forma de ataque que un byte de rotación invita por diseño. El diseño de esta fase especificaba el AAD como 'el id del usuario' y era incompleto; lo encontró un test de tampering, no una revisión."
---

# El AAD tiene que cubrir la cabecera, no solo el dato que la decisión nombró

El formato at-rest de `users.totp_secret_enc` (P2-D8) es:

```
version(1) || key_id(1) || nonce(12) || ciphertext || tag(16)
```

El diseño especificaba **`AAD = user.id`** y explicaba bien por qué: ata el blob a su fila, así
que un valor copiado de la columna de un usuario a la de otro no abre en vez de descifrarse en
un secreto TOTP válido.

**Estaba incompleto, y lo encontró un test de tampering, no una lectura.**

## El agujero

`version` y `key_id` no están cifrados —no son secretos— y de ahí se salta fácil a que tampoco
hace falta autenticarlos. Pero con `AAD = user.id` solamente, esos dos bytes quedan **fuera de
todo lo que el tag cubre**: un atacante con escritura sobre la columna los edita y nada lo
nota.

Y el byte peligroso es justamente el que existe para el futuro. **La confusión de versiones es
la forma de ataque que un byte de rotación invita por diseño**: el día que haya dos formatos o
dos claves, poder reescribir ese byte es poder elegir contra qué se valida.

## El arreglo

`AAD = version || key_id || user.id`. GCM autentica el AAD junto al ciphertext, así que
cualquier edición de la cabecera invalida el tag.

Se mantuvo **además** el chequeo explícito de `version` y `key_id` antes de descifrar. No es
redundancia inútil: da un error claro y temprano, y —esto lo confirmó la mutación— **cuando se
sacó el AAD entero, el caso de tampering sobre `key_id` siguió pasando**, porque el chequeo
explícito lo agarró. Dos mecanismos independientes sobre el mismo byte, y la mutación mostró
cuál cubre qué.

## La regla transferible

En cualquier formato AEAD con cabecera en claro, **el AAD tiene que cubrir toda la cabecera**,
no solo el campo que la decisión de diseño nombró. La pregunta al escribirlo no es "¿qué es
secreto?" sino **"¿qué pasa si un atacante reescribe este byte?"** — son preguntas distintas y
solo la segunda decide qué va en el AAD.

Corolario práctico: un caso de tampering tiene que tocar **cada región** del blob —cabecera,
nonce, ciphertext, tag—, no solo el ciphertext. Un test que solo edita el ciphertext pasa con
una cabecera completamente desprotegida.

Relacionado: [[2026-09-03-ab-probe-must-touch-a-writable-column]] y
[[2026-09-03-when-guard-kills-the-feature-not-just-the-edge-case]] — la misma familia: el alcance real
de una protección se mide, no se deduce de lo que su autor creyó estar cubriendo.
