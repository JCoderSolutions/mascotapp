---
type: bug
score: 4
topic_key: mascotapp/convention/the-layer-that-answers-first
task: T-02-015
status: guardado
approved: 2026-09-04 por el usuario (T-02-015, cierre de PR-02-09)
observation_id: obs-9e00e3a6413dc7d2
extends: obs-9e00e3a6413dc7d2
nota_de_guardado: "El `mem_save` devolvió el MISMO `observation_id` que la instancia original, no uno nuevo: compartir `topic_key` suma la nota al hilo existente en vez de abrir uno aparte. Es lo correcto acá —es el mismo patrón, cuarta instancia— pero conviene saberlo: el `observation_id` identifica el TEMA, no el candidato. La trazabilidad fina de esta instancia vive en este archivo, no en el id."
rationale: "Cuarta instancia del patrón ya guardado, pero con un giro que el original no cubre y que cambia qué hay que hacer al respecto. Las tres anteriores fueron en tests de base de datos y la capa intrusa era del motor — RLS, un grant, un trigger — o sea algo que ya estaba ahí antes de escribir el test. Esta fue en Go puro y la capa intrusa la escribí YO, en la misma tarea, tres funciones más abajo. El patrón original dice 'verificá que la capa que decís medir es la que contesta'. Lo que falta es la consecuencia operativa: cada chequeo redundante que agregás es una capa nueva que puede contestar primero, y desarma en silencio todo test negativo escrito antes de que existiera. La defensa en profundidad no es el error; ignorar que reordena quién contesta, sí."
---

# Agregar una defensa en profundidad puede desarmar en silencio el test del chequeo de abajo

Cuarta instancia de `obs-9e00e3a6413dc7d2` (*la capa que contesta primero vacía el test de
abajo*), y la primera fuera de la base de datos.

## El caso

`Verify` en `apps/api/internal/auth/token.go` tiene dos defensas que un token falsificado
tiene que atravesar:

1. `jwt.WithValidMethods([]string{"HS256"})` — el allowlist de algoritmos, **la única**
   defensa contra confusión de algoritmos (`alg: none`, sustitución HS/RS).
2. `claims.validate()` **a la salida** — defensa en profundidad, agregada en la misma tarea,
   que refusa claims que no identifican a nadie (`sub` en cero, `role` vacío, `amr` vacío).

Dos tests de falsificación cubrían (1): uno con `alg: none`, otro con `alg: HS512` firmado con
el mismo secreto. Los dos rojos. Todo verde.

**El mutante que saca `jwt.WithValidMethods` SOBREVIVIÓ.** Los dos tests seguían rojos con la
defensa quitada.

Los payloads que esos tests forjaban no llevaban `amr`. Con el allowlist puesto los rechazaba
el chequeo de algoritmo; con el allowlist sacado los rechazaba `claims.validate()` a la
salida. **El resultado observable era idéntico en los dos casos.** Los tests nunca habían
medido el chequeo de algoritmo.

## El giro respecto de las tres instancias anteriores

Las tres primeras fueron en tests de base de datos: una policy RLS que contestaba antes que un
índice único, un grant que contestaba antes que una policy, un trigger que contestaba antes
que un `CHECK`. En todas, **la capa intrusa preexistía al test**. La lección era leer bien el
esquema antes de escribir la sonda.

Acá la capa intrusa **no existía cuando el test se escribió**. La escribí yo, en la misma
tarea, después. El test era correcto en el momento de escribirse y dejó de serlo por una
adición mía posterior — sin fallar, sin advertir nada, sin ningún cambio de color en la suite.

## La regla que sale de esto

**Todo chequeo redundante que agregás es una capa nueva que puede contestar primero.** Cuando
agregues una validación de defensa en profundidad, los tests negativos que ya existían para
las capas de abajo **quedan bajo sospecha**: cada uno hay que releerlo preguntando si su
entrada sigue siendo válida para todo salvo la cosa que dice probar.

La forma concreta: **una sonda negativa se construye con una entrada COMPLETA Y VÁLIDA en la
que lo único mal es exactamente el atributo bajo prueba.** Acá se extrajo un
`forgeablePayload(now)` que produce un conjunto de claims entero y correcto; el test solo le
rompe el header. Con eso el mutante muere.

No es que la defensa en profundidad esté mal — se queda, y hay un test propio para ella. Lo
que está mal es agregarla sin auditar a quién le sacó el turno.

## Y una segunda cosa que la mutación arreglada dijo

Con los payloads corregidos, al mutante lo mata **exactamente el caso de HS512, no el de
`alg: none`.** Sin el allowlist, `golang-jwt` rechaza `none` por su cuenta. O sea:
`jwt.WithValidMethods` es load-bearing **solo** para la sustitución HS256→HS512.

Eso importa para el próximo que lea el código. Si alguien saca el allowlist y comprueba que
"el test de `alg: none` sigue pasando", concluye que la opción es decorativa — y abre justo el
agujero que ese test no cubre.
