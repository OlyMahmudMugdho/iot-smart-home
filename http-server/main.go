package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	mqttBroker       = "tcp://broker.hivemq.com:1883"
	mqttTopicRelay   = "myhome/room/relay/set"
	mqttTopicMetrics = "myhome/room/metrics"
)

var (
	mqttClient mqtt.Client
	metrics    = make(map[string]interface{})
	mu         sync.RWMutex
)

// Publish relay commands
func publish(msg string) {
	token := mqttClient.Publish(mqttTopicRelay, 0, false, msg)
	token.Wait()
}

// HTTP Handlers
func relayOn(w http.ResponseWriter, r *http.Request) {
	publish("ON")
	w.Write([]byte("Relay ON command sent\n"))
}

func relayOff(w http.ResponseWriter, r *http.Request) {
	publish("OFF")
	w.Write([]byte("Relay OFF command sent\n"))
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// MQTT callback to receive metrics
func mqttMetricsCallback(client mqtt.Client, msg mqtt.Message) {
	var newMetrics map[string]interface{}
	if err := json.Unmarshal(msg.Payload(), &newMetrics); err != nil {
		log.Println("Failed to unmarshal metrics:", err)
		return
	}
	mu.Lock()
	for k, v := range newMetrics {
		metrics[k] = v
	}
	mu.Unlock()
}

func main() {
	// MQTT client setup
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("go_mqtt_controller")
	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Subscribe to sensor metrics
	if token := mqttClient.Subscribe(mqttTopicMetrics, 0, mqttMetricsCallback); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// HTTP endpoints
	http.HandleFunc("/relay/on", relayOn)
	http.HandleFunc("/relay/off", relayOff)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("HTTP Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
