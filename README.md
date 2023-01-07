[![Go](https://github.com/zabeloliver/shc-prometheus-exporter/actions/workflows/go.yml/badge.svg)](https://github.com/zabeloliver/shc-prometheus-exporter/actions/workflows/go.yml)  

# shc-prometheus-exporter
Exports Bosch SHC Values to be scrapped by Prometheus.

## Todos (Order does not imply priority)
[ ] Code restructoring  
[ ] Include Bosch CA Certificate  
[ ] Add more Events  
[ ] Add CLI-Args parallel to the `config.yaml` as well as support for Env-Vars  
[ ] Create Github Release-Action  


## Usage
1. Install shc-prometheus-exporter
  a. Option 1: Download pre-build executable for your platform from the latest release
  b. Option 2: If you have already go installed, you can install it by go install github.com/zabeloliver/shc-prometheus-exporter@latest
2. Create a new client within your SHC according to https://github.com/BoschSmartHome/bosch-shc-api-docs/tree/master/postman
3. Create a `config.yaml` with your settings:  
``` yaml
filenames:
  crt: client-cert.pem # Filename of your crt-file. Needs to be in the same folder as the executable
  key: client-key.pem # Filename of your key-file. Needs to be in the same folder as the executable

shc:
  ip: 169.254.127.236 # IP Address of your SHC
  pollTimeout: 30 # Long-Polling Timeout

metrics:
  port: 9123 # Port, where to access the metrics. For example http://localhost:9123/metrics
  ```  
4. Run the application
5. Optional: Configure Prometheus and Grafana (Tip: Use Docker-Containers )