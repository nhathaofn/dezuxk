package models

// AppSettings represents configuration options manageable from the UI.
type AppSettings struct {
	GatewayPort int    `json:"gatewayPort"`
	GatewayIP   string `json:"gatewayIP"`
	IsRunning   bool   `json:"isRunning"`
}
