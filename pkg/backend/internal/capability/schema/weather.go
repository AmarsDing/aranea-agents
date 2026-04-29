package schema

// WeatherInput and WeatherOutput are kept as the canonical example schema
// described by the design document. They are not registered as a runtime
// backend until a weather provider is configured.
type WeatherInput struct {
	City string `json:"city" jsonschema:"city name, e.g. Shanghai"`
	Unit string `json:"unit,omitempty" jsonschema:"temperature unit: celsius or fahrenheit"`
}

type WeatherOutput struct {
	City        string  `json:"city"`
	Temperature float64 `json:"temperature"`
	Unit        string  `json:"unit"`
	Summary     string  `json:"summary"`
}
