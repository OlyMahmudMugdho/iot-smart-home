package main

import (
	"fmt"
	"log"
	"net/http"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	mqttBroker = "tcp://broker.hivemq.com:1883"
	mqttTopic  = "myhome/room/relay/set"
)

var mqttClient mqtt.Client

func publish(msg string) {
	token := mqttClient.Publish(mqttTopic, 0, false, msg)
	token.Wait()
}

func relayOn(w http.ResponseWriter, r *http.Request) {
	publish("ON")
	w.Write([]byte("Relay ON command sent\n"))
}

func relayOff(w http.ResponseWriter, r *http.Request) {
	publish("OFF")
	w.Write([]byte("Relay OFF command sent\n"))
}

func main() {
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("go_mqtt_controller")

	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	http.HandleFunc("/relay/on", relayOn)
	http.HandleFunc("/relay/off", relayOff)

	fmt.Println("HTTP Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
