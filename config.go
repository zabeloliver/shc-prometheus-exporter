package main

type config struct {
	Filenames struct {
		Crt string `yaml:"crt"`
		Key string `yaml:"key"`
	} `yaml:"filenames"`
	Shc struct {
		Ip          string `yaml:"ip"`
		Polltimeout int    `yaml:"pollTimeout"`
	} `yaml:"shc"`
	Metrics struct {
		Port string `yaml:"port"`
	} `yaml:"metrics"`
}

func NewDefaultConfig() (c config) {
	c = config{}
	c.Filenames.Crt = "client-crt.pem"
	c.Filenames.Key = "client-key.pem"
	c.Shc.Ip = "localhost"
	c.Shc.Polltimeout = 30
	c.Metrics.Port = "9123"
	return
}
