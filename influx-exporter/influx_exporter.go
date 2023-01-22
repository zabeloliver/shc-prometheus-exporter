package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	influxdb2 "github.com/influxdata/influxdb-client-go"
	"github.com/influxdata/influxdb-client-go/api"
	"github.com/spf13/viper"
	"github.com/zabeloliver/shc-api/shcClient"
	"github.com/zabeloliver/shc-api/shcStructs"
)

type DeviceRoomMap map[string]string

func writeDeviceEventsToInfluxDB(event shcStructs.DeviceEvent) {
	if strings.HasPrefix(event.DeviceId, "hdm:") {
		influxLine := ""
		writeLine := false
		r := deviceMapping[event.DeviceId]

		switch event.Id {
		case "PowerMeter":
			{
				sugar.Infof("%s %s Total: %f, Actual: %f, Room: %s", event.Id, event.DeviceId, event.State["energyConsumption"], event.State["powerConsumption"], r)
				influxLine = fmt.Sprintf("shc_energyConsumption,deviceId=%s,room=%s level=%f", event.DeviceId, r, event.State["energyConsumption"].(float64))
				influxLine += fmt.Sprintf("\nshc_powerConsumption,deviceId=%s,room=%s level=%f", event.DeviceId, r, event.State["powerConsumption"].(float64))
				writeLine = true
			}
		case "ShutterControl":
			{
				sugar.Infof("%s %s ShutterLevel: %f, Room: %s", event.Id, event.DeviceId, event.State["level"], r)
				influxLine = fmt.Sprintf("shc_shutterLevel,deviceId=%s,room=%s level=%f", event.DeviceId, r, event.State["level"].(float64))
				writeLine = true
			}
		case "HumidityLevel":
			{
				sugar.Infof("%s %s, Humidity: %f, Room: %s", event.Id, event.DeviceId, event.State["humidity"], r)
				influxLine = fmt.Sprintf("shc_humidityLevel,deviceId=%s,room=%s level=%f", event.DeviceId, r, event.State["humidity"].(float64))
				writeLine = true
			}
		case "TemperatureLevel":
			{
				sugar.Infof("%s %s, Temperature: %f, Room: %s", event.Id, event.DeviceId, event.State["temperature"], r)
				influxLine = fmt.Sprintf("shc_temperatureLevel,deviceId=%s,room=%s level=%f", event.DeviceId, r, event.State["temperature"].(float64))
				writeLine = true
			}
		case "PowerSwitch":
			{
				s := event.State["switchState"]
				sugar.Infof("%s %s, State: %s, Room: %s", event.Id, event.DeviceId, s, r)
				if s == "ON" {
					influxLine = fmt.Sprintf("shc_switchState,deviceId=%s,room=%s state=%du", event.DeviceId, r, 1)
				} else {
					influxLine = fmt.Sprintf("shc_switchState,deviceId=%s,room=%s state=%du", event.DeviceId, r, 0)
				}
				writeLine = true
			}
		default:
			sugar.Info(event)
		}
		if writeLine {
			// add Timestamp to line
			influxLine += " " + fmt.Sprint(time.Now().UTC().UnixNano())
			sugar.Info(influxLine)
			err := influxApi.WriteRecord(context.Background(), influxLine)
			if err != nil {
				sugar.Panic(err)
			}
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
			mapping[el.Id] = strings.ReplaceAll(rooms[idx].Name, " ", "_")
		}
	}
	return
}

var (
	sugar         *zap.SugaredLogger
	deviceMapping DeviceRoomMap
	influxApi     api.WriteAPIBlocking
	wg            sync.WaitGroup
	configPath    string
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

	wg.Add(1)
	shcClient.Subscripe()
	shcClient.Poll(writeDeviceEventsToInfluxDB)

	// Create a new client using an InfluxDB server base URL and an authentication token
	influxClient := influxdb2.NewClient(viper.GetString("influxdb.host"), viper.GetString("influxdb.token"))
	// Use blocking write client for writes to desired bucket
	influxApi = influxClient.WriteAPIBlocking(viper.GetString("influxdb.org"), viper.GetString("influxdb.bucket"))
	wg.Wait()
}
