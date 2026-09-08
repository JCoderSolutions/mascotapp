---
type: convention
score: 4
topic_key: mascotapp/convention/a-rotating-survivor-outranks-a-stable-one
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-3b0eca453ae1938e
task: T-01-015
rationale: "Cuando el sobreviviente cambia de identidad entre rondas, el defecto está en el test, no en la cobertura."
---

# Un sobreviviente que **rota** entre rondas vale más que uno estable

Ronda 1: murió S1, sobrevivió S2.
Arreglo el hueco que explicaba S2.
Ronda 2: murió S2, **sobrevivió S1** — que ya había muerto.

Eso no es ruido. Un sobreviviente que cambia de identidad después de un arreglo
dice que el arreglo movió algo que no era cobertura, sino **una precondición
compartida del test**.

Los dos defectos estaban en el test, y los dos eran invisibles en verde:

1. El caso de fail-closed nombraba **una sola tabla**, así que romper la política
   de otra pasaba inadvertido. Arreglo: derivar la lista del catálogo
   (`Schema.Tenant` menos `Pending`), no escribirla a mano.
2. El caso de "conexión que nunca llevó scope" solo era **virgen para la primera
   tabla del loop** — el pool de una sola conexión ya había pasado por la
   anterior. Arreglo: un pool fresco por tabla.

El segundo lo *creó* el arreglo del primero. Sin la ronda 2 quedaba escondido
detrás de un 7/7 que no era cierto.

## La regla

Después de cerrar un sobreviviente, **volvé a correr la ronda entera**, no solo
el mutante que arreglaste. Y si el sobreviviente rota, mirá la precondición
compartida antes que la cobertura.

Corolario de [[2026-08-30-el-harness-de-mutacion-que-medía-el-host]]: la ronda de mutación
es un instrumento, y los instrumentos también se calibran.
