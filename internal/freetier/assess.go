package freetier

import "fmt"

// Estados de la cuota. Los cuatro primeros son los de siempre; UNKNOWN sólo
// lo emite la API /v1 (ver Assessment.Verdict).
const (
	StatusOK        = "OK"
	StatusAttention = "ATTENTION"
	StatusWarning   = "WARNING"
	StatusCritical  = "CRITICAL"
	StatusUnknown   = "UNKNOWN"
)

// Assessment separa las dos familias de cuota del Free Tier.
//
// Hay cuotas que estan al 100 % porque alguien decidio usarlas enteras: las
// 4 OCPUs ARM, los 24 GB de RAM, los 200 GB de disco. Un Free Tier bien
// aprovechado vive exactamente ahi, y un status que grita CRITICAL por eso
// es un semaforo siempre en rojo: nadie lo mira, y cuando de verdad pasa
// algo tampoco lo mira. Esta maquina llevaba meses en CRITICAL.
//
// Y hay cuotas que suben solas con el uso —object storage, almacenamiento de
// base de datos y egress— donde llegar al tope si tiene consecuencia: una
// factura. Esas son las que merecen escalar el estado.
//
// La distincion no borra informacion: los porcentajes de asignacion siguen
// publicandose (AllocationPercentage y las metricas por recurso), asi que
// una alerta sobre "las OCPUs cambiaron" sigue siendo posible. Lo que deja
// de hacer es confundir "esta lleno porque asi lo quisiste" con "se esta
// llenando solo".
type Assessment struct {
	// AccruingPercentage es el maximo de las cuotas que se llenan solas.
	AccruingPercentage int
	// AllocationPercentage es el maximo de las cuotas asignadas por diseno.
	AllocationPercentage int
	// Status es el estado heredado: el que publican /usage, /status y el
	// gauge oci_overall_status. No sabe de datos que faltan —una fuente
	// caída cuenta como 0 %— y se deja así a propósito para no cambiar el
	// contrato; Verdict es la versión que sí lo sabe.
	Status   string
	Warnings []string
	// Unavailable lista las cuotas acumulativas cuyo dato no llegó.
	Unavailable []string
	// Verdict es Status corregido por lo que no se sabe: si todo lo conocido
	// está en OK pero falta una cuota acumulativa, el veredicto es UNKNOWN.
	// Un CRITICAL conocido no se esconde detrás de un dato que falta.
	Verdict string
}

// Complete dice si llegaron todas las cuotas acumulativas.
func (a Assessment) Complete() bool { return len(a.Unavailable) == 0 }

// Assess clasifica el uso y decide el estado general.
func Assess(r Reading) Assessment {
	usage := &r.Usage
	assessment := Assessment{Warnings: []string{}, Unavailable: []string{}}

	// --- Cuota que se llena sola: escala el estado ---
	for _, q := range AccruingQuotas() {
		if !r.Available(q.Source) {
			assessment.Unavailable = append(assessment.Unavailable, q.Name)
			continue
		}
		percentage := q.legacyPercentage(usage)
		if percentage <= 0 {
			continue
		}
		if percentage > assessment.AccruingPercentage {
			assessment.AccruingPercentage = percentage
		}
		if percentage >= q.WarnThreshold {
			assessment.Warnings = append(assessment.Warnings,
				fmt.Sprintf("%s at %d%%%s", q.Name, percentage, q.warningDetail(usage)))
		}
	}

	// --- Cuota asignada por diseno: se informa, no escala ---
	allocated := []int{
		usage.Compute.ARM.OCPUs.Percentage,
		usage.Compute.ARM.MemoryGB.Percentage,
		usage.Compute.AMD.Instances.Percentage,
		usage.BlockStorage.Total.Percentage,
		usage.PublicIPs.Percentage,
		usage.Database.AutonomousDBs.Percentage,
	}
	for _, p := range allocated {
		if p > assessment.AllocationPercentage {
			assessment.AllocationPercentage = p
		}
	}

	assessment.Status = statusFor(assessment.AccruingPercentage)
	assessment.Verdict = assessment.Status
	if assessment.Verdict == StatusOK && !assessment.Complete() {
		assessment.Verdict = StatusUnknown
	}
	return assessment
}

func statusFor(accruing int) string {
	switch {
	case accruing >= 90:
		return StatusCritical
	case accruing >= 80:
		return StatusWarning
	case accruing >= 60:
		return StatusAttention
	}
	return StatusOK
}

// UnavailableWarnings describe las fuentes que no respondieron, para que la
// API heredada también lo diga aunque su status no cambie.
func UnavailableWarnings(r Reading) []string {
	warnings := []string{}
	for _, name := range AllSources {
		if s := r.Status(name); !s.Available {
			warnings = append(warnings, fmt.Sprintf("%s data unavailable (%s): values for it are not real zeros", name, s.ErrorCode))
		}
	}
	return warnings
}

// SaturationWarnings avisa de la saturación, sin tocar el status.
//
// Un paquete descartado por el shaper de OCI no acerca la factura ni un
// centimo, asi que subir el status por esto haria saltar los checks de
// cuota por algo que no lo es. Aparece como warning porque es lo unico
// que distingue "todo va lento" de "no pasa nada".
//
// Antes sólo lo emitía /usage; /status contestaba la misma pregunta sin él.
func SaturationWarnings(s SaturationUsage) []string {
	if s.IngressDropsLastHour > 0 {
		return []string{fmt.Sprintf(
			"OCI dropped %.0f inbound packets in the last hour (VNIC ingress throttle): other services on this host are losing SYNs",
			s.IngressDropsLastHour)}
	}
	return nil
}
