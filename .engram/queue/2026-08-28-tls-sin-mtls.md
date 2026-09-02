---
type: architecture
score: 4
topic_key: mascotapp/security/transport
task: T-00-011
status: guardado
observation_id: obs-b24f892dc7c32085
rationale: "Evita re-discutir mTLS cada trimestre y evita implementarlo por culto al cargo."
---

MascotApp usa **TLS 1.3 sin mTLS**. Motivo por canal:

- Navegador a API: cliente público, no hay forma operativa de distribuir/rotar certs.
- API a Neon Postgres: Neon autentica con SCRAM sobre TLS; no ofrece auth por cert de cliente.
- API a Cloudflare R2: firma HMAC SigV4; mTLS no es parte del protocolo.

**Costo que lo descarta solo:** Cloud Run no soporta certs de cliente en `*.run.app`.
Exige Global External ALB + Certificate Manager: ~USD 18–25/mes fijos más USD 0.45 por
millón de conexiones. Destruye el requisito de costo cero.

**Por qué en integraciones con VISA sí era obligatorio:** mandato contractual del
adquirente bajo PCI-DSS, canal B2B con endpoints fijos y PAN en tránsito. MascotApp
**no procesa pagos** (solo enlaza a links propios del refugio), así que queda **fuera de
alcance PCI** y ese driver no existe.

Se implementa en su lugar: TLS 1.3 mínimo, HSTS preload, `sslmode=verify-full` con CA
pinneada (esto neutraliza el MITM), access tokens de 15 min con rotación de refresh.

Modo de fallo evitado: **un certificado mTLS vencido es caída total**, no degradación.

Condiciones de reapertura en ADR-0004.
