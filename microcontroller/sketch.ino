#define DECODE_NEC
#include <IRremote.hpp>
#include <DHT.h>
#include <WiFi.h>
#include <PubSubClient.h>
#include <WiFiClientSecure.h>
#include "secret.h"

// --- Pins (adjust if needed) ---
constexpr uint8_t IR_RECV_PIN {3};
constexpr uint8_t RELAY_PIN {2};      // IR controlled LED via relay
constexpr uint8_t LED2_PIN {12};      // Room light controlled by sensors
constexpr uint8_t LDR_PIN {13};       // Analog pin (LDR)
constexpr uint8_t DHT_PIN {37};       // DHT22
constexpr uint8_t MQ2_PIN {10};       // MQ-2 gas sensor
constexpr uint8_t TRIG1_PIN {8};
constexpr uint8_t ECHO1_PIN {9};
constexpr uint8_t TRIG2_PIN {6};
constexpr uint8_t ECHO2_PIN {7};
constexpr uint8_t BUZZER_PIN {5};

constexpr uint16_t TOGGLE_BTN = 0x68;
const int LDR_THRESHOLD = 2000;
const int MQ2_THRESHOLD = 3000;
const int BUZZER_FREQ = 2000;
#define DHTTYPE DHT22
DHT dht(DHT_PIN, DHTTYPE);

// Person counting
volatile int personCount = 0;
unsigned long lastActivate1 = 0;
unsigned long lastActivate2 = 0;
bool lastState1 = false;
bool lastState2 = false;
const long DISTANCE_TRIGGER_CM = 200;
// Fix: Reduced window from 70s to 5s for realistic transit
const unsigned long PAIR_WINDOW_MS = 5000; 
// Fix: Debounce for individual sensor noise, not used for cross-sensor logic anymore
const unsigned long EDGE_DEBOUNCE_MS = 50; 

// LED2 Manual Mode Control
bool manualMode = false;      // When true, MQTT controls LED2; when false, sensors control LED2
bool manualLedState = false;  // Desired LED2 state when in manual mode

// MQTT Topics
const char* mqttTopicRelay = "myhome/room/relay/set";
const char* mqttTopicMetrics = "myhome/room/metrics";
const char* mqttTopicLED = "myhome/room/led/set";           // LED2 ON/OFF control
const char* mqttTopicManualMode = "myhome/room/led/manual"; // Manual mode ON/OFF
const char* mqttTopicFetch = "myhome/room/fetch";           // Topic to request state fetch

WiFiClientSecure secureClient;
PubSubClient client(secureClient);

// Timers
unsigned long lastPublishTime = 0;
const unsigned long PUBLISH_INTERVAL = 1000; // Publish every 1s instead of delay

// --- Helper functions ---
long getDistancePins(uint8_t trigPin, uint8_t echoPin) {
  digitalWrite(trigPin, LOW);
  delayMicroseconds(2);
  digitalWrite(trigPin, HIGH);
  delayMicroseconds(10);
  digitalWrite(trigPin, LOW);
  long duration = pulseIn(echoPin, HIGH, 30000); // 30ms timeout (approx 5m)
  if (duration == 0) return -1;
  return duration * 0.034 / 2;
}

void mqttCallback(char* topic, byte* payload, unsigned int length) {
  String msg;
  for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
  msg.trim();
  
  String topicStr = String(topic);

  // Handle Relay control
  if (topicStr == mqttTopicRelay) {
    if (msg == "ON") digitalWrite(RELAY_PIN, HIGH);
    else if (msg == "OFF") digitalWrite(RELAY_PIN, LOW);
    else if (msg == "TOGGLE") digitalWrite(RELAY_PIN, !digitalRead(RELAY_PIN));
    Serial.println("Relay command: " + msg);
  }
  
  // Handle LED2 control
  else if (topicStr == mqttTopicLED) {
    if (msg == "ON") manualLedState = true;
    else if (msg == "OFF") manualLedState = false;
    else if (msg == "TOGGLE") manualLedState = !manualLedState;
    
    if (manualMode) digitalWrite(LED2_PIN, manualLedState ? HIGH : LOW);
    Serial.println("LED command: " + msg);
  }
  
  // Handle Manual Mode toggle
  else if (topicStr == mqttTopicManualMode) {
    if (msg == "ON") {
      manualMode = true;
      digitalWrite(LED2_PIN, manualLedState ? HIGH : LOW);
    }
    else if (msg == "OFF") manualMode = false;
    Serial.println("Manual mode: " + msg);
  }
}

void reconnectMQTT() {
  if (!client.connected()) {
    Serial.print("Connecting to AWS IoT...");
    if (client.connect("ESP32_SmartHome")) {
      Serial.println("connected!");
      client.subscribe(mqttTopicRelay);
      client.subscribe(mqttTopicLED);
      client.subscribe(mqttTopicManualMode);
      
      // Send fetch event on boot/connection
      client.publish(mqttTopicFetch, "1");
      Serial.println("Sent state fetch request");
    } else {
      Serial.print("failed, rc=");
      Serial.print(client.state());
      Serial.println(" try again in 2s");
      delay(2000);
    }
  }
}

