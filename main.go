package main

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	hostUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pingz_host_up",
		Help: "1 if the host is up, 0 otherwise",
	}, []string{"host"},
	)
)

type Host struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
}

type Config struct {
	Port      int64  `yaml:"port"`
	Frequency string `yaml:"frequency"`
	Hosts     []Host `yaml:"hosts"`
}

func NewConfig() Config {
	return Config{Port: 3000, Frequency: "1s"}
}

func readConfig(path string) (Config, error) {
	config := NewConfig()

	log.Tracef("Reading config file '%s'", path)
	dat, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(dat, &config)
	if err != nil {
		return config, err
	}

	for _, host := range config.Hosts {
		_, err := url.ParseRequestURI(host.Url)
		if err != nil {
			log.Fatalf("Invalid URL: name=%s url=%s err=%s", host.Name, host.Url, err)
		}
	}

	return config, nil
}

func (h *Host) ping() error {
	resp, err := http.Get(h.Url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("response code was not '200'")
	}

	return nil
}

func pingLoop(config Config) {
	frequency, err := time.ParseDuration(config.Frequency)
	if err != nil {
		log.Fatalf("Failed to parse frequency: %s", err)
	}

	log.Infof("Pinging every %s", config.Frequency)

	for {
		start := time.Now()

		for _, host := range config.Hosts {
			err := host.ping()
			isUp := err == nil

			if isUp {
				hostUp.WithLabelValues(host.Name).Set(1.0)
			} else {
				hostUp.WithLabelValues(host.Name).Set(0.0)
				log.Warnf("ping: name=%s url=%s isUp=%t err=%s", host.Name, host.Url, isUp, err)
			}
		}

		end := time.Now()
		diff := end.Sub(start)
		time.Sleep(frequency - diff)
	}
}

func main() {
	log.Info("Starting pingz...")
	path := getopt.StringLong("config", 'c', "./config.yaml", "Path to config.yaml file")
	help := getopt.BoolLong("help", 'h', "Help")
	getopt.Parse()

	if *help {
		getopt.Usage()
		os.Exit(0)
	}

	config, err := readConfig(*path)
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err)
	}

	log.Infof("Read config at path '%s'", *path)
	log.Infof("%+v", config)

	go pingLoop(config)

	log.Infof("Listening on 0.0.0.0:%d", config.Port)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+strconv.FormatInt(config.Port, 10), nil))
}
