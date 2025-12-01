package main

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

//go:embed index.html
var staticFiles embed.FS

// MQTT Topics
const (
	mqttTopicRelay      = "myhome/room/relay/set"
	mqttTopicMetrics    = "myhome/room/metrics"
	mqttTopicLED        = "myhome/room/led/set"
	mqttTopicManualMode = "myhome/room/led/manual"
)

var (
	mqttClient mqtt.Client
	metrics    = make(map[string]interface{})
	mu         sync.RWMutex

	ledState   bool
	manualMode bool
	stateMu    sync.RWMutex
)

// Publish helper with logging
func publish(topic, msg string) {
	token := mqttClient.Publish(topic, 0, false, msg)
	token.Wait()
	if token.Error() != nil {
		log.Printf("Publish failed on %s: %v\n", topic, token.Error())
	} else {
		log.Printf("Published '%s' to topic '%s'\n", msg, topic)
	}
}

// -------- HTTP Handlers ----------
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
	w.Write([]byte("Manual mode disabled\n"))
}

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

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	mu.RLock()
	defer mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	content, err := staticFiles.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Could not load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// MQTT metrics callback
func mqttMetricsCallback(client mqtt.Client, msg mqtt.Message) {
	var newMetrics map[string]interface{}
	if err := json.Unmarshal(msg.Payload(), &newMetrics); err != nil {
		log.Println("Failed to parse metrics:", err)
		return
	}
	mu.Lock()
	for k, v := range newMetrics {
		metrics[k] = v
	}
	mu.Unlock()
	log.Printf("Received metrics: %v\n", newMetrics)
}

// -------- AWS IoT TLS Setup ----------
func createTLSConfig() *tls.Config {
	certPath := os.Getenv("AWS_CERT_PATH")
	keyPath := os.Getenv("AWS_KEY_PATH")
	caPath := os.Getenv("AWS_CA_PATH")

	if certPath == "" || keyPath == "" || caPath == "" {
		log.Fatal("Missing AWS IoT certificate environment variables")
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Fatalf("Failed to load cert/key: %v", err)
	}

	caCert, err := os.ReadFile(caPath)
	if err != nil {
		log.Fatalf("Failed to read CA file: %v", err)
	}

	ca := x509.NewCertPool()
	if ok := ca.AppendCertsFromPEM(caCert); !ok {
		log.Fatal("Failed to append CA certificate")
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            ca,
		InsecureSkipVerify: false,
	}
}

func main() {
	awsEndpoint := os.Getenv("AWS_IOT_ENDPOINT")
	if awsEndpoint == "" {
		log.Fatal("Missing AWS_IOT_ENDPOINT environment variable")
	}

	mqttBroker := "ssl://" + awsEndpoint + ":8883"
	tlsConfig := createTLSConfig()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetClientID("go_mqtt_controller_unique") // Make sure unique
	opts.SetTLSConfig(tlsConfig)
	opts.SetKeepAlive(60)
	opts.SetPingTimeout(10)
	opts.AutoReconnect = true

	mqttClient = mqtt.NewClient(opts)
	if tok := mqttClient.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatal("MQTT Connect failed:", tok.Error())
	}
	log.Println("Connected to AWS IoT")

	// Subscribe to metrics
	if tok := mqttClient.Subscribe(mqttTopicMetrics, 0, mqttMetricsCallback); tok.Wait() && tok.Error() != nil {
		log.Fatal("MQTT Subscribe failed:", tok.Error())
	}
	log.Println("Subscribed to metrics topic")

	// HTTP Endpoints
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/relay/on", relayOn)
	http.HandleFunc("/relay/off", relayOff)
	http.HandleFunc("/led/on", ledOn)
	http.HandleFunc("/led/off", ledOff)
	http.HandleFunc("/manual/on", manualModeOn)
	http.HandleFunc("/manual/off", manualModeOff)
	http.HandleFunc("/state", stateHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("HTTP server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
