---
type: architecture
score: 5
topic_key: mascotapp/arch/policy-lands-at-version-aware-exemption
task: T-02-004
rationale: "El catalogo (rlstest/catalog.go) tiene mas de un consumidor -- el meta-test de HEAD y el walk stepwise de rollback -- y una unica lista de excepciones (NoPolicy) no puede describir correctamente a los dos a la vez el dia que una tabla gana su politica en una migracion posterior a su creacion. El patron que lo resuelve es transferible a cualquier declaracion de esquema con mas de un lector con horizontes temporales distintos."
---

**Sacar una tabla de `NoPolicy` puede romper un test que la tarea que la escribio no
menciona, si esa tabla existio (con RLS, sin policy) antes de la migracion que le da su
primera politica.**

## El hueco

`rlstest.Schema` es una unica declaracion, pero tiene DOS consumidores con horizontes de
tiempo distintos:

- `TestCatalog_EveryRelationIsClassifiedAndProtected` corre contra el esquema en HEAD --
  todas las migraciones aplicadas.
- `TestMigrations_EveryStepDownLeavesAConsistentSchema` (el walk stepwise) corre la MISMA
  regla (`CheckProtection`) contra el esquema en CADA version intermedia, bajando de a una
  migracion por vez.

Cuando una tabla se crea con RLS encendida pero sin politica en la migracion N, y recien
gana su primera politica real en la migracion M > N (el caso de `refresh_tokens`: creada en
`00002`, con politica desde `00013`), `NoPolicy` tiene que decir DOS cosas incompatibles a
la vez: "en HEAD, esta tabla SI tiene que tener politica" (para que el meta-test la exija) y
"entre N y M, esta tabla NO tiene que tener politica" (para que el walk no lea una migracion
que todavia no corrio como un rollback roto). Una lista sin nocion de version no puede
sostener las dos afirmaciones al mismo tiempo.

## El arreglo: version-aware, no un caso especial hardcodeado

`Classification.PolicyLandsAt map[string]int64` -- tabla -> version de goose en la que
aterriza su primera politica. Mismo patron que `Pending` (que ya resuelve el problema
analogo para "la tabla no existe todavia"), pero para un hueco distinto: `Pending` es sobre
EXISTENCIA, `PolicyLandsAt` es sobre PROTECCION dentro de una tabla que ya existe.

`CheckProtection(t)` (sin version, HEAD-only) NO consulta `PolicyLandsAt` -- sigue
exactamente tan estricta como antes. `CheckProtectionAt(t, at)` es la que el walk llama, y
trata la tabla como exenta de "debe tener politica" solo cuando `at < PolicyLandsAt[tabla]`.
Las dos comparten la regla real (`checkProtection`, privada) para que RLSEnabled/RLSForced no
diverjan entre las dos rutas.

## La leccion transferible

Una declaracion de esquema con mas de un consumidor, y consumidores con horizontes de
tiempo distintos (uno mira el estado final, el otro mira el historial completo), no puede
resolverse con una sola lista de excepciones atemporal. La lista necesita, o bien una
dimension de tiempo explicita (esto), o bien consumidores separados con reglas separadas --
lo primero es mas barato cuando la mayoria de la logica se comparte y solo una condicion de
borde cambia. Antes de sacar algo de una excepcion existente, preguntar: **¿todos los
consumidores de esta declaracion asumen el mismo punto en el tiempo?**

---

## Addendum del gatekeeping: una exencion que nadie chequea deriva sola

El mecanismo aterrizo sin nada que atara el **numero** a la realidad. `Validate()` probaba
que la entrada nombra una tabla declarada y que no se contradice con `NoPolicy`; ninguna de
las dos cosas dice que la policy realmente aterrice en la 13.

- Un numero declarado **de menos** ya lo atrapaba `CheckProtectionAt`, exigiendo una policy
  que todavia no llego.
- Un numero declarado **de mas** no lo atrapaba nadie, y es la direccion peligrosa: la
  exencion tapa versiones en las que la policy YA existe, y el walk deja de chequear estados
  reales. Deriva en la direccion permisiva -- exactamente el modo de falla que este catalogo
  existe para evitar, y la razon por la que sus otras listas se derivan en vez de escribirse
  a mano.

**Regla general: toda exencion se ata a la realidad que exime.** Si la exencion es un
numero, algo tiene que fallar cuando ese numero no coincide con lo observable. Se verifico
por mutacion: declarar 20 en vez de 13 pone rojo el walk con un mensaje que nombra la
version, el conteo y la declaracion.

## Y donde va la asercion: `at` es una etiqueta, no un estado

El primer intento la puso dentro de `checkCatalogAt`, **y estaba mal**. Esa funcion recibe
`at` como una etiqueta del que llama, y sus propios unit tests la ejercitan contra una base
**ya migrada del todo**: ahi "version 1" no describe nada sobre el esquema que tiene
adelante, y la asercion rechazaba un esquema sano.

Lo encontro el subtest `a_clean_intermediate_schema_passes`, que existe justamente para eso:
atrapar a quien agrega una regla que rechaza el esquema bueno. **Un test negativo que prueba
que el checker acepta lo correcto vale tanto como el que prueba que rechaza lo roto.**

La asercion vive en el **walk**, que es el unico lugar donde la base fue realmente revertida
a `at` y donde comparar las dos cosas significa algo.
