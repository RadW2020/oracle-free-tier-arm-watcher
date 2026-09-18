# Postmortem — 17/09/2026: la descarga de AIDRA tumba los checks del host

**Estado:** cerrado con mitigación desplegable (falta desplegar)
**Detectado por:** Checkly (email) · **Duración del impacto:** ~17:13 → ~20:05 CEST
**Escrito:** 18/09/2026

Todas las horas en **CEST** (UTC+2). Las consultas de OCI devuelven UTC; la
conversión está hecha.

---

## 1. Qué se vio

Siete emails de Checkly que en realidad son **dos incidencias**, porque
`standardEscalation` (`checkly/src/escalation.ts`) avisa al primer run fallido
y repite 2 veces cada 10 min:

| Emails | Incidencia real |
|---|---|
| 17:14 / 17:24 / 17:34 — *Failure, Shogunito Web UI* | Un único run fallido (17:13:12) + 2 recordatorios |
| 17:58 / 18:08 — *Degradation, Strong Core Web* | Un único run degradado (17:48, 10.547 ms) + 1 recordatorio |
| 18:13 / 18:18 — *Recovery* | Los siguientes runs de cada check |

El fallo no fue de aplicación: `dial tcp4 80.225.189.40:443: i/o timeout`,
20 s **sin completar el handshake TCP** (`tcp: 19.979 ms`). La petición nunca
llegó al servidor.

Detrás de los dos checks que alertaron, **todo el host se degradó a la vez**
durante ~2 h:

| Check | Normal | Durante |
|---|---|---|
| Shogunito API Health | 165 ms | 3.828 ms |
| Strong Core Historial (SSR) | 180 ms | 3.098 ms |
| AIDRA API Health | 180 ms | 2.101 ms |
| Ciaobox Public Health | 240 ms | 1.499 ms |
| Shogunito MinIO S3 API | 160 ms | 1.021 ms |

El dato que señala la capa culpable: **el handshake TCP pasó de 29 ms
(constante todo el día) a 211–260 ms**, y los tiempos observados (0,9 / 1,5 /
3,8 / 5,9 / 10,5 s) son la escalera de retransmisión de TCP. Eso es pérdida de
paquetes, no CPU ni aplicación.

---

## 2. Causa raíz

**El pipeline de AIDRA descargando escenas Sentinel-1 satura el ingress de la
VNIC, y OCI responde descartando paquetes de entrada — incluidos los SYN de
todo lo demás que vive en la máquina.**

Confirmado con los logs de la propia AIDRA (Loki, vía
`POST {GRAFANA_URL}/api/ds/query`) y las métricas de OCI Monitoring:

```
17:14:25  Scheduled scan starting for zone 'gibraltar'   (trigger_type: scheduled)
17:14:27  Starting product download  S1C_IW_GRDH_1SDV_20260915T061827...SAFE
17:14:29  Cannot determine file size; falling back to single-stream
17:19:28  Download complete  downloaded_mb: 1698.39      (1,7 GB en 5 min = ~45 Mbps)
17:21-18:15  detección YOLO+CFAR sobre 1.363 tiles       (CPU plana al 35,5 %)
18:15:05  Pipeline completed successfully  (total_ms: 3.640.543 = 60,7 min)
```

Y lo que OCI midió en esa misma ventana:

| Métrica (`oci_vcn` / `oci_computeagent`) | Valor |
|---|---|
| `VnicFromNetworkBytes` | 379 MB/min sostenidos (≈50 Mbps), 250.000 paq/min de 1.516 B |
| `VnicIngressDropsThrottle` | **~81.000 paquetes descartados en 6 min** (0 antes, 0 después) |
| `CpuUtilization` | 6 % → 35,5 % plano durante 55 min → 6 % |
| `MemoryUtilization` | 30 % → 31 % |
| `LoadAverage` | 1,5 sobre 4 OCPUs |
| `VnicConntrackIsFull` / `...DropsConntrackFull` | 0 |
| `VnicEgressDropsThrottle` | 0 |

### Qué es `VnicIngressDropsThrottle`

Es el contador de **paquetes que OCI tira antes de entregarlos a la VNIC de la
instancia, porque el tráfico de entrada supera el caudal permitido para la
forma de la VM**. No es un límite del sistema operativo ni de la app: lo aplica
la red virtual de Oracle, aguas arriba de la instancia, y por eso **no deja
rastro dentro de la máquina**: no aparece en `dmesg`, ni en las estadísticas de
la interfaz, ni en los logs de Traefik. Para la VM, esos paquetes sencillamente
nunca existieron.

