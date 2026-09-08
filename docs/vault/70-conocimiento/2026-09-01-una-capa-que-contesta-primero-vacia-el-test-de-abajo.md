---
type: bug
score: 5
topic_key: mascotapp/convention/the-layer-that-answers-first
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-9e00e3a6413dc7d2
task: T-01-024
rationale: "Tercera vez en tres tareas que un test sigue verde porque otra capa contesta antes que la que decia probar. Es un patron, no un accidente, y la deteccion siempre vino de la mutacion."
---

# Cuando otra capa contesta primero, el test de abajo deja de probar lo que dice

Un test que espera un rechazo **no prueba cual capa lo rechazo**. Si una capa mas arriba
contesta antes, el test sigue verde y la capa que decia fijar puede desaparecer sin que nadie se
entere.

Este proyecto lo vio **tres veces**, y las tres las encontro la mutacion, nunca la lectura:

1. **T-01-025 — `TRUNCATE` y la foreign key nueva.** Con `form_submissions` apuntando a
   `form_template_versions`, un `TRUNCATE` plano se rechaza con `0A000` **antes** de llegar al
   trigger `BEFORE TRUNCATE`. El test del trigger podia pasar con el trigger borrado.
   `TRUNCATE ... CASCADE` es el bypass de esa capa, y es lo unico que llega al trigger.
2. **T-01-024 — `ON DELETE RESTRICT` y el trigger de inmutabilidad.** Cambiar la referencia de
   `form_submissions` a `ON DELETE CASCADE` dejo la suite **entera** en verde: todos los casos
   que borraban una version borraban una **publicada**, asi que el trigger de 00007 contestaba
   primero. La referencia pudo haber estado cascadeando desde el dia que se escribio. Lo que la
   ejercita es un **DRAFT** que junto submissions — el caso que la propia migracion nombra y
   que nadie afirmaba — porque un draft es borrable por diseno y el trigger se corre al costado.
3. **T-01-022 — dos capas con el mismo SQLSTATE.** `GRANT INSERT ON pets TO app_public` dejo los
   dos casos conductuales verdes, porque `public_catalog` es `FOR SELECT` y la politica INSERT
   faltante rechaza con el **mismo** `42501`. Ahi lo que se corrigio fue el **comentario**, no
   el test: la enumeracion es lo que fija los grants.

**La regla que sale de esto:** despues de escribir un test que espera un rechazo, preguntar
*"si borro la capa que digo estar probando, esto se pone rojo?"*. Si no se puede contestar
leyendo, se contesta mutando. Y si la respuesta es que no, hay dos salidas legitimas —
**construir el caso que si la ejercita** (el draft, el `CASCADE`), o **corregir lo que el test
dice que prueba**. Dejarlo como esta no es ninguna de las dos.

El corolario incomodo: **agregar una capa puede vaciar un test que ya existia y estaba bien**.
La foreign key de 00008 no rompio nada; desactivo la asercion del trigger de 00007 sin tocarla.
Por eso la ronda de mutacion se corre por tarea y no una sola vez al final.

Ver [[2026-08-31-truncate-es-la-escritura-que-ninguna-politica-ve]] y
[[2026-08-31-dos-capas-un-solo-sqlstate]].
