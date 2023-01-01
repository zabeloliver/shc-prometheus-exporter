package main

type Room struct {
	//Type string `json:"@type"`
	Id   string `json:"id"`
	Name string `json:"name"`
}

type DeviceServices struct {
	Service string
}

type Device struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Service     []string `json:"deviceServiceIds"`
	Room        string   `json:"roomId"`
	DeviceModel string   `json:"deviceModel"`
}

type Thermostat struct {
	Id          string
	Room        string
	Temperature float64
	Humidity    float64
}

type StateTemperature struct {
	Type  string  `json:"@type"`
	Value float64 `json:"temperature"`
}

type StateHumidity struct {
	Type  string  `json:"@type"`
	Value float64 `json:"humidity"`
}

type DeviceEvent struct {
	Type     string         `json:"@type"`
	Id       string         `json:"id"`
	State    map[string]any `json:"state"`
	DeviceId string         `json:"deviceId"`
}
