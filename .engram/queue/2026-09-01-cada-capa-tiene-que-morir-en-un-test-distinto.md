---
type: convention
score: 5
topic_key: mascotapp/convention/each-layer-dies-in-its-own-test
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-5ea94dcd00c0adb7
task: T-01-029
rationale: "Afirmar 'defensa en profundidad con N capas' no significa nada hasta que se muestra que sacar cualquiera de las N pone algo en rojo. Es la unica forma de distinguir N capas de una capa mas N-1 adornos."
---

# Cada capa tiene que morir en un test DISTINTO

Decir *"esto esta protegido por cuatro capas independientes"* no significa nada hasta que se
muestra que **sacar cualquiera de las cuatro pone algo en rojo**. Si no, lo que hay es **una** capa
y tres adornos que se leen tranquilizadores.

La forma de demostrarlo es una ronda de mutacion donde cada mutante saca **exactamente una** capa,
y despues se mira **cual test murio**. `application_events` (T-01-029):

| Capa quitada | Murio en |
|---|---|
| trigger `BEFORE UPDATE OR DELETE` | los casos de owner (`update`, `delete`) |
| trigger `BEFORE TRUNCATE` | el caso `truncate` |
| grants `UPDATE`/`DELETE` revocados | el inventario de grants **y** el caso A/B |
| policies partidas por comando | el inventario de policies |

**Cuatro capas, cuatro tests distintos.** Si dos capas mueren en el MISMO test y en ningun otro,
en realidad hay una sola propiedad afirmada y las otras capas estan sin cobertura — el veredicto
KILLED las tapa.

**Por que cada capa existe, que es lo que decide contra QUE actor la probas:**

- los **grants** paran al rol de aplicacion ordinario → se prueba como `app_tenant`;
- la **ausencia** de policy `UPDATE`/`DELETE` bajo `FORCE` para un grant restaurado y al owner →
  se prueba en el inventario, porque su ausencia no se puede ejercitar;
- los **triggers** paran a un rol con `BYPASSRLS`, que RLS no toca → **se prueban como OWNER**, que
  es el unico actor contra el que las otras dos no valen nada.

**Y el SQLSTATE es lo que separa las capas en el resultado.** El `RAISE` de plpgsql da `P0001`,
distinto a proposito del `42501` del grant. Un test que solo afirma *"algo lo rechazo"* deja que
una capa cubra a la otra y no lo vas a notar nunca.

Ver [[una-capa-que-contesta-primero-vacia-el-test-de-abajo]] — el modo de falla inverso, donde una
capa que contesta ANTES vacia el test de la capa de abajo.
