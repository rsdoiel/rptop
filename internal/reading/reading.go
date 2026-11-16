// Package reading is the one value-and-unit type every rptop collector
// produces, with the three renderings DR-0002 asks for.
package reading

import (
	"encoding/json"
	"strconv"
)

/** Reading is one measured element: a name, a value and its unit.
 *
 * Fields:
 *   Name   (string)  — dotted element name, for example "temp.soc"
 *   Value  (float64) — the measurement
 *   Unit   (string)  — "°C", "rpm", "V", "bytes", "MHz", "" for a bare number
 *   Digits (int)     — decimal places shown by String and Number
 *
 * Example:
 *   r := reading.Reading{Name: "temp.soc", Value: 54.6, Unit: "°C", Digits: 1}
 *   r.String() // "54.6 °C"
 */
type Reading struct {
	Name   string
	Value  float64
	Unit   string
	Digits int
}

/** Number returns the bare value, formatted with r.Digits places.
 *
 * Returns:
 *   string — for example "54.6"
 *
 * Example:
 *   r.Number() // "54.6"
 */
func (r Reading) Number() string {
	return strconv.FormatFloat(r.Value, 'f', r.Digits, 64)
}

/** String returns "VALUE UNIT", or just the value when the unit is empty.
 *
 * Returns:
 *   string — for example "54.6 °C"
 *
 * Example:
 *   fmt.Println(r) // 54.6 °C
 */
func (r Reading) String() string {
	if r.Unit == "" {
		return r.Number()
	}
	return r.Number() + " " + r.Unit
}

/** JSON returns the reading as a single-line JSON object.
 *
 * Returns:
 *   string — {"name":...,"value":...,"unit":...}; value keeps r.Digits places
 *
 * Example:
 *   r.JSON() // {"name":"temp.soc","value":54.6,"unit":"°C"}
 */
func (r Reading) JSON() string {
	name, _ := json.Marshal(r.Name)
	unit, _ := json.Marshal(r.Unit)
	return `{"name":` + string(name) + `,"value":` + r.Number() + `,"unit":` + string(unit) + `}`
}
