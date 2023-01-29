package main

import (
	"bytes"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"

	"github.com/zabeloliver/shc-api/shcClient"
	"github.com/zabeloliver/shc-api/shcStructs"
)

type DeviceRoomMap map[string]string

type PrometheusMetrics struct {
	roomTemperature   *prometheus.GaugeVec
	roomHumidity      *prometheus.GaugeVec
	switchState       *prometheus.GaugeVec
	shutterLevel      *prometheus.GaugeVec
	energyConsumption *prometheus.GaugeVec
	powerConsumption  *prometheus.GaugeVec
}

func NewShcMetrics(reg prometheus.Registerer) *PrometheusMetrics {
	m := &PrometheusMetrics{
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

func writeDeviceEventsToMetricsRegistry(event shcStructs.DeviceEvent) {
	if strings.HasPrefix(event.DeviceId, "hdm:") {
		r := deviceMapping[event.DeviceId]

		switch event.Id {
		case "PowerMeter":
			{
				sugar.Infof("%s %s Total: %f, Actual: %f, Room: %s", event.Id, event.DeviceId, event.State["energyConsumption"], event.State["powerConsumption"], r)
				metrics.energyConsumption.WithLabelValues(event.DeviceId, r).Set(event.State["energyConsumption"].(float64))
				metrics.powerConsumption.WithLabelValues(event.DeviceId, r).Set(event.State["powerConsumption"].(float64))
			}
		case "ShutterControl":
			{
				sugar.Infof("%s %s ShutterLevel: %f, Room: %s", event.Id, event.DeviceId, event.State["level"], r)
				metrics.shutterLevel.WithLabelValues(event.DeviceId, r).Set(event.State["level"].(float64))
			}
		case "HumidityLevel":
			{
				sugar.Infof("%s %s, Humidity: %f, Room: %s", event.Id, event.DeviceId, event.State["humidity"], r)
				metrics.roomHumidity.WithLabelValues(event.DeviceId, r).Set(event.State["humidity"].(float64))

			}
		case "TemperatureLevel":
			{
				sugar.Infof("%s %s, Temperature: %f, Room: %s", event.Id, event.DeviceId, event.State["temperature"], r)
				metrics.roomTemperature.WithLabelValues(event.DeviceId, r).Set(event.State["temperature"].(float64))

			}
		case "PowerSwitch":
			{
				s := event.State["switchState"]
				sugar.Infof("%s %s, State: %s, Room: %s", event.Id, event.DeviceId, s, r)
				if s == "ON" {
					metrics.switchState.WithLabelValues(event.DeviceId, r).Set(1)
				} else {
					metrics.switchState.WithLabelValues(event.DeviceId, r).Set(0)
				}
			}
		default:
			sugar.Info(event)
		}
	} else {
		sugar.Info(event)
	}
}

func createMapping(rooms []shcStructs.Room, devices []shcStructs.Device) (mapping DeviceRoomMap) {
	mapping = make(DeviceRoomMap)
	for _, el := range devices {
		idx := slices.IndexFunc(rooms, func(c shcStructs.Room) bool { return c.Id == el.Room })
		if idx != -1 {
			mapping[el.Id] = rooms[idx].Name
		}
	}
	return
}

var (
	sugar         *zap.SugaredLogger
	deviceMapping DeviceRoomMap
	configPath    string
	metrics       *PrometheusMetrics
)

func NewLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{
		"stdout", "shc_exporter.log",
	}
	return cfg.Build()
}

func initConfig() {
	// set defaults
	viper.SetDefault("files.certificate.crt", "client-cert.pem")
	viper.SetDefault("files.certificate.key", "client-key.pem")
	viper.SetDefault("shc.host", "localhost")
	viper.SetDefault("shc.polltimeout", 30)
	viper.SetDefault("metrics.port", 9123)
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
	viper.SetEnvPrefix("shc")
	viper.SetConfigType("yaml")
	cfg, err := os.ReadFile(configPath)
	if err != nil {
		sugar.Info("No configuration file found. Using Default config")
	}
	err = viper.ReadConfig(bytes.NewBuffer(cfg)) // Find and read the config file
	if err != nil {                              // Handle errors reading the config file
		sugar.Errorf("Error while reading config file: %w. Using Default config", err)
	}
	sugar.Infof("Configuration from %v", viper.AllSettings())
}

func initLogger() {
	logger, _ := NewLogger()
	sugar = logger.Sugar()
}

func initCliFlags() {
	flag.StringVar(&configPath, "configFile", "config.yaml", "Path to the config.yaml File.")
	flag.Parse()
}

func init() {
	initLogger()
	initCliFlags()
	initConfig()
}

func main() {
	defer sugar.Sync() // flushes buffer, if any

	sugar.Info("Starting Influx-Exporter")
	sugar.Info("Reading Crt-File")
	crt, err := os.ReadFile(viper.GetString("files.certificate.crt"))
	if err != nil {
		sugar.Fatal(err)
	}
	sugar.Info("Reading Key-File")
	key, err := os.ReadFile(viper.GetString("files.certificate.key"))
	if err != nil {
		sugar.Fatal(err)
	}

	shcClient := shcClient.NewShcApiClient(viper.GetString("shc.host"), crt, key, sugar)

	rooms := shcClient.GetRooms()
	devices := shcClient.GetDevices()
	deviceMapping = createMapping(rooms, devices)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		sugar.Info("Catch Keyboard interrupt")
		shcClient.Unsubscripe()
		os.Exit(0)
	}()

	sugar.Info("Creating Metrics-Registry")
	// Create a non-global registry.
	reg := prometheus.NewRegistry()

	sugar.Info("Registering Metrics")
	reg.Register(collectors.NewBuildInfoCollector())
	reg.Register(collectors.NewGoCollector())
	// Create new metrics and register them using the custom registry.
	metrics = NewShcMetrics(reg)

	shcClient.Subscripe()
	shcClient.Poll(writeDeviceEventsToMetricsRegistry)

	// Expose metrics and custom registry via an HTTP server
	// using the HandleFor function. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	err = http.ListenAndServe(":"+viper.GetString("metrics.port"), nil)
	if err != nil {
		sugar.Fatal(err)
	}
}
