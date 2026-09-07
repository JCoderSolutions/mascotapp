---
type: convention
score: 3
topic_key: mascotapp/convention/probe-names-come-from-the-ledger
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-6873e19af4076fac
task: T-01-019
rationale: "Un literal elegido porque 'todavía no existe' es una bomba de tiempo con fecha conocida."
---

# Una tabla sonda no toma prestado un nombre que el esquema va a reclamar

Un test de T-01-011 necesitaba una tabla **declarada pero todavía no creada** —
esa forma exacta, para que la regla de exhaustividad siguiera verde mientras la
de protección fallaba. Eligió `pets`, porque en ese momento `pets` estaba
declarada y no existía.

T-01-019 creó `pets`. El test murió con `42P07: relation already exists`.

Renombrarlo a otro literal — `documents`, `audit_log` — **solo posterga la misma
falla once veces más**, una por cada tabla que queda pendiente. Es una bomba de
tiempo con fecha conocida.

## El arreglo

Tomar el nombre **del ledger, en tiempo de ejecución**:

```go
probe := firstPendingTable(t)   // Schema.Pending, ordenado, determinista
```

Sobrevive a toda migración futura. Y cuando el ledger se vacíe al cerrar la fase,
hace `t.Skip` **con motivo escrito**: la forma que este test necesita — declarada
y ausente — dejó de existir, así que borren el subtest en vez de inventar un
nombre que la declaración no conoce.

## La regla

Si un test crea un objeto en el esquema compartido, su nombre tiene que ser
**imposible de colisionar** (`ab_*`, `*_probe`, `leftover_from_a_down`) **o
derivado de la misma fuente de verdad que decide qué existe.** Un literal elegido
porque "todavía no existe" no es ninguna de las dos.