void handleUltrasonicPairing(bool rising1, bool rising2) {
  unsigned long now = millis();
  
  if (rising1) {
    lastActivate1 = now;
    // Fix: Check if Sensor 2 was triggered recently (Entry 2->1 implies Exit if sensor 2 is inside? Wait.
    // Usually 1 is outside, 2 is inside. 
    // Entry: 1 -> 2. Exit: 2 -> 1.
    // Let's assume 1->2 is Entry (Count++) and 2->1 is Exit (Count--).
    
    // If 2 was triggered before 1 (and within window), it is an Exit (2 -> 1)
    // Fix: Removed EDGE_DEBOUNCE_MS check here to allow fast movement
    if (lastActivate2 > 0 && (now - lastActivate2) <= PAIR_WINDOW_MS) {
       if (lastActivate2 < lastActivate1) { 
         if (personCount > 0) personCount--; 
         Serial.println("Person Exited. Count: " + String(personCount));
         lastActivate1 = lastActivate2 = 0; // Reset sequence
       }
    }
  }
  
  if (rising2) {
    lastActivate2 = now;
    // If 1 was triggered before 2 (and within window), it is an Entry (1 -> 2)
    // Fix: Removed EDGE_DEBOUNCE_MS check here
    if (lastActivate1 > 0 && (now - lastActivate1) <= PAIR_WINDOW_MS) {
      if (lastActivate1 < lastActivate2) { 
        personCount++; 
        Serial.println("Person Entered. Count: " + String(personCount));
        lastActivate1 = lastActivate2 = 0; // Reset sequence
      }
    }
  }
}

// Publish metrics
void publishMetrics(int ldr, long d1, long d2, int mq2Value, float temp, float hum, int count) {
  String payload = "{";
  payload += "\"LDR\":" + String(ldr);
  payload += ",\"Dist1\":" + String(d1);
  payload += ",\"Dist2\":" + String(d2);
  payload += ",\"MQ2\":" + String(mq2Value);
  payload += ",\"Temp\":" + String(temp);
  payload += ",\"Hum\":" + String(hum);
  payload += ",\"Count\":" + String(count);
  payload += ",\"Relay\":" + String(digitalRead(RELAY_PIN) ? "true" : "false");
  payload += ",\"LED2\":" + String(digitalRead(LED2_PIN) ? "true" : "false");
  payload += ",\"ManualMode\":" + String(manualMode ? "true" : "false");
  payload += "}";
  client.publish(mqttTopicMetrics, payload.c_str());
}

// --- Setup ---
void setup() {
  Serial.begin(115200);
  
  pinMode(RELAY_PIN, OUTPUT); digitalWrite(RELAY_PIN, LOW);
  pinMode(LED2_PIN, OUTPUT); digitalWrite(LED2_PIN, LOW);
  pinMode(MQ2_PIN, INPUT);
  pinMode(TRIG1_PIN, OUTPUT); pinMode(ECHO1_PIN, INPUT);
  pinMode(TRIG2_PIN, OUTPUT); pinMode(ECHO2_PIN, INPUT);
  pinMode(BUZZER_PIN, OUTPUT); digitalWrite(BUZZER_PIN, LOW);

  IrReceiver.begin(IR_RECV_PIN);
  dht.begin();

  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  Serial.println("\nWiFi Connected");

  secureClient.setCACert(AWS_CERT_CA);
  secureClient.setCertificate(AWS_CERT_CRT);
  secureClient.setPrivateKey(AWS_CERT_PRIVATE);

  client.setServer(AWS_IOT_ENDPOINT, AWS_IOT_PORT);
  client.setCallback(mqttCallback);
  
  reconnectMQTT();
}

// --- Loop ---
void loop() {
  if (!client.connected()) reconnectMQTT();
  client.loop();

  // IR Remote
  if (IrReceiver.decode()) {
    if (IrReceiver.decodedIRData.command == TOGGLE_BTN) {
      digitalWrite(RELAY_PIN, !digitalRead(RELAY_PIN));
    }
    IrReceiver.resume();
  }

  // Read Sensors (Fast Polling)
  int ldrValue = analogRead(LDR_PIN);
  long dist1 = getDistancePins(TRIG1_PIN, ECHO1_PIN);
  long dist2 = getDistancePins(TRIG2_PIN, ECHO2_PIN);
  int mq2Value = analogRead(MQ2_PIN);

  // Person Counting Logic
  // Threshold logic
  bool objectClose1 = (dist1 > 0 && dist1 < DISTANCE_TRIGGER_CM);
  bool objectClose2 = (dist2 > 0 && dist2 < DISTANCE_TRIGGER_CM);
  unsigned long now = millis();

  // Detect Rising Edges (State changed from Open -> Blocked)
  // Added explicit debounce to avoid flickering triggers from same person standing in beam
  // Fix: Ensure debounce is only for the SAME sensor to avoid noise
  bool rising1 = objectClose1 && !lastState1 && (now - lastActivate1 > EDGE_DEBOUNCE_MS);
  bool rising2 = objectClose2 && !lastState2 && (now - lastActivate2 > EDGE_DEBOUNCE_MS);

  lastState1 = objectClose1; 
  lastState2 = objectClose2;

  if (rising1 || rising2) handleUltrasonicPairing(rising1, rising2);
  if (personCount < 0) personCount = 0;

  // LED2 Logic
  if (!manualMode) {
    if (personCount > 0 && ldrValue > LDR_THRESHOLD) digitalWrite(LED2_PIN, HIGH);
    else digitalWrite(LED2_PIN, LOW);
  } else {
    digitalWrite(LED2_PIN, manualLedState ? HIGH : LOW);
  }

  // Alarm
  if (mq2Value > MQ2_THRESHOLD) tone(BUZZER_PIN, BUZZER_FREQ);
  else noTone(BUZZER_PIN);

  // Publish Metrics (Non-blocking)
  // Fix: Replaced delay(500) with non-blocking timer
  if (millis() - lastPublishTime > PUBLISH_INTERVAL) {
    float t = dht.readTemperature();
    float h = dht.readHumidity();
    publishMetrics(ldrValue, dist1, dist2, mq2Value, t, h, personCount);
    lastPublishTime = millis();
  }
  
  // Small delay to prevent CPU hogging, but small enough for sensors
  delay(50);
}
