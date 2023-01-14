package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

type DeviceRoomMap map[string]string

type JsonRPC struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type JsonRPCResult struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  string `json:"result"`
}

type PollResult struct {
	Jsonrpc string        `json:"jsonrpc"`
	Result  []DeviceEvent `json:"result"`
	Error   JsonRpcError  `json:"error,omitempty"`
}

type JsonRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func getResourcePath(path string) []byte {
	// Request /hello via the created HTTPS client over port 8443 via GET
	url, _ := url.JoinPath(shcApiUrl, path)
	r, err := apiClient.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	// Read the response body
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Fatal(err)
	}
	return body
}

func getRooms() []Room {
	var path string = "rooms"
	var rooms []Room

	// Print the response body to stdout
	err := json.Unmarshal(getResourcePath(path), &rooms)
	if err != nil {
		log.Fatal(err)
	}
	sugar.Info("Get List of Rooms: ", len(rooms))
	return rooms
}

func getDevices() []Device {
	var path string = "devices"
	var devices []Device

	// Print the response body to stdout
	err := json.Unmarshal(getResourcePath(path), &devices)
	if err != nil {
		log.Fatal(err)
	}
	sugar.Info("Get List of Devices: ", len(devices))
	return devices
}

func subscripe() {
	sugar.Info("Getting Polling ID")
	payload, err := json.Marshal(subscribeBody)
	if err != nil {
		sugar.Panic(err)
	}
	bodyReader := bytes.NewReader(payload)
	if err != nil {
		sugar.Panic(err)
	}
	sugar.Info("Subscription Request: ", string(payload))
	res, err := apiClient.Post(shcPollUrl, "application/json", bodyReader)

	if err != nil {
		sugar.Panic(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		sugar.Panic(err)
	}
	rpc := JsonRPCResult{}
	err = json.Unmarshal(body, &rpc)
	if err != nil {
		sugar.Panic(err)
	}
	id := rpc.Result

	// prefill rpc-bodies
	pollBody.Params = []any{id, viper.GetString("shc.polltimeout")}
	unsubscribeBody.Params = []any{id}
	sugar.Info("Subscribtion ID: ", id)
}

func unsubscripe() {
	sugar.Info("Unsubscribing from Polling")
	payload, err := json.Marshal(unsubscribeBody)
	if err != nil {
		sugar.Panic(err)
	}
	bodyReader := bytes.NewReader(payload)
	if err != nil {
		sugar.Panic(err)
	}
	sugar.Info("Unsubscription Request: ", string(payload))
	res, err := apiClient.Post(shcPollUrl, "application/json", bodyReader)
	if err != nil {
		sugar.Panic(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		sugar.Panic(err)
	}
	rpc := JsonRPCResult{}
	err = json.Unmarshal(body, &rpc)
	if err != nil {
		sugar.Panic(err)
	}
	sugar.Info("Unsubscribe Response: ", rpc)
}

func poll(metrics *metrics) {
	sugar.Info("Starting Long Polling")
	go func() {
		for {
			payload, err := json.Marshal([]JsonRPC{pollBody})
			//sugar.Info(string(payload))
			if err != nil {
				sugar.Panic(err)
			}
			bodyReader := bytes.NewReader(payload)
			if err != nil {
				sugar.Panic(err)
			}

			res, err := apiClient.Post(shcPollUrl, "application/json", bodyReader)
			if err != nil {
				sugar.Error(err)
				// wait some time before trying a new request
				time.Sleep(5 * time.Second)
				continue
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				sugar.Error(err)
				continue
			}
			results := []PollResult{}
			err = json.Unmarshal(body, &results)
			if err != nil {
				sugar.Error(err)
				continue
			}

			if len(results[0].Result) != 0 {
				for _, event := range results[0].Result {
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
			} else if results[0].Error.Code != 0 {
				sugar.Error(results)
				sugar.Info("Somehow, there is an issue, when the polling ID is invalid, during the polling.")
				sugar.Info("The returned error from SHC is empty without message and subsequent calls will hang. So this is an workaround where we generate a new polling ID.")
				time.Sleep(5 * time.Second)
				// resubscripe
				subscripe()
			}
		}
	}()
}

func createMapping(rooms []Room, devices []Device) (mapping DeviceRoomMap) {
	mapping = make(DeviceRoomMap)
	for _, el := range devices {
		idx := slices.IndexFunc(rooms, func(c Room) bool { return c.Id == el.Room })
		if idx != -1 {
			mapping[el.Id] = rooms[idx].Name
		}
	}
	return
}

var (
	logger          *zap.Logger
	sugar           *zap.SugaredLogger
	unsubscribeBody JsonRPC = JsonRPC{Jsonrpc: "2.0", Method: "RE/unsubscribe"}
	subscribeBody   JsonRPC = JsonRPC{Jsonrpc: "2.0", Method: "RE/subscribe", Params: []any{"com/bosch/sh/remote/*", nil}}
	pollBody        JsonRPC = JsonRPC{Jsonrpc: "2.0", Method: "RE/longPoll"}
	deviceMapping   DeviceRoomMap
	apiClient       *http.Client
	shcApiUrl       string
	shcPollUrl      string
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
	viper.SetDefault("filenames.crt", "client-cert.pem")
	viper.SetDefault("filenames.key", "client-key.pem")
	viper.SetDefault("shc.ip", "localhost")
	viper.SetDefault("shc.polltimeout", 30)
	viper.SetDefault("metrics.port", 9123)
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("shc")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetConfigType("yml")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		sugar.Errorf("fatal error config file: %w. Using Default config", err)
	}
	sugar.Infof("Configuration: %v", viper.AllSettings())
}

func initLogger() {
	logger, _ = NewLogger()
	sugar = logger.Sugar()
}

func init() {
	initLogger()
	initConfig()
}

func main() {
	defer logger.Sync() // flushes buffer, if any

	shcApiUrl = "https://" + viper.GetString("shc.ip") + ":8444/smarthome"
	shcPollUrl = "https://" + viper.GetString("shc.ip") + ":8444/remote/json-rpc"

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		sugar.Info("Catch Keyboard interrupt")
		unsubscripe()
		os.Exit(1)
	}()

	sugar.Info("Starting Application")
	sugar.Info("Reading Crt-File")
	crt, err := os.ReadFile(viper.GetString("filenames.crt"))
	if err != nil {
		sugar.Panic(err)
	}
	sugar.Info("Reading Key-File")
	key, err := os.ReadFile(viper.GetString("filenames.key"))
	if err != nil {
		sugar.Panic(err)
	}

	sugar.Info("Generating X509-KeyPair")
	cert, err := tls.X509KeyPair(crt, key)
	if err != nil {
		sugar.Panic(err)
	}

	// https://venilnoronha.io/a-step-by-step-guide-to-mtls-in-go
	apiClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: true,
			},
			TLSHandshakeTimeout: 10 * time.Second,
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
		},
		Timeout: 2 * time.Duration(viper.GetInt("shc.polltimeout")) * time.Second,
	}

	sugar.Info("Creating Metrics-Registry")
	// Create a non-global registry.
	reg := prometheus.NewRegistry()

	sugar.Info("Registering Metrics")
	reg.Register(collectors.NewBuildInfoCollector())
	reg.Register(collectors.NewGoCollector())
	// Create new metrics and register them using the custom registry.
	//m := NewMetrics(reg)

	rooms := getRooms()
	devices := getDevices()
	deviceMapping = createMapping(rooms, devices)

	//updateMetrics(client, thermos, m)
	//subscripe()
	//poll(m)

	// Expose metrics and custom registry via an HTTP server
	// using the HandleFor function. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	metricsPath := ":" + viper.GetString("metrics.port")
	sugar.Infof("Metrics served at: %v", metricsPath)
	err = http.ListenAndServe(metricsPath, nil)
	if err != nil {
		sugar.Fatal(err)
	}
}
