package llmparse

import (
	"fmt"
	"reflect"
	"sort"
)

// Strategy interface
type Strategy interface {
	Name() string
	Detect(input string) float64
	Parse(input string, target any) error
}

// Converter interface
type Converter interface {
	Name() string
	Supports(input any, target any) float64
	Convert(input any, target any) error
}

// Engine orchestrates parsing + conversion
type Engine struct {
	strategies []Strategy
	converters []Converter
}

// NewEngine creates engine with strategies and converters
func NewEngine(strategies []Strategy, converters []Converter) *Engine {
	return &Engine{strategies: strategies, converters: converters}
}

// Parse executes strategies + converters to produce typed output
func (e *Engine) Parse(input string, target any) error {
	var intermediate any

	// Step 1: parse using strategies
	if err := e.parseWithStrategies(input, &intermediate); err != nil {
		return err
	}

	// Step 2: try direct mapping
	if err := mapToTarget(intermediate, target); err == nil {
		return nil
	}

	// Step 3: try converters
	type scoredConv struct {
		c     Converter
		score float64
	}
	var scored []scoredConv
	for _, c := range e.converters {
		score := c.Supports(intermediate, target)
		if score > 0 {
			scored = append(scored, scoredConv{c, score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	for _, sc := range scored {
		if err := sc.c.Convert(intermediate, target); err == nil {
			return nil
		}
	}

	return fmt.Errorf("unable to convert parsed data")
}

// parseWithStrategies attempts each strategy in confidence order
func (e *Engine) parseWithStrategies(input string, target any) error {
	type scoredStrategy struct {
		s     Strategy
		score float64
	}
	var scored []scoredStrategy
	for _, s := range e.strategies {
		score := s.Detect(input)
		if score > 0 {
			scored = append(scored, scoredStrategy{s, score})
		}
	}

	if len(scored) == 0 {
		return fmt.Errorf("no strategy detected")
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	for _, ss := range scored {
		if err := ss.s.Parse(input, target); err == nil {
			return nil
		}
	}

	return fmt.Errorf("all strategies failed")
}

// -----------------------------
// Optional helper for converters
// -----------------------------
func assignStringSliceToStructField(list []string, target any) error {
	v := reflect.ValueOf(target).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(list))
			return nil
		}
	}
	return fmt.Errorf("no compatible field found")
}
