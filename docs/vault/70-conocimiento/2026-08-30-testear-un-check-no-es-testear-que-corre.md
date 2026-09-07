---
type: convention
score: 4
topic_key: mascotapp/convention/guarantees-must-be-observable
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-c12eb1e15c811174
task: T-01-008 (role guard)
rationale: "Un check correcto y no aplicado se lee idéntico a uno aplicado. La cobertura lo muestra verde y la revisión lo aprueba. Solo lo atrapa preguntar quién observa que corrió."
---

**Testear un check no es testear que el check corre.**

`T-01-008` construyó el guard de ADR-0002: el rol con el que conecta la suite no puede tener
`rolsuper` ni `rolbypassrls`. Se probó bien:

- `CheckRoleCannotBypassRLS` es pura, con las seis combinaciones afirmadas
- Se probó **contra un rol que de verdad puede saltear** — el owner del contenedor es
  superusuario, y el test exige que el guard lo rechace

Cinco mutantes. Cuatro murieron. **El quinto sobrevivió: borrar la llamada al guard desde el
constructor del pool no rompió nada.**

Y ese mutante era el DoD entero de la tarea: *"el guard corre en el setup del harness, no como
test suelto que un filtro `-run` puede saltear."*

El check era **correcto** y **no estaba aplicado**. El único test que lo ejercitaba lo llamaba
él mismo, directo. El harness podía no llamarlo nunca y todo seguía en verde.

## El arreglo: hacer la garantía observable

```go
func guardPool(...) RoleCapabilities   // devuelve lo que verificó
env.guarded[role] = guardPool(...)     // el harness lo registra
```

Y un test afirma que ambos roles están en ese registro. Borrar la llamada borra el registro, y
el test falla nombrando el riesgo: *"el harness entregó un pool de app_tenant sin guardarlo;
toda aserción A/B de esta fase pasaría vacuamente si ese rol pudiera saltear RLS."*

## La regla

Cuando una garantía se enuncia como **"esto pasa durante el setup"**, algo tiene que
**observar que pasó**. Si no, la garantía es un comentario.

Y el corolario práctico: la pregunta que hay que hacerse frente a cualquier chequeo de
seguridad no es *"¿está bien escrito?"* sino **"¿qué test falla si borro la llamada?"** Si la
respuesta es "ninguno", el chequeo es decorativo por más correcto que sea.

Mismo patrón, tercera vez en esta fase: [[2026-08-29-mutation-testing-vacuous-coverage]] (el
test no tenía elementos después del punto de parada) y el mutante M10 de `T-01-007` (el test de
ida y vuelta `up → down → up` no miraba el estado del medio). **Los tres eran verdes.**
