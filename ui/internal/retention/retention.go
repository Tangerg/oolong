// Package retention gives long-lived collections one capacity policy so a past
// burst does not become permanent storage.
package retention

const (
	capacityMultiple = 2
	allocationSlack  = 16
)

// ShouldCompact bounds retained peaks while allowing small spare allocations to
// avoid churn. Division keeps the threshold safe at integer limits.
func ShouldCompact(capacity, live int) bool {
	if live <= 0 {
		return capacity > 0
	}
	if capacity <= allocationSlack {
		return false
	}
	return (capacity-allocationSlack-1)/capacityMultiple >= live
}
