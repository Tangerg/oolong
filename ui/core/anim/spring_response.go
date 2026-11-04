package anim

import "math"

type springStep struct {
	positionWeight         springCoefficient
	anchorAtTarget         bool
	velocityToPosition     springCoefficient
	displacementToVelocity springCoefficient
	velocityWeight         springCoefficient
}

func (step springStep) advance(position, target, velocity float64) (float64, float64) {
	displacement := coefficient(target).subtract(coefficient(position))
	anchor, direction := position, 1.0
	if step.anchorAtTarget {
		anchor, direction = target, -1
	}
	nextPosition := coefficient(anchor).
		add(step.positionWeight.multiply(displacement).scale(direction)).
		add(step.velocityToPosition.scale(velocity))
	nextVelocity := step.displacementToVelocity.multiply(displacement).
		add(step.velocityWeight.scale(velocity))
	return nextPosition.value(), nextVelocity.value()
}

// Either endpoint can carry the significant part of a position: subtracting a
// tiny response from one would otherwise erase motion near the start or target.
func (step springStep) withPosition(progress, retention springCoefficient) springStep {
	step.positionWeight = progress
	if math.Abs(retention.value()) < math.Abs(progress.value()) {
		step.positionWeight = retention
		step.anchorAtTarget = true
	}
	return step
}

func springResponse(frequency, damping float64) springStep {
	const seriesRadius = 0.5
	attenuation := frequency * damping
	if frequency <= seriesRadius && attenuation <= seriesRadius {
		return seriesStep(frequency, attenuation)
	}
	switch {
	case damping < 1:
		return oscillatingStep(frequency, damping)
	case damping == 1:
		return criticalStep(frequency)
	default:
		return overdampedStep(frequency, damping)
	}
}

// Integrating the velocity response keeps the O(frequency²) position response
// separate from the cancelling O(frequency) terms in the closed forms.
func seriesStep(frequency, attenuation float64) springStep {
	squared := frequency * frequency
	previous, term := 0.0, 1.0
	response, derivative, integral := 1.0, 1.0, 0.5
	// With negligible damping, odd and even terms alternate; one rounded-zero
	// term cannot establish convergence of this second-order recurrence.
	const convergedTerms = 2
	unchanged := 0
	for order := 1.0; unchanged < convergedTerms; order++ {
		next := -(2*attenuation*order*term + squared*previous) / (order * (order + 1))
		updatedResponse := response + next
		updatedDerivative := derivative + (order+1)*next
		updatedIntegral := integral + next/(order+2)
		if updatedResponse == response && updatedDerivative == derivative && updatedIntegral == integral {
			unchanged++
		} else {
			unchanged = 0
		}
		response, derivative, integral = updatedResponse, updatedDerivative, updatedIntegral
		previous, term = term, next
	}
	frequencySquared := coefficient(frequency).scale(frequency)
	progress := frequencySquared.scale(integral)
	step := springStep{
		velocityToPosition:     coefficient(response),
		displacementToVelocity: frequencySquared.scale(response),
		velocityWeight:         coefficient(derivative),
	}
	return step.withPosition(progress, coefficient(1).subtract(progress))
}

func oscillatingStep(frequency, damping float64) springStep {
	ratio := math.Sqrt((1 - damping) * (1 + damping))
	phase, attenuation := frequency*ratio, frequency*damping
	sin, cos := math.Sincos(phase)
	decay := springDecay(attenuation)
	response := decay.scale(sin).divide(coefficient(phase))
	correction := response.scale(attenuation)
	retention := decay.scale(cos).add(correction)
	halfSin := math.Sin(phase / 2)
	progress := coefficient(-math.Expm1(-attenuation)).
		add(decay.scale(2 * halfSin * halfSin)).
		subtract(correction)
	step := springStep{
		velocityToPosition:     response,
		displacementToVelocity: response.scale(frequency).scale(frequency),
		velocityWeight:         decay.scale(cos).subtract(correction),
	}
	return step.withPosition(progress, retention)
}

func criticalStep(frequency float64) springStep {
	decay := springDecay(frequency)
	retention := decay.scale(1 + frequency)
	step := springStep{
		velocityToPosition:     decay,
		displacementToVelocity: decay.scale(frequency).scale(frequency),
		velocityWeight:         decay.scale(1 - frequency),
	}
	return step.withPosition(coefficient(1).subtract(retention), retention)
}

func overdampedStep(frequency, damping float64) springStep {
	inverse := 1 / damping
	ratio := math.Sqrt((1 - inverse) * (1 + inverse))
	// Rationalizing the slow root avoids subtracting nearly equal roots; keeping
	// its exponent also preserves motion when frequency/damping underflows.
	slow := coefficient(frequency).divide(coefficient(damping)).scale(1 / (1 + ratio))
	fast := coefficient(frequency).scale(damping).scale(1 + ratio)
	gap := coefficient(frequency).scale(damping).scale(2 * ratio)
	decay := springDecay(slow.value())
	response := decay.multiply(integratedDecay(gap))
	correction := slow.multiply(response)
	progress := slow.multiply(integratedDecay(slow).subtract(response))
	retention := decay.add(correction)
	step := springStep{
		velocityToPosition:     response,
		displacementToVelocity: response.scale(frequency).scale(frequency),
		velocityWeight:         springDecay(fast.value()).subtract(correction),
	}
	return step.withPosition(progress, retention)
}

func integratedDecay(rate springCoefficient) springCoefficient {
	value := rate.value()
	if value == 0 {
		return coefficient(1)
	}
	if value <= 1 {
		return coefficient(-math.Expm1(-value) / value)
	}
	return coefficient(-math.Expm1(-value)).divide(rate)
}

// Eight factors keep the exponential normal until even three MaxFloat64
// factors could not rescue the final product from underflow.
func springDecay(attenuation float64) springCoefficient {
	const squarings = 3
	decay := coefficient(math.Exp(-attenuation / (1 << squarings)))
	for range squarings {
		decay = decay.multiply(decay)
	}
	return decay
}

// A response may underflow before multiplying a large state makes it
// representable again. Only the final position and velocity may lose range.
type springCoefficient struct {
	fraction float64
	exponent int
}

func coefficient(value float64) springCoefficient {
	fraction, exponent := math.Frexp(value)
	return springCoefficient{fraction: fraction, exponent: exponent}
}

func (c springCoefficient) value() float64 {
	return math.Ldexp(c.fraction, c.exponent)
}

func (c springCoefficient) scale(value float64) springCoefficient {
	return c.multiply(coefficient(value))
}

func (c springCoefficient) multiply(other springCoefficient) springCoefficient {
	if c.fraction == 0 || other.fraction == 0 {
		return springCoefficient{}
	}
	product := coefficient(c.fraction * other.fraction)
	product.exponent += c.exponent + other.exponent
	return product
}

func (c springCoefficient) divide(other springCoefficient) springCoefficient {
	if c.fraction == 0 {
		return springCoefficient{}
	}
	quotient := coefficient(c.fraction / other.fraction)
	quotient.exponent += c.exponent - other.exponent
	return quotient
}

func (c springCoefficient) add(other springCoefficient) springCoefficient {
	if c.fraction == 0 {
		return other
	}
	if other.fraction == 0 {
		return c
	}
	if c.exponent < other.exponent {
		return other.add(c)
	}
	sum := coefficient(c.fraction + math.Ldexp(other.fraction, other.exponent-c.exponent))
	if sum.fraction != 0 {
		sum.exponent += c.exponent
	}
	return sum
}

func (c springCoefficient) subtract(other springCoefficient) springCoefficient {
	other.fraction = -other.fraction
	return c.add(other)
}
