package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"log"
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
)

// shc config
const shcIp = "169.254.127.236"
const shcApiUrl string = "https://" + shcIp + ":8444/smarthome"
const shcPollUrl string = "https://" + shcIp + ":8444/remote/json-rpc"
const crtName string = "client-cert.pem"
const keyName string = "client-key.pem"
const port string = "9123"
const pollTimeOut = 30

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
}

func (thermostat *Thermostat) getTemperature() {
	state := StateTemperature{}
	json.Unmarshal(getDeviceService(thermostat.Id, "TemperatureLevel"), &state)
	thermostat.Temperature = state.Value
}

func (thermostat *Thermostat) getHumidity() {
	state := StateHumidity{}
	json.Unmarshal(getDeviceService(thermostat.Id, "HumidityLevel"), &state)
	thermostat.Humidity = state.Value
}

func getDeviceService(deviceId string, serviceId string) []byte {
	path, _ := url.JoinPath("devices", deviceId, "services", serviceId)
	return getResourcePath(path)
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
	body, err := ioutil.ReadAll(r.Body)
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
	return devices
}

func getThermostats(devices []Device, rooms []Room) []Thermostat {
	var thermostats []Thermostat
	for _, element := range devices {
		if element.DeviceModel == "BWTH" {
			idx := slices.IndexFunc(rooms, func(c Room) bool { return c.Id == element.Room })
			thermostat := Thermostat{
				Id:   element.Id,
				Room: rooms[idx].Name,
			}
			thermostats = append(thermostats, thermostat)
		}
	}
	return thermostats
}

func getThermostatsState(thermostats []Thermostat) {
	for idx := range thermostats {
		thermostats[idx].getTemperature()
		thermostats[idx].getHumidity()
	}
}

type metrics struct {
	roomTemperature *prometheus.GaugeVec
	roomHumidity    *prometheus.GaugeVec
	switchState     *prometheus.GaugeVec
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
	}
	reg.MustRegister(m.roomTemperature)
	reg.MustRegister(m.roomHumidity)
	reg.MustRegister(m.switchState)
	return m
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

	body, err := ioutil.ReadAll(res.Body)
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
	pollBody.Params = []any{id, pollTimeOut}
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

	body, err := ioutil.ReadAll(res.Body)
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
				sugar.Panic(err)
			}
			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				sugar.Panic(err)
			}
			results := []PollResult{}
			err = json.Unmarshal(body, &results)
			if err != nil {
				sugar.Panic(err)
			}

			if len(results[0].Result) != 0 {
				for _, event := range results[0].Result {
					if strings.HasPrefix(event.DeviceId, "hdm:") {
						r := deviceMapping[event.DeviceId]
						switch event.Id {
						case "HumidityLevel":
							{
								sugar.Info("%s %s", event.Id, event.DeviceId, " State: ", event.State["humidity"], " Room: ", r)
								metrics.roomHumidity.WithLabelValues(event.DeviceId, r).Set(event.State["humidity"].(float64))
							}
						case "TemperatureLevel":
							{
								sugar.Info(event.Id, event.DeviceId, " State: ", event.State["temperature"], " Room: ", r)
								metrics.roomTemperature.WithLabelValues(event.DeviceId, r).Set(event.State["temperature"].(float64))
							}
						case "PowerSwitch":
							{
								s := event.State["switchState"]
								sugar.Info("%s %s", event.Id, event.DeviceId, " State: ", s, " Room: ", r)
								if s == "ON" {
									metrics.switchState.WithLabelValues(event.DeviceId, r).Set(1)
								} else {
									metrics.switchState.WithLabelValues(event.DeviceId, r).Set(0)
								}
							}
						default:
							sugar.Info(event)
						}
					}
				}
			}
		}
	}()
}

func updateMetrics(thermostats []Thermostat, metrics *metrics) {
	sugar.Info("Starting background fetching")
	go func() {
		for {
			getThermostatsState(thermostats)
			for _, element := range thermostats {
				metrics.roomTemperature.WithLabelValues(element.Id, element.Room).Set(element.Temperature)
				metrics.roomHumidity.WithLabelValues(element.Id, element.Room).Set(element.Humidity)
			}
			time.Sleep(5 * time.Second)
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
)

func main() {
	logger, _ = zap.NewDevelopment()
	defer logger.Sync() // flushes buffer, if any
	sugar = logger.Sugar()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		unsubscripe()
		os.Exit(1)
	}()

	sugar.Info("Starting Application")
	sugar.Info("Reading Crt-File")
	crt, err := os.ReadFile(crtName)
	if err != nil {
		sugar.Panic(err)
	}
	sugar.Info("Reading Key-File")
	key, err := os.ReadFile(keyName)
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
		},
	}

	sugar.Info("Creating Metrics-Registry")
	// Create a non-global registry.
	reg := prometheus.NewRegistry()

	sugar.Info("Registering Metrics")
	reg.Register(collectors.NewBuildInfoCollector())
	reg.Register(collectors.NewGoCollector())
	// Create new metrics and register them using the custom registry.
	m := NewMetrics(reg)

	rooms := getRooms()
	sugar.Info("Fetching Rooms: ", len(rooms))

	devices := getDevices()
	sugar.Info("Fetching Devices: ", len(devices))

	thermos := getThermostats(devices, rooms)
	sugar.Info("Fetching Thermostats: ", len(thermos))

	deviceMapping = createMapping(rooms, devices)

	//updateMetrics(client, thermos, m)
	subscripe()
	poll(m)

	// Expose metrics and custom registry via an HTTP server
	// using the HandleFor function. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		sugar.Fatal(err)
	}
}
