---
type: bug
score: 4
topic_key: mascotapp/convention/mutation-kill-requires-a-fail-line
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-85289f0218a5964f
task: T-01-013
rationale: "Un harness que puntúa mejor cuanto peor se porta la máquina invierte el signo de la señal."
---

# Un harness de mutación que cuenta cualquier exit no-cero como "mutante muerto" mide el host, no el suite

El harness era: aplicar el mutante, correr `go test`, y

```python
return p.returncode == 0    # False => "killed"
```

En este host Windows, Application Control rechaza binarios de test recién
linkeados. Ese rechazo **nunca corre un solo test** y sale con código 1. El
harness lo acreditaba como mutante cazado.

O sea: **cuanto peor se portaba la máquina, mejor puntuaba el suite.** El signo
de la señal estaba invertido.

Se descubrió al no aceptar un resultado favorable sin poder explicarlo: un
mutante que yo había predicho que sobreviviría dio "killed", fui a ver por qué, y
apareció el bloqueo.

## La regla

Un runner de mutación tiene **tres** resultados, no dos: *murió*, *sobrevivió*, y
**no corrió**. Colapsar el tercero en el primero es el sesgo que más caro sale,
porque produce números más altos justo cuando el entorno está peor.

## Dato lateral, y contradice lo anotado antes

Ese mutante fue rechazado **8 veces seguidas** por Application Control. El
bloqueo es **determinista por contenido del binario**, no aleatorio — lo que
descarta las dos explicaciones por timing que se registraron en T-01-009 y
T-01-012.