Por qué eso rompe cosas que no tienen nada que ver con la descarga: el
descarte no distingue flujos. Cuando el shaper está tirando el ~5 % de lo que
entra, tira ese 5 % de *todo* lo que entra. Si lo que cae es un paquete de
datos de la descarga, TCP lo retransmite y no se nota. Si lo que cae es el SYN
de la petición de Checkly, el cliente espera 1 s, reintenta, espera 2 s,
reintenta, 4 s… y a los 20 s se rinde con `i/o timeout` — que es exactamente el
error del email de las 17:14. Lo mismo, en versión suave, explica los
0,9–10,5 s de los demás checks: cada retransmisión perdida se paga a golpe de
RTO duplicado.

### Por qué ayer y no los demás días

El ciclo de las 17:15 es rutinario (`scheduler_interval_hours = 6`: 05:15,
11:15, 17:15 y 23:15) y el patrón CPU+descarga de los días 15, 16 y 17 es
idéntico. Lo que cambió ayer fue **el bucle de Tip & Cue**: cada ejecución
genera cues que disparan otra ejecución, y **cada una vuelve a descargar la
misma escena de 1.698 MB**, porque el pipeline borra el `.zip` y el `.SAFE` al
terminar (`engine.py:699-704`, limpieza deliberada por disco) y el guard
`Product already downloaded, skipping` nunca encuentra el fichero.

```
18:29:25  Pipeline starting (trigger_type: cue)  → descarga la MISMA escena
18:55:18  Download attempt failed, retrying  (attempt 1, peer closed connection)
19:09:20  Download attempt failed, retrying  (attempt 2)
19:26:11  Download complete 1698.39 MB
19:27:24  Pipeline starting (trigger_type: cue)  → otra vez la misma escena
19:32:21  Download complete 1698.39 MB
19:44:26  ...  19:54:29  Download complete 1698.39 MB
19:56:02  ...  20:03:19  Download complete 1698.39 MB
```

Cuatro descargas redundantes del mismo fichero, ~6,8 GB, entre las 18:29 y las
20:05. Eso convirtió una ráfaga de 6 minutos al día en hora y media de
transferencias encadenadas, y es lo que solapó tantos runs de Checkly. La
degradación de las 19:13 (5.916 ms) cae dentro de esta segunda tanda.

### Cabo suelto honesto

El run que falló (17:13:12–17:13:37) es **~50 s anterior** al arranque del
scheduler de AIDRA (17:14:25) y cae en un minuto con 0 drops registrados. Con
los datos disponibles no es atribuible a la descarga: puede ser un blip de
camino Frankfurt→Madrid o del propio runner. La misma reserva vale para el
Ciaobox lento de las 17:04. Lo que sí queda explicado sin ambigüedad es el
resto de la ventana, incluida la degradación que alertó.

---

## 3. Lo que NO fue

- **Ni Shogunito ni Strong Core.** Sus aplicaciones estaban sanas; les tiraron
  los paquetes por debajo.
- **Ni CPU, ni RAM, ni disco.** 35 % de 4 OCPUs, load 1,5, memoria al 31 %, IO
  en línea base.
- **Ni los límites del Free Tier.** Nada cerca de ningún tope.
- **Ni conntrack ni security lists.** Contadores a cero / ruido habitual.
- **Ni un pico de egress.** El egress real de la VNIC son 8,5 GB/día. Ojo con
  `NetworksBytesOut` de `oci_computeagent`: marca ~50 GB/día porque **suma las
  interfaces de Docker**. Para tráfico real hacia internet, mirar siempre
  `oci_vcn` / `VnicToNetworkBytes`.

---

## 4. Por qué el watcher no vio nada

Porque **no está construido para ver esto**. El watcher es un vigilante de
*cuota*, no de *saturación*:

- Sus 12 métricas (`metrics.go`) son todas «usado vs. límite»: OCPUs, RAM,
  block storage, object storage, IPs, balanceadores, Autonomous DB. Ninguna
  habla de rendimiento.
- La única métrica de red que consulta es `VnicToNetworkBytes`
  (`oci.go:450-495`) — **egress**, la dirección contraria a la que rompió — y
  la agrega con `[1d].sum()` desde el día 1 del mes para calcular el % de los
  10 TB. Una ráfaga de 6 minutos es invisible a resolución de 1 día.
- `oci_overall_status` sólo escala por porcentajes de cuota, así que se quedó
  en OK con toda la razón.
