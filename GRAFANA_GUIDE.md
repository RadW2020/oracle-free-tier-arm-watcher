# 📊 Integración con Grafana Cloud

Este proyecto ahora expone métricas en formato Prometheus, lo que permite monitorizar tu Oracle Free Tier directamente desde Grafana Cloud.

## ¿Qué necesitamos?

1. **Oracle Free Tier Watcher** en funcionamiento (este proyecto).
2. Una cuenta en **Grafana Cloud** (tienen un tier gratuito generoso).
3. **Grafana Alloy** (recomendado) o el antiguo Grafana Agent instalado en tu servidor.

## Pasos para la configuración

### 1. Activar el endpoint de métricas

El Watcher ahora expone automáticamente las métricas en:
`http://localhost:8088/metrics`

Puedes configurar la frecuencia de actualización en tu archivo `.env`:

```env
METRICS_INTERVAL=15m
```

_Nota: Se recomienda un intervalo de entre 5 y 15 minutos para evitar demasiadas llamadas a la API de Oracle._

### 2. Instalar Grafana Alloy en tu instancia de Oracle

Sigue las instrucciones oficiales de Grafana Cloud para instalar Alloy en Linux (ARM64 si usas Ampere). Generalmente es un comando similar a este:

```bash
curl -fsSL https://apt.grafana.com/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/grafana.gpg
echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" | sudo tee /etc/apt/sources.list.d/grafana.list
sudo apt-get update
sudo apt-get install alloy
```

### 3. Configurar Alloy (`/etc/alloy/config.alloy`)

Configura Alloy para que escanee tanto el Watcher (métricas de cuenta) como el propio sistema (métricas de VM). Necesitarás tu **Remote Write URL**, **User ID** y **API Key** desde el portal de Grafana Cloud.

```hcl
// 1. Métricas de la CUENTA (vía Watcher)
prometheus.scrape "oracle_watcher" {
  targets = [{"__address__" = "localhost:8088"}]
  forward_to = [prometheus.remote_write.grafana_cloud.receiver]
}

// 2. Métricas de la VM (CPU, RAM, Disco, etc.)
prometheus.exporter.unix "local_system" {
  include_exporter_metrics = true
}

prometheus.scrape "local_system" {
  targets    = prometheus.exporter.unix.local_system.targets
  forward_to = [prometheus.remote_write.grafana_cloud.receiver]
}

// 3. Envío a Grafana Cloud
prometheus.remote_write "grafana_cloud" {
  endpoint {
    url = "https://prometheus-prod-XX-XXXX.grafana.net/api/prom/push"
    auth {
      username = "TU_USER_ID"
      password = "TU_GRAFANA_CLOUD_API_KEY"
    }
  }
}
```

### 4. ¿Qué estamos viendo en cada métrica?

- **Métricas `oci_...`**: Son métricas de **toda tu cuenta Oracle**. No importa en qué VM corran tus cosas, estas métricas te dicen cuánto te queda para llegar al límite de la Free Tier.
- **Métricas `node_...`**: Son métricas de **esta VM específica**. Te dicen si la CPU de este servidor está al 100% o si te estás quedando sin memoria RAM real.

### 5. Visualización en Grafana

Una vez que los datos lleguen a Grafana Cloud, puedes crear un Dashboard usando estas métricas:

- `oci_compute_arm_ocpus_used` / `limit`
- `oci_compute_arm_memory_gb_used` / `limit`
- `oci_compute_amd_instances_used` / `limit`
- `oci_block_storage_gb_used` / `limit` (Total)
- `oci_storage_boot_volumes_gb` (Solo boot)
- `oci_storage_block_volumes_gb` (Solo block)
- `oci_object_storage_gb_used` / `limit` (Total)
- `oci_object_storage_bucket_gb{bucket_name="..."}` (Por bucket)
- `oci_database_autonomous_used` / `limit` (Instancias)
- `oci_database_storage_gb_used` / `limit` (Almacenamiento DB)
- `oci_public_ips_used` / `limit`
- `oci_overall_status` (0=OK, 1=ATTENTION, 2=WARNING, 3=CRITICAL)

## Métricas Disponibles

| Métrica                             | Descripción                                 |
| ----------------------------------- | ------------------------------------------- |
| `oci_compute_arm_ocpus_used`        | OCPUs ARM en uso                            |
| `oci_compute_arm_memory_gb_used`    | Memoria ARM (GB) en uso                     |
| `oci_compute_amd_instances_used`    | Instancias AMD Micro en uso                 |
| `oci_block_storage_gb_used`         | Almacenamiento en bloque total (GB)         |
| `oci_storage_boot_volumes_gb`       | Almacenamiento de volúmenes de arranque     |
| `oci_object_storage_gb_used`        | Object Storage total (GB)                   |
| `oci_object_storage_bucket_gb`      | Tamaño por bucket (usa label `bucket_name`) |
| `oci_database_autonomous_used`      | Bases de datos Autonomous en uso            |
| `oci_database_storage_gb_used`      | Almacenamiento de bases de datos (GB)       |
| `oci_public_ips_used`               | IPs públicas reservadas                     |
| `oci_overall_status`                | Estado general (0-3)                        |
| `oci_watcher_last_update_timestamp` | Fecha última sincronización (Unix)          |

---

_Tip: Puedes configurar alertas en Grafana Cloud para que te avisen por Discord/Telegram si `oci_overall_status > 1`._
