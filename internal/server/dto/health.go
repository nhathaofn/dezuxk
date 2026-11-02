package dto

// HealthAccounts contains account readiness counts.
type HealthAccounts struct {
	Ready int `json:"ready"`
	Total int `json:"total"`
}

// HealthResponse represents the payload returned by GET /health.
type HealthResponse struct {
	Status    string         `json:"status"`
	Version   string         `json:"version"`
	UptimeSec int64          `json:"uptime_sec"`
	Accounts  HealthAccounts `json:"accounts"`
}