- Y aunque hubiera mirado: 1,9 GB de **entrada** no consumen ninguna cuota del
  Free Tier (el ingress es gratis), así que nunca habría disparado nada.
- Cadencia: el watcher refresca cada 15 min (`METRICS_INTERVAL`) y el check
  `Oracle Free Tier Monitor` que lo lee corre cada 3 h.

No es un fallo del watcher: es un hueco de cobertura. Los datos que sí
explican la incidencia (`VnicFromNetworkBytes`, `VnicIngressDropsThrottle`,
`CpuUtilization`) están en la API de Monitoring de OCI, gratis, y el watcher ya
tiene cliente y credenciales para pedirlas.

---

## 5. Acciones

### Hecha (pendiente de desplegar)

**Techo de ancho de banda en la descarga de AIDRA** — repo `AIDRA`:

- `src/config.py`: `download_rate_limit_mbps: float = 20.0` (0 = sin límite).
- `src/pipeline/ingestion.py`: `DownloadThrottle`, un token bucket **compartido
  por todos los workers** (un límite por conexión no acotaría nada: lo que
  satura es la suma). Se aplica a los dos caminos de descarga, y el tamaño de
  lectura del socket se reduce para que el caudal sea liso en vez de ráfagas a
  velocidad de línea.
- `src/main.py`, `docker-compose*.yml`, `.env.example`: el valor viaja desde
  `Settings` (`DOWNLOAD_RATE_LIMIT_MBPS`).
- `tests/test_pipeline/test_ingestion_throttle.py`: 10 tests. `ruff` limpio,
  93 tests de pipeline y 99 de invariantes en verde.

A 20 Mbps una escena de 1,7 GB tarda ~11 min en vez de 5, muy dentro de las
6 h entre ciclos y de los 240 min del orphan reaper.

### Pendientes (no hechas)

1. **La re-descarga redundante de Tip & Cue.** Es la causa de que esto pasara
   de ráfaga diaria a hora y media de ruido. Opciones: conservar el `.zip`
   mientras haya cues vivos sobre esa escena, o que un run disparado por cue
   reutilice la escena del run que lo generó. Es un cambio de diseño en AIDRA,
   no una mitigación.
2. **Métricas de saturación en el watcher**: exponer `VnicFromNetworkBytes`,
   `VnicIngressDropsThrottle` y `CpuUtilization` junto a las de cuota. Es la
   diferencia entre «te quedan 9,7 TB» y «te están tirando paquetes».
3. **Check de Checkly sobre los drops**, al estilo de los de bandwidth, para
   que el próximo email diga la causa en vez del síntoma.

---

## 6. Cómo volver a mirar esto

```bash
export SUPPRESS_LABEL_WARNING=True
set -a && . ./.env && set +a

# Paquetes descartados por el shaper de OCI (el síntoma)
oci monitoring metric-data summarize-metrics-data -c "$OCI_TENANCY_ID" \
  --namespace oci_vcn --query-text 'VnicIngressDropsThrottle[1m].sum()' \
  --start-time 2026-09-17T15:00:00Z --end-time 2026-09-17T16:00:00Z

# Tráfico REAL de la VNIC (no el de computeagent, que suma Docker)
oci monitoring metric-data summarize-metrics-data -c "$OCI_TENANCY_ID" \
  --namespace oci_vcn --query-text 'VnicFromNetworkBytes[1m].sum()' \
  --start-time 2026-09-17T15:00:00Z --end-time 2026-09-17T16:00:00Z

# CPU / memoria / load del host
oci monitoring metric-data summarize-metrics-data -c "$OCI_TENANCY_ID" \
  --namespace oci_computeagent --query-text 'CpuUtilization[5m].mean()' \
  --start-time 2026-09-17T15:00:00Z --end-time 2026-09-17T17:00:00Z
```

Los logs de AIDRA se consultan sin SSH a través de su Grafana
(`GRAFANA_URL` en el `.env` de AIDRA, datasources `Loki` / `Prometheus` /
`PostgreSQL`):

```bash
curl -s -u "admin:$GRAFANA_PASSWORD" --get \
  "https://aidra.uliber.com/api/datasources/proxy/uid/Loki/loki/api/v1/query_range" \
  --data-urlencode '{service_name="aidra"} |~ "download|Pipeline"' \
  --data-urlencode "start=<ns>" --data-urlencode "end=<ns>"
```

Nota de método: la API de Monitoring etiqueta cada bucket con el **final** del
intervalo. El bucket `15:15` de una serie `[1m]` contiene 15:14:00–15:15:00.
