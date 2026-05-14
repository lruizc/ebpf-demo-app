package config

// City represents a configured weather city with its geographic coordinates.
type City struct {
	Slug        string
	DisplayName string
	Latitude    float64
	Longitude   float64
}

// DefaultCities returns the built-in South-American city catalog.
func DefaultCities() []City {
	return []City{
		{Slug: "bogota", DisplayName: "Bogotá", Latitude: 4.7110, Longitude: -74.0721},
		{Slug: "lima", DisplayName: "Lima", Latitude: -12.0464, Longitude: -77.0428},
		{Slug: "santiago", DisplayName: "Santiago", Latitude: -33.4489, Longitude: -70.6693},
		{Slug: "buenos-aires", DisplayName: "Buenos Aires", Latitude: -34.6037, Longitude: -58.3816},
		{Slug: "caracas", DisplayName: "Caracas", Latitude: 10.4806, Longitude: -66.9036},
		{Slug: "quito", DisplayName: "Quito", Latitude: -0.1807, Longitude: -78.4678},
		{Slug: "montevideo", DisplayName: "Montevideo", Latitude: -34.9011, Longitude: -56.1645},
		{Slug: "sao-paulo", DisplayName: "São Paulo", Latitude: -23.5505, Longitude: -46.6333},
	}
}
