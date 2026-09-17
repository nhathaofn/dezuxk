package models

// GatewayStatus represents the running state and network address of the API Gateway.
type GatewayStatus struct {
	IsRunning bool   `json:"isRunning"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
}
