package main

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	roomTemperature   *prometheus.GaugeVec
	roomHumidity      *prometheus.GaugeVec
	switchState       *prometheus.GaugeVec
	shutterLevel      *prometheus.GaugeVec
	energyConsumption *prometheus.GaugeVec
	powerConsumption  *prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		roomTemperature: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "room_temperature",
				Help: "Current room temperature in degree celsius.",
			},
			[]string{"id", "room"}),
		roomHumidity: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "room_humidity",
				Help: "Current room humidity in percent.",
			},
			[]string{"id", "room"},
		),
		switchState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "switch_state",
				Help: "Current state of switch.",
			},
			[]string{"id", "room"},
		),
		shutterLevel: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "shutter_level",
				Help: "Current shutter level.",
			},
			[]string{"id", "room"},
		),
		energyConsumption: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "total_power_consumption",
				Help: "Total energy Consumption.",
			},
			[]string{"id", "room"},
		),
		powerConsumption: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "actual_power_consumption",
				Help: "Actual energy consumption.",
			},
			[]string{"id", "room"},
		),
	}
	reg.MustRegister(m.roomTemperature)
	reg.MustRegister(m.roomHumidity)
	reg.MustRegister(m.switchState)
	reg.MustRegister(m.shutterLevel)
	reg.MustRegister(m.energyConsumption)
	reg.MustRegister(m.powerConsumption)
	return m
}
