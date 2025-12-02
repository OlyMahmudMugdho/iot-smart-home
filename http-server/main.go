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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
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
	mqttTopicFetch      = "myhome/room/fetch"
)

var (
	mqttClient mqtt.Client
	metrics    = make(map[string]interface{})
	mu         sync.RWMutex

	ledState   bool
	manualMode bool
	stateMu    sync.RWMutex

	// DynamoDB
	svc       *dynamodb.DynamoDB
	tableName string
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

	// Save to DynamoDB
	go saveStateToDynamoDB(newMetrics)
}

// MQTT fetch callback
func mqttFetchCallback(client mqtt.Client, msg mqtt.Message) {
	log.Println("Received fetch event from ESP32, syncing state...")
	go syncStateToDevice()
}

// -------- DynamoDB Logic ----------

func initDynamoDB() {
	tableName = os.Getenv("DDB_TABLE")
	if tableName == "" {
		log.Println("Warning: DDB_TABLE env var not set. DynamoDB features disabled.")
		return
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc = dynamodb.New(sess)
}

func saveStateToDynamoDB(data map[string]interface{}) {
	if svc == nil || tableName == "" {
		return
	}

	item := make(map[string]interface{})
	for k, v := range data {
		item[k] = v
	}
	// Primary Key: state_id (Number) = 1
	// Based on user data: "state_id": { "N": "1" }
	item["state_id"] = 1

	// Add timestamp (likely a sort key based on schema analysis)
	item["timestamps"] = time.Now().Unix()

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Println("Error marshalling item for DynamoDB:", err)
		return
	}

	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}

	_, err = svc.PutItem(input)
	if err != nil {
		log.Println("Error saving to DynamoDB:", err)
	} else {
		log.Println("Saved state to DynamoDB")
	}
}

func syncStateToDevice() {
	if svc == nil || tableName == "" {
		return
	}

	log.Println("Syncing state from DynamoDB to Device...")

	// Use Query to get the latest item for state_id = 1
	input := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("state_id = :sid"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":sid": {N: aws.String("1")},
		},
		// If timestamps is a sort key, this gets the latest.
		// If there is no sort key, this just returns the item(s).
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int64(1),
	}

	result, err := svc.Query(input)
	if err != nil {
		log.Println("Error fetching from DynamoDB:", err)
		return
	}

	if len(result.Items) == 0 {
		log.Println("No state found in DynamoDB")
		return
	}

	var state map[string]interface{}
	err = dynamodbattribute.UnmarshalMap(result.Items[0], &state)
	if err != nil {
		log.Println("Error unmarshalling DynamoDB item:", err)
		return
	}

	// Sync Relay
	if val, ok := state["Relay"]; ok {
		shouldBeOn := false
		if b, ok := val.(bool); ok {
			shouldBeOn = b
		} else if s, ok := val.(string); ok {
			shouldBeOn = (s == "true")
		}

		if shouldBeOn {
			publish(mqttTopicRelay, "ON")
		} else {
			publish(mqttTopicRelay, "OFF")
		}
	}

	// Sync Manual Mode
	if val, ok := state["ManualMode"]; ok {
		shouldBeOn := false
		if b, ok := val.(bool); ok {
			shouldBeOn = b
		} else if s, ok := val.(string); ok {
			shouldBeOn = (s == "true")
		}

		if shouldBeOn {
			publish(mqttTopicManualMode, "ON")
			// Also sync LED state if Manual Mode is ON
			if ledVal, ok := state["LED2"]; ok {
				ledOn := false
				if b, ok := ledVal.(bool); ok {
					ledOn = b
				} else if s, ok := ledVal.(string); ok {
					ledOn = (s == "true")
				}
				if ledOn {
					publish(mqttTopicLED, "ON")
				} else {
					publish(mqttTopicLED, "OFF")
				}
			}
		} else {
			publish(mqttTopicManualMode, "OFF")
		}
	}
}

// -------- AWS IoT TLS Setup ----------
func createTLSConfig() *tls.Config {
	// Use Environment Variables for content
	certPEM := os.Getenv("AWS_CERT_PEM")
	keyPEM := os.Getenv("AWS_PRIVATE_KEY_PEM")
	caPEM := os.Getenv("AWS_ROOT_CA_PEM")

	if certPEM == "" || keyPEM == "" || caPEM == "" {
		log.Fatal("Missing AWS IoT certificate environment variables (PEM content)")
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		log.Fatalf("Failed to load cert/key pair: %v", err)
	}

	ca := x509.NewCertPool()
	if ok := ca.AppendCertsFromPEM([]byte(caPEM)); !ok {
		log.Fatal("Failed to append CA certificate")
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            ca,
		InsecureSkipVerify: false,
	}
}

func main() {
	// Init DynamoDB
	initDynamoDB()

	awsEndpoint := os.Getenv("AWS_IOT_ENDPOINT")
	if awsEndpoint == "" {
		log.Fatal("Missing AWS_IOT_ENDPOINT environment variable")
	}

	mqttBroker := "ssl://" + awsEndpoint + ":8883"
	tlsConfig := createTLSConfig()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetClientID("go_mqtt_controller_unique_" + fmt.Sprint(time.Now().Unix()))
	opts.SetTLSConfig(tlsConfig)
	opts.SetKeepAlive(60)
	opts.SetPingTimeout(10)
	opts.AutoReconnect = true
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("Connected to AWS IoT")
		// Subscribe to Metrics
		if tok := c.Subscribe(mqttTopicMetrics, 0, mqttMetricsCallback); tok.Wait() && tok.Error() != nil {
			log.Println("MQTT Subscribe metrics failed:", tok.Error())
		} else {
			log.Println("Subscribed to metrics topic")
		}
		// Subscribe to Fetch Events
		if tok := c.Subscribe(mqttTopicFetch, 0, mqttFetchCallback); tok.Wait() && tok.Error() != nil {
			log.Println("MQTT Subscribe fetch failed:", tok.Error())
		} else {
			log.Println("Subscribed to fetch topic")
		}
	})

	mqttClient = mqtt.NewClient(opts)
	if tok := mqttClient.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatal("MQTT Connect failed:", tok.Error())
	}

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
