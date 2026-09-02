---
type: convention
score: 3
topic_key: mascotapp/ops/failed-report-is-not-lost-work
task: fase-02 planning
approved: 2026-09-02 (aprobacion explicita del usuario)
observation_id: obs-216cdb370cd7e725
rationale: "sdd-design se cayo por limite de sesion y se reporto como 'no escribio design.md'. Habia escrito 604 lineas completas. Se perdio una sesion de trabajo por no mirar el filesystem."
---

# Un agente que falla al reportar no necesariamente fallo al escribir

`sdd-design` se cayo por limite de sesion de proveedor. Su ultimo texto era *"Now I have the full
picture. Writing the design."*, y de ahi se concluyo —razonablemente, y **mal**— que no habia
alcanzado a escribir el artefacto.

**Habia escrito `design.md` completo**: 604 lineas, once decisiones, todas las secciones incluida
la de preguntas abiertas. Lo que falto fue el **espejo en Engram y el reporte**, que son los dos
ultimos pasos. Un `ls` del directorio lo habria dicho en un segundo.

**La forma general.** El texto de un agente es evidencia de **su intencion**, no de **su efecto**.
Los efectos estan en el filesystem, en la base, en el indice de git — y ahi es donde se miran.
Sobre todo cuando el fallo es **de infraestructura y no de la tarea**: un limite de sesion, una
desconexion o un timeout cortan *en algun punto*, y ese punto casi nunca es el que el ultimo
mensaje sugiere.

**El chequeo, antes de declarar perdido el trabajo o relanzar:** listar los artefactos que la fase
tenia que producir y ver cuales existen y con que tamano. Relanzar sobre trabajo ya hecho no es
gratis — cuesta otra sesion, y ademas puede **pisar** una version buena con una peor.
