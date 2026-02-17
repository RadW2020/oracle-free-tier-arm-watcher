# TODO: Bandwidth Monitoring

## ⚠️ Estado actual

El watcher NO monitoriza bandwidth/egress automáticamente.

**Riesgo:** El bandwidth es el recurso más peligroso del Free Tier.
- Límite: 10 TB/mes
- Precio si excedes: ~$85 por TB extra
- Fácil de exceder con: DDoS, bots, serving de archivos grandes

## 🛡️ Protección TEMPORAL (mientras se implementa)

### 1. Budget Alert en OCI (CRÍTICO - HAZLO HOY)

```
OCI Console → Billing & Cost Management → Budgets
→ Create Budget:
   - Name: Free Tier Safety
   - Amount: $1.00
   - Threshold: 1% ($0.01)
   - Email: tu-email
```

**Si gastas $0.01 en CUALQUIER cosa (incluido bandwidth), recibirás email.**

### 2. Verificación manual semanal

```
OCI Console → Governance → Usage Reports
→ Filter: "Data Transfer Out" o "egress"
→ Verificar: < 10 TB/mes
```

### 3. Cloudflare (opcional pero muy recomendado)

Si pones Cloudflare delante de tu API:
- Cache reduce requests
- Protección DDoS gratis
- Límite de rate automático

## 📝 Implementación pendiente

### Endpoint `/bandwidth` a añadir:

```go
type BandwidthUsage struct {
    CurrentMonth struct {
        EgressGB      float64 `json:"egressGB"`
        LimitTB       int     `json:"limitTB"`
        Percentage    int     `json:"percentage"`
        DaysRemaining int     `json:"daysRemaining"`
        ProjectedGB   float64 `json:"projectedGB"`
    } `json:"currentMonth"`
    Status   string   `json:"status"`
    Warnings []string `json:"warnings"`
}
```

### Requiere:

1. **OCI Usage API SDK**
   ```go
   import "github.com/oracle/oci-go-sdk/v65/usageapi"
   ```

2. **Permisos adicionales** en tu OCI User:
   ```
   Allow group YourGroup to read usage-report in tenancy
   ```

3. **Query de metrics**:
   ```go
   // Agregar egress traffic del mes actual
   // Calcular % usado
   // Proyectar uso fin de mes
   ```

## 🚀 Prioridad

**ALTA** - Implementar en los próximos días.

Mientras tanto, el Budget Alert de $1 es tu red de seguridad.

## 📚 Recursos

- [OCI Usage API Docs](https://docs.oracle.com/en-us/iaas/Content/Billing/Concepts/usageapi.htm)
- [OCI Go SDK Usage API](https://pkg.go.dev/github.com/oracle/oci-go-sdk/v65/usageapi)
