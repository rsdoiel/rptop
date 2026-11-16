package reading

import "testing"

func TestRenderings(t *testing.T) {
	r := Reading{Name: "temp.soc", Value: 54.6, Unit: "°C", Digits: 1}
	if got := r.String(); got != "54.6 °C" {
		t.Errorf("String() = %q", got)
	}
	if got := r.Number(); got != "54.6" {
		t.Errorf("Number() = %q", got)
	}
	if got := r.JSON(); got != `{"name":"temp.soc","value":54.6,"unit":"°C"}` {
		t.Errorf("JSON() = %s", got)
	}
}

func TestDigitsAndEmptyUnit(t *testing.T) {
	r := Reading{Name: "fan", Value: 3725, Unit: "rpm"}
	if got := r.String(); got != "3725 rpm" {
		t.Errorf("String() = %q", got)
	}
	v := Reading{Name: "volts.core", Value: 0.7838, Unit: "V", Digits: 4}
	if got := v.Number(); got != "0.7838" {
		t.Errorf("Number() = %q", got)
	}
	n := Reading{Name: "x", Value: 3, Unit: ""}
	if got := n.String(); got != "3" {
		t.Errorf("empty unit String() = %q", got)
	}
}
