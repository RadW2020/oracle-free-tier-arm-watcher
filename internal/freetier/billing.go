package freetier

import (
	"math"
	"time"
)

// MinDaysForProjection es lo mínimo que tiene que llevar el mes para
// extrapolar el egress. Con un día de datos, una descarga puntual se
// convertiría en una proyección de 30 descargas.
const MinDaysForProjection = 3.0

// EgressProjection extrapola el egress del mes a su final.
type EgressProjection struct {
	DaysElapsed   float64
	DaysInMonth   int
	RatePerDayGB  float64
	MonthEndGB    float64
	CrossesOn     *time.Time // día en que cruzaría el límite, si cruza este mes
	Projectable   bool
	NotProjection string // por qué no se proyecta, si no se proyecta
}

// ProjectEgress hace una proyección lineal del egress acumulado del mes.
//
// Lineal a propósito: es la que se puede explicar en una frase y la que un
// agente puede comprobar a mano. El tráfico de esta máquina es regular
// (~8,5 GB/día); para uno con picos, la proyección avisa pronto y de más,
// que es el lado bueno por el que equivocarse cuando lo que está en juego es
// una factura.
func ProjectEgress(usedGB, limitGB float64, now time.Time) EgressProjection {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 1, 0)
	p := EgressProjection{
		DaysElapsed: now.Sub(start).Hours() / 24,
		DaysInMonth: int(next.Sub(start).Hours() / 24),
	}
	if p.DaysElapsed < MinDaysForProjection {
		p.NotProjection = "insufficient_history"
		return p
	}
	p.Projectable = true
	p.RatePerDayGB = usedGB / p.DaysElapsed
	p.MonthEndGB = math.Round(p.RatePerDayGB*float64(p.DaysInMonth)*10) / 10
	if p.RatePerDayGB > 0 && p.MonthEndGB >= limitGB && usedGB < limitGB {
		daysToLimit := (limitGB - usedGB) / p.RatePerDayGB
		crosses := now.Add(time.Duration(daysToLimit * 24 * float64(time.Hour)))
		if crosses.Before(next) {
			day := time.Date(crosses.Year(), crosses.Month(), crosses.Day(), 0, 0, 0, 0, time.UTC)
			p.CrossesOn = &day
		}
	}
	return p
}
