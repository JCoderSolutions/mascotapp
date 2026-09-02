---
type: convention
score: 4
topic_key: mascotapp/convention/recompute-announced-numbers
task: fase-02 planning
rationale: "Un agente reporto 23 PRs sumando 4.875 con 'todos <=400'. Recomputando contra los est: reales, uno daba 410 y el total 4.890. El presupuesto era justo lo que el usuario acababa de decidir."
---

# Un numero anunciado se recomputa contra su fuente, no se acepta

Un agente cerro la cadena de entrega de la Fase 02 con **23 PRs, 4.875 lineas, ninguno sobre
400**. Los tres numeros eran del mismo reporte y los tres se leian consistentes entre si.

**Recomputados contra los `est:` de cada tarea en el archivo**, uno no cerraba: `PR-02-05` daba
**410**, no 395, y el total era 4.890. El agente habia dicho que subia una estimacion de 180 a 200,
**escribio 215 en la tarea** y llevo 200 a la aritmetica. Un solo digito, en el unico PR que se
pasaba del presupuesto que el usuario acababa de elegir explicitamente por encima de pedir una
excepcion.

**La forma general.** Un total es una **afirmacion derivada**: no se lee, se **reconstruye desde
las partes**. Y la reconstruccion es barata — veinte lineas de script contra el archivo que ya
esta escrito. Lo caro es lo otro: un presupuesto que nadie recomputa deja de ser un presupuesto y
pasa a ser una decoracion, y la primera vez que eso se nota es cuando alguien intenta revisar el
PR que se paso.

**El corolario que decide como se arregla:** cuando el numero real supera el limite, **se mueve el
trabajo, no el numero**. Bajarle el estimado a la tarea para que entre es exactamente como un
presupuesto se vuelve ceremonia. Aca se partio el PR en dos — y el corte resulto mejor que el
original, porque `00015` y `00016` son **dos migraciones** y la propia regla del documento decia
*"una migracion es un PR"*.

Aplica igual a: conteos de cobertura, cantidad de tests, filas de un catalogo, requisitos y
escenarios en un merge de specs. Ver [[mascotapp/convention/enumerate-dont-list]].
