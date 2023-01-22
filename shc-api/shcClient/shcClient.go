package shcClient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/zabeloliver/shc-api/shcJsonRpc"
	"github.com/zabeloliver/shc-api/shcStructs"
	"go.uber.org/zap"
)

type ShcApiClient struct {
	Host           string
	client         http.Client
	pollingId      string
	pollingTimeout int
	shcApiUrl      string
	shcPollUrl     string
	logger         *zap.SugaredLogger
}

func NewShcApiClient(host string, crt []byte, key []byte, logger *zap.SugaredLogger) *ShcApiClient {

	logger.Info("Generating X509-KeyPair")
	cert, err := tls.X509KeyPair(crt, key)
	if err != nil {
		logger.Panic(err)
	}

	timeout := 30
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: true,
			},
			TLSHandshakeTimeout: 10 * time.Second,
			Dial: (&net.Dialer{
				Timeout: time.Duration(timeout+5) * time.Second,
			}).Dial,
		},
	}

	return &ShcApiClient{
		Host:           host,
		client:         *httpClient,
		shcApiUrl:      host + ":8444/smarthome",
		shcPollUrl:     host + ":8444/remote/json-rpc",
		pollingTimeout: timeout,
		logger:         logger,
	}
}

func (c *ShcApiClient) SetPollingTimeout(timeout int) {
	c.pollingTimeout = timeout
	c.client.Timeout = time.Duration(timeout+5) * time.Second
}

func (c *ShcApiClient) getResourcePath(path string) []byte {
	// Request /hello via the created HTTPS client over port 8443 via GET
	url, _ := url.JoinPath(c.shcApiUrl, path)
	r, err := c.client.Get(url)
	if err != nil {
		c.logger.Panic(err)
	}

	// Read the response body
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.logger.Error(err)
	}
	return body
}

func (c *ShcApiClient) GetRooms() []shcStructs.Room {
	var path string = "rooms"
	var rooms []shcStructs.Room

	// Print the response body to stdout
	err := json.Unmarshal(c.getResourcePath(path), &rooms)
	if err != nil {
		c.logger.Error(err)
	}
	c.logger.Info("Get List of Rooms: ", len(rooms))
	return rooms
}

func (c *ShcApiClient) GetDevices() []shcStructs.Device {
	var path string = "devices"
	var devices []shcStructs.Device

	// Print the response body to stdout
	err := json.Unmarshal(c.getResourcePath(path), &devices)
	if err != nil {
		c.logger.Panic(err)
	}
	c.logger.Info("Get List of Devices: ", len(devices))
	return devices
}

func (c *ShcApiClient) JsonRpcRequest(request shcJsonRpc.JsonRPC) (result shcJsonRpc.JsonRPCResult) {
	payload, err := json.Marshal(request)
	if err != nil {
		c.logger.Panic(err)
	}
	bodyReader := bytes.NewReader(payload)
	if err != nil {
		c.logger.Panic(err)
	}
	c.logger.Info("JsonRpc Request: ", string(payload))
	res, err := c.client.Post(c.shcPollUrl, "application/json", bodyReader)

	if err != nil {
		c.logger.Panic(err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		c.logger.Panic(err)
	}
	rpc := shcJsonRpc.JsonRPCResult{}
	err = json.Unmarshal(body, &rpc)
	if err != nil {
		c.logger.Panic(err)
	}
	return rpc
}

func (c *ShcApiClient) Subscripe() {
	c.logger.Info("Subscribing to Polling")
	result := c.JsonRpcRequest(shcJsonRpc.JsonRPC{Jsonrpc: "2.0", Method: "RE/subscribe", Params: []any{"com/bosch/sh/remote/*", nil}})
	c.pollingId = result.Result
	c.logger.Info("Subscription Polling ID: ", c.pollingId)
}

func (c *ShcApiClient) Unsubscripe() {
	c.logger.Info("Unsubscribing from Polling")
	if c.pollingId != "" {
		result := c.JsonRpcRequest(shcJsonRpc.JsonRPC{Jsonrpc: "2.0", Method: "RE/unsubscribe", Params: []any{c.pollingId}})
		c.pollingId = ""
		c.logger.Info("Unsubscribe Response: ", result)
	} else {
		c.logger.Warn("Cannot unsubscripe without Polling ID")
	}

}

func (c *ShcApiClient) Poll(f func(event shcStructs.DeviceEvent)) {
	c.logger.Info("Starting Long Polling")
	go func() {
		for {
			payload, err := json.Marshal(shcJsonRpc.JsonRPC{Jsonrpc: "2.0", Method: "RE/longPoll", Params: []any{c.pollingId, c.pollingTimeout}})
			//logger.Info(string(payload))
			if err != nil {
				c.logger.Panic(err)
			}
			bodyReader := bytes.NewReader(payload)
			if err != nil {
				c.logger.Panic(err)
			}

			res, err := c.client.Post(c.shcPollUrl, "application/json", bodyReader)
			if err != nil {
				c.logger.Error(err)
				// wait some time before trying a new request
				time.Sleep(5 * time.Second)
				continue
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				c.logger.Error(err)
				continue
			}

			results := shcJsonRpc.PollResult{}
			err = json.Unmarshal(body, &results)
			if err != nil {
				fmt.Printf("%s : %s", err, body)
				//logger.Errorf("Error: %s, Body: %s - ", err, body)
				continue
			}

			if results.Error.Code != 0 {
				c.logger.Error(results)
				c.logger.Error("Somehow, there is an issue, during the polling. Will resubscripe.")
				time.Sleep(5 * time.Second)
				// resubscripe
				c.Subscripe()
			} else {
				if len(results.Result) != 0 {
					for _, event := range results.Result {
						if f != nil {
							f(event)
						}
					}
				}
				// else: poll timed out without event
			}
		}
	}()
}
