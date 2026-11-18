package usage

import (
	"math/big"
	"slices"
)

// ModelPrice lists a model's unit prices in US cents per million tokens.
type ModelPrice struct {
	ID                 string
	Name               string
	UncachedInputCents uint64
	CachedInputCents   uint64
	OutputCents        uint64
	Promotional        bool
}

var modelPrices = []ModelPrice{
	{"gpt-6.1-sol", "GPT-6.1 Sol", 200, 10, 1000, false},
	{"gpt-6.2-sol", "GPT-6.2 Sol", 200, 10, 1000, false},
	{"gpt-6.2-astra", "GPT-6.2 Astra", 800, 40, 4000, false},
	{"gpt-6.1-terra", "GPT-6.1 Terra", 100, 5, 500, false},
	{"gpt-6-astra", "GPT-6 Astra", 1000, 100, 5000, false},
	{"gpt-6-sol", "GPT-6 Sol", 200, 20, 1000, false},
	{"gpt-6-luna", "GPT-6 Luna", 10, 1, 50, false},
	{"gpt-5.6-sol", "GPT-5.6 Sol", 400, 40, 2000, true},
	{"gpt-5.6-terra", "GPT-5.6 Terra", 200, 20, 1200, false},
	{"gpt-5.6-luna", "GPT-5.6 Luna", 20, 2, 120, false},
}

var modelPriceByID = func() map[string]ModelPrice {
	byID := make(map[string]ModelPrice, len(modelPrices))
	for _, price := range modelPrices {
		byID[price.ID] = price
	}
	return byID
}()

// ModelPrices returns the prices used for estimates in display order.
func ModelPrices() []ModelPrice {
	return slices.Clone(modelPrices)
}

// APICost estimates the USD subtotal for usage that could be priced.
type APICost struct {
	USD        *string `json:"usd"`
	Incomplete bool    `json:"incomplete"`
}

// APICostBreakdown gives the estimates for the token usage categories.
type APICostBreakdown struct {
	Input              APICost `json:"input"`
	CachedInput        APICost `json:"cachedInput"`
	UncachedInput      APICost `json:"uncachedInput"`
	CacheWriteInput    APICost `json:"cacheWriteInput"`
	Output             APICost `json:"output"`
	ReasoningOutput    APICost `json:"reasoningOutput"`
	NonReasoningOutput APICost `json:"nonReasoningOutput"`
}

type costTally struct {
	cachedInput        costAmount
	uncachedInput      costAmount
	cacheWriteInput    costAmount
	output             costAmount
	reasoningOutput    costAmount
	nonReasoningOutput costAmount
	incomplete         bool
}

type costAmount struct {
	// One unit is 1/1,000,000 of a cent. The integer is immutable because
	// tallies are copied by value when read from the index.
	units      *big.Int
	incomplete bool
}

func (c *costTally) addUsage(usage tokenUsage, model string) {
	price, known := modelPriceByID[model]
	if !known || usage.Input == nil || usage.CachedInput == nil || usage.Output == nil {
		c.incomplete = true
		return
	}
	input, cached, output := *usage.Input, *usage.CachedInput, *usage.Output
	if cached > input || input > *usage.Total || output != *usage.Total-input {
		c.incomplete = true
		return
	}
	uncached := input - cached
	c.uncachedInput.addTokens(uncached, price.UncachedInputCents)
	c.cachedInput.addTokens(cached, price.CachedInputCents)
	c.output.addTokens(output, price.OutputCents)

	if written := usage.CacheWriteInput; written != nil && *written <= uncached {
		c.cacheWriteInput.addTokens(*written, price.UncachedInputCents)
	} else {
		c.cacheWriteInput.incomplete = true
	}
	if reasoning := usage.ReasoningOutput; reasoning != nil && *reasoning <= output {
		c.reasoningOutput.addTokens(*reasoning, price.OutputCents)
		c.nonReasoningOutput.addTokens(output-*reasoning, price.OutputCents)
	} else {
		c.reasoningOutput.incomplete = true
		c.nonReasoningOutput.incomplete = true
	}
}

func (c *costTally) add(other costTally) {
	c.cachedInput.add(other.cachedInput)
	c.uncachedInput.add(other.uncachedInput)
	c.cacheWriteInput.add(other.cacheWriteInput)
	c.output.add(other.output)
	c.reasoningOutput.add(other.reasoningOutput)
	c.nonReasoningOutput.add(other.nonReasoningOutput)
	c.incomplete = c.incomplete || other.incomplete
}

func (c costTally) estimate(logsIncomplete bool) (APICost, APICostBreakdown) {
	// Sum the independently billed inputs and outputs before rounding.
	// Cache writes and both output subdivisions belong to those same totals.
	input := c.uncachedInput
	input.add(c.cachedInput)
	total := input
	total.add(c.output)
	incomplete := c.incomplete || logsIncomplete
	return total.estimate(incomplete), APICostBreakdown{
		Input:              input.estimate(incomplete),
		CachedInput:        c.cachedInput.estimate(incomplete),
		UncachedInput:      c.uncachedInput.estimate(incomplete),
		CacheWriteInput:    c.cacheWriteInput.estimate(incomplete),
		Output:             c.output.estimate(incomplete),
		ReasoningOutput:    c.reasoningOutput.estimate(incomplete),
		NonReasoningOutput: c.nonReasoningOutput.estimate(incomplete),
	}
}

func (c *costAmount) addTokens(tokens, centsPerMillion uint64) {
	if tokens == 0 && c.units != nil {
		return
	}
	var count, rate big.Int
	units := new(big.Int).Mul(count.SetUint64(tokens), rate.SetUint64(centsPerMillion))
	c.add(costAmount{units: units})
}

func (c *costAmount) add(other costAmount) {
	c.incomplete = c.incomplete || other.incomplete
	if c.units == nil {
		c.units = other.units
	} else if other.units != nil {
		c.units = new(big.Int).Add(c.units, other.units)
	}
}

func (c costAmount) estimate(incomplete bool) APICost {
	result := APICost{Incomplete: c.incomplete || incomplete}
	if c.units != nil {
		amount := new(big.Rat).SetFrac(c.units, big.NewInt(100_000_000))
		result.USD = new(amount.FloatString(2))
	} else if !result.Incomplete {
		result.USD = new("0.00")
	}
	return result
}
