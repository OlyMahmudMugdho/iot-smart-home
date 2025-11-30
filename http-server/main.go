package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

//go:embed index.html
var staticFiles embed.FS

const (
	mqttBroker          = "tcp://broker.hivemq.com:1883"
	mqttTopicRelay      = "myhome/room/relay/set"
	mqttTopicMetrics    = "myhome/room/metrics"
	mqttTopicLED        = "myhome/room/led/set"
	mqttTopicManualMode = "myhome/room/led/manual"
)

var (
	mqttClient mqtt.Client
	metrics    = make(map[string]interface{})
	mu         sync.RWMutex

	// LED and Manual Mode state
	ledState   bool
	manualMode bool
	stateMu    sync.RWMutex
)

// Publish to a specific topic
func publish(topic, msg string) {
	token := mqttClient.Publish(topic, 0, false, msg)
	token.Wait()
}

// HTTP Handlers
func relayOn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	publish(mqttTopicRelay, "ON")
	w.Write([]byte("Relay ON command sent\n"))
}

func relayOff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	publish(mqttTopicRelay, "OFF")
	w.Write([]byte("Relay OFF command sent\n"))
}

// LED Control Handlers
func ledOn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	stateMu.Lock()
	ledState = true
	stateMu.Unlock()
	publish(mqttTopicLED, "ON")
	w.Write([]byte("LED ON command sent\n"))
}

func ledOff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	stateMu.Lock()
	ledState = false
	stateMu.Unlock()
	publish(mqttTopicLED, "OFF")
	w.Write([]byte("LED OFF command sent\n"))
}

// Manual Mode Handlers
func manualModeOn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	stateMu.Lock()
	manualMode = true
	stateMu.Unlock()
	publish(mqttTopicManualMode, "ON")
	w.Write([]byte("Manual mode enabled\n"))
}

func manualModeOff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	stateMu.Lock()
	manualMode = false
	stateMu.Unlock()
	publish(mqttTopicManualMode, "OFF")
	w.Write([]byte("Manual mode disabled - sensors will control LED\n"))
}

// Get current state
func stateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	stateMu.RLock()
	defer stateMu.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ledState":   ledState,
		"manualMode": manualMode,
	})
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// Serve the UI
func indexHandler(w http.ResponseWriter, r *http.Request) {
	content, err := staticFiles.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Could not load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
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
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/relay/on", relayOn)
	http.HandleFunc("/relay/off", relayOff)
	http.HandleFunc("/led/on", ledOn)
	http.HandleFunc("/led/off", ledOff)
	http.HandleFunc("/manual/on", manualModeOn)
	http.HandleFunc("/manual/off", manualModeOff)
	http.HandleFunc("/state", stateHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("HTTP Server running on :8080")
	fmt.Println("Open http://localhost:8080 in your browser")
	fmt.Println("MQTT Topics:")
	fmt.Println("  - Relay:       ", mqttTopicRelay)
	fmt.Println("  - LED:         ", mqttTopicLED)
	fmt.Println("  - Manual Mode: ", mqttTopicManualMode)
	fmt.Println("  - Metrics:     ", mqttTopicMetrics)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
