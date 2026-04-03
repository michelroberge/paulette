package main

import (
	"fmt"
	"github.com/michelroberge/paulette/backend/internal/llmparse"
)

type FruitList struct {
	Items []string `json:"items"`
}

func main() {
	engine := llmparse.NewEngine(
		[]llmparse.Strategy{
			llmparse.JSONStrategy{},
			llmparse.ListStrategy{},
			llmparse.CodeStrategy{},
		},
		[]llmparse.Converter{
			llmparse.StringListToStruct{},
			llmparse.KVToStruct{},
		},
	)

	llmOutput := `
Sure! Here's your list:

1. Apple
2. Banana
3. Cherry
`

	var result FruitList
	err := engine.Parse(llmOutput, &result)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Parsed: %+v\n", result)
}
