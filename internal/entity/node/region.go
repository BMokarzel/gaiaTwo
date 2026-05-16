package node

// Region representa uma região geográfica do provedor.
type Region struct {
	Base
	ProviderID ProviderID `json:"provider_id"`
	Code       string     `json:"code"` // "us-east-1", "europe-west1"
	Continent  string     `json:"continent,omitempty"`
	Country    string     `json:"country,omitempty"`
}

// Zone representa uma AZ/zona dentro de uma região.
type Zone struct {
	Base
	RegionURN URN    `json:"region_urn"`
	Code      string `json:"code"` // "us-east-1a"
}
